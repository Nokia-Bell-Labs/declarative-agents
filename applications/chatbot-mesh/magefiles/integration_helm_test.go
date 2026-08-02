// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSplitImageRef(t *testing.T) {
	cases := []struct {
		image, repo, tag string
	}{
		{"declarative-agents/agent-core:0123456789ab", "declarative-agents/agent-core", "0123456789ab"},
		{"ghcr.io/nokia-bell-labs/agent-core:0.1.0", "ghcr.io/nokia-bell-labs/agent-core", "0.1.0"},
		{"agent-core", "agent-core", "latest"},
		{"localhost:5000/agent-core:dev", "localhost:5000/agent-core", "dev"},
	}
	for _, c := range cases {
		repo, tag := splitImageRef(c.image)
		if repo != c.repo || tag != c.tag {
			t.Errorf("splitImageRef(%q) = (%q, %q), want (%q, %q)", c.image, repo, tag, c.repo, c.tag)
		}
	}
}

func TestChatbotIntegrationImagesPropagateCheckoutRevision(t *testing.T) {
	images, err := resolveChatbotIntegrationImages(filepath.Clean(".."))
	if err != nil {
		t.Fatal(err)
	}
	if len(images.Revision) != 12 {
		t.Fatalf("revision = %q, want 12-character commit", images.Revision)
	}
	for _, image := range []string{images.Runtime, images.Applier} {
		if !strings.HasSuffix(image, ":"+images.Revision) {
			t.Errorf("image %q does not carry revision %s", image, images.Revision)
		}
	}
	if args := strings.Join(smokeRuntimeBuildArgs(images.Runtime), " "); !strings.Contains(args, "-t "+images.Runtime) {
		t.Fatalf("runtime build args omit commit image: %s", args)
	}
}

func TestCollectorDefaultRenderStaysSelfContained(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	out, err := exec.Command("helm", "template", "t", findChartDir(t)).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	render := string(out)
	if strings.Contains(render, "jaeger") {
		t.Fatal("default collector render still references Jaeger")
	}
	for _, forbidden := range []string{"otlp/external:", "resource/integration:", "test.run.id"} {
		if strings.Contains(render, forbidden) {
			t.Errorf("default production render contains integration-only %q", forbidden)
		}
	}
}

func TestCollectorKindOverlayExportsBothSignalsWithRunIdentity(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := findChartDir(t)
	out, err := exec.Command("helm", "template", "t", chart,
		"--values", filepath.Join(chart, "ci", "kind-values.yaml")).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template kind overlay: %v\n%s", err, out)
	}
	render := string(out)
	for _, want := range []string{
		"otlp/external:",
		`endpoint: "host.docker.internal:4317"`,
		"resource/integration:",
		"key: test.repository",
		`value: "Nokia-Bell-Labs/declarative-agents"`,
		"key: test.module",
		"key: test.target",
		"key: vcs.ref.head.revision",
		"key: test.run.id",
		"metrics:",
		"prometheus/dolt:",
		`t-chatbot-mesh-dolt:11228`,
		"CHROMA_OPEN_TELEMETRY__ENDPOINT",
		`http://t-chatbot-mesh-collector-metrics:4317`,
		"CHROMA_OPEN_TELEMETRY__SERVICE_NAME",
		`value: "rag0-chroma"`,
		`args: ["--config", "/etc/dolt/config.yaml"]`,
	} {
		if !strings.Contains(render, want) {
			t.Errorf("kind collector render missing %q", want)
		}
	}
}

func TestDoltDatabaseInitializationRenderContract(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := findChartDir(t)
	render := func(args ...string) string {
		t.Helper()
		command := append([]string{"template", "t", chart}, args...)
		out, err := exec.Command("helm", command...).CombinedOutput()
		if err != nil {
			t.Fatalf("helm template %v: %v\n%s", args, err, out)
		}
		return string(out)
	}

	defaultRender := render()
	for _, want := range []string{
		"name: initialize-dolt-database",
		`command: ["dolt"]`,
		`"--host=t-chatbot-mesh-dolt"`,
		`"CREATE DATABASE IF NOT EXISTS ` + "`agent_checkpoints`" + `"`,
		`"root@tcp(t-chatbot-mesh-dolt:3306)/agent_checkpoints"`,
	} {
		if !strings.Contains(defaultRender, want) {
			t.Errorf("default render missing Dolt initialization contract %q", want)
		}
	}

	customRender := render("--set", "dolt.database=mesh_history")
	for _, want := range []string{
		`"CREATE DATABASE IF NOT EXISTS ` + "`mesh_history`" + `"`,
		`"root@tcp(t-chatbot-mesh-dolt:3306)/mesh_history"`,
	} {
		if !strings.Contains(customRender, want) {
			t.Errorf("custom database render missing %q", want)
		}
	}

	externalRender := render("--set", "dolt.enabled=false")
	for _, forbidden := range []string{
		"initialize-dolt-database",
		"CREATE DATABASE IF NOT EXISTS",
		"--dolt-dsn",
	} {
		if strings.Contains(externalRender, forbidden) {
			t.Errorf("external/disabled Dolt render contains chart-owned %q", forbidden)
		}
	}
}

func TestStageTelemetryKindConfigEnablesControlPlaneTracing(t *testing.T) {
	base := filepath.Join(t.TempDir(), "kind.yaml")
	if err := os.WriteFile(base, []byte(`kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    image: kindest/node:v1.36.1@sha256:test
`), 0o644); err != nil {
		t.Fatal(err)
	}
	path, cleanup, err := stageTelemetryKindConfig(base, helmTelemetryIdentity{
		OTLPEndpoint: "host.docker.internal:4317",
		RunID:        "run-123",
		Commit:       "abc123",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	generated := string(data)
	for _, want := range []string{
		"tracing-config-file", "/etc/kubernetes/tracing.yaml",
		"OTEL_RESOURCE_ATTRIBUTES", "test.run.id=run-123",
		"kind: KubeletConfiguration", "host.docker.internal:4317",
		"samplingRatePerMillion: 1000000",
	} {
		if !strings.Contains(generated, want) {
			t.Errorf("generated kind config missing %q:\n%s", want, generated)
		}
	}
}

func TestHelmInstallSmokePassesRunIdentityToGateway(t *testing.T) {
	var command []string
	run := func(name string, args ...string) ([]byte, error) {
		command = append([]string{name}, args...)
		return nil, nil
	}
	telemetry := helmTelemetryIdentity{
		OTLPEndpoint: "host.docker.internal:4317",
		RunID:        "run-123",
		Commit:       "abc123",
	}
	image := "declarative-agents/agent-core:0123456789ab"
	if err := helmInstallSmokeWithRunner("/chart", image, telemetry, run); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(command, " ")
	for _, want := range []string{
		"collector.externalOTLPEndpoint=host.docker.internal:4317",
		"collector.integrationResource.target=integration:helmSmoke",
		"collector.integrationResource.commit=abc123",
		"collector.integrationResource.runID=run-123",
		"image.repository=declarative-agents/agent-core",
		"image.tag=0123456789ab",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("helm command missing %q: %s", want, joined)
		}
	}
}

func TestCollectSharedMetricsEvidenceRetainsAgentAndDolt(t *testing.T) {
	started := time.Now().Add(-time.Minute)
	const runID = "run-123"
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/query/metrics" {
			_ = json.NewEncoder(w).Encode(map[string]any{"metrics": []map[string]any{
				{"name": "dispatch_count", "services": []string{"chatbot", "rag0"}},
				{"name": "dss_concurrent_queries", "services": []string{"dolt"}},
			}})
			return
		}
		// The get response carries the run id in a record's resource attrs.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metric_name": strings.TrimPrefix(r.URL.Path, "/query/metrics/"),
			"records": []map[string]any{{"metric": map[string]any{
				"resource": []map[string]any{{"key": "test_run_id", "value": runID}},
			}}},
			"record_count": 1,
		})
	}))
	defer collector.Close()

	evidence, err := collectSharedMetricsEvidence(collector.URL, helmTelemetryIdentity{
		RunID: runID, Started: started,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(evidence.AgentMetrics, "dispatch_count") ||
		!containsString(evidence.DoltMetrics, "dss_concurrent_queries") {
		t.Fatalf("metric evidence missing: agent=%v dolt=%v",
			evidence.AgentMetrics, evidence.DoltMetrics)
	}
}

func TestCollectSharedMetricsEvidenceRejectsMissingRunID(t *testing.T) {
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/query/metrics" {
			_ = json.NewEncoder(w).Encode(map[string]any{"metrics": []map[string]any{
				{"name": "dispatch_count", "services": []string{"chatbot", "rag0"}},
				{"name": "dss_rows", "services": []string{"dolt"}},
			}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metric_name": "dispatch_count",
			"records":     []map[string]any{{"metric": map[string]any{"resource": []map[string]any{}}}},
		})
	}))
	defer collector.Close()

	_, err := collectSharedMetricsEvidence(collector.URL, helmTelemetryIdentity{RunID: "run-xyz"})
	if err == nil || !strings.Contains(err.Error(), "run id") {
		t.Fatalf("expected a missing-run-id error, got %v", err)
	}
}

func TestAssertSmokeChatServedRejectsEmptyAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"answer":"   "}`))
	}))
	defer srv.Close()

	if err := assertSmokeChatServed(srv.URL); err == nil {
		t.Fatal("assertSmokeChatServed should reject an empty answer")
	}
}

func TestAssertSmokeChatServedAcceptsAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"answer":"The Northwind array is rated at 55 MW."}`))
	}))
	defer srv.Close()

	if err := assertSmokeChatServed(srv.URL); err != nil {
		t.Fatalf("assertSmokeChatServed rejected a served answer: %v", err)
	}
}

func TestHelmSmokeSkipReasonMissingBinary(t *testing.T) {
	// With an empty PATH none of the required binaries resolve, so the smoke test
	// records a skip for the first missing tool rather than attempting a run.
	t.Setenv("PATH", "")
	if reason := helmSmokeSkipReason(t.TempDir(), t.TempDir()); reason == "" {
		t.Fatal("helmSmokeSkipReason should report a skip when required binaries are absent")
	}
}

func TestChatbotRolloutDrainsActiveRequests(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	out, err := exec.Command("helm", "template", "t", findChartDir(t)).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	render := string(out)
	for _, want := range []string{
		"maxUnavailable: 0",
		"maxSurge: 1",
		"terminationGracePeriodSeconds: 150",
		"preStop:",
		`--post-data='{"reason":"kubernetes rollout","status":"success"}'`,
		`"$base/exit"`,
		"timeout: 135s",
		"drain_policy: drain_then_stop",
	} {
		if !strings.Contains(render, want) {
			t.Errorf("rendered chatbot rollout contract missing %q", want)
		}
	}
}
