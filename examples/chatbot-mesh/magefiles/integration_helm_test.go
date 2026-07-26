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
	if !strings.Contains(render, "endpoint: t-chatbot-mesh-jaeger:4317") {
		t.Fatal("default collector does not export traces to embedded Jaeger")
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
		"endpoint: t-chatbot-mesh-jaeger:4317",
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

func TestSharedTraceCountFiltersByRunID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("service") != "chatbot" ||
			!strings.Contains(r.URL.Query().Get("tags"), `"test.run.id":"run-123"`) {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"data":[{"traceID":"abc"}]}`))
	}))
	defer server.Close()
	count, err := sharedTraceCount(server.URL, "chatbot", "run-123")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("trace count = %d, want 1", count)
	}
}

func TestCollectSharedTelemetryEvidenceIdentifiesSlowComponent(t *testing.T) {
	started := time.Now().Add(-time.Minute)
	jaeger := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		service := r.URL.Query().Get("service")
		duration := int64(1000)
		tags := []map[string]any{}
		if service == "rag0-chroma" {
			duration = 9000
		}
		if service == "chatbot" {
			tags = []map[string]any{
				{"key": "gen_ai.operation.name", "value": "chat"},
				{"key": "gen_ai.provider.name", "value": "ollama"},
				{"key": "gen_ai.request.model", "value": "qwen2.5:3b"},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{
			"traceID": service + "-trace",
			"spans": []any{map[string]any{
				"operationName": service + ".operation",
				"processID":     "p1",
				"duration":      duration,
				"tags":          tags,
			}},
			"processes": map[string]any{"p1": map[string]any{"serviceName": service}},
		}}})
	}))
	defer jaeger.Close()

	prometheus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		selector := r.URL.Query().Get("match[]")
		var series []map[string]string
		switch {
		case strings.HasPrefix(selector, "target_info"):
			series = []map[string]string{{"job": "chatbot"}, {"job": "rag0"}, {"job": "dolt"}}
		case strings.HasPrefix(selector, "dispatch_count_total"):
			series = []map[string]string{
				{"__name__": "dispatch_count_total", "job": "chatbot"},
				{"__name__": "dispatch_count_total", "job": "rag0"},
			}
		default:
			series = []map[string]string{{"__name__": "dss_concurrent_queries", "job": "dolt"}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": series})
	}))
	defer prometheus.Close()

	evidence, err := collectSharedTelemetryEvidence(jaeger.URL, prometheus.URL, helmTelemetryIdentity{
		RunID: "run-123", Started: started,
	})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.SlowestService != "rag0-chroma" ||
		evidence.SlowestOperation != "rag0-chroma.operation" {
		t.Fatalf("slowest evidence = %s/%s, want rag0-chroma/rag0-chroma.operation",
			evidence.SlowestService, evidence.SlowestOperation)
	}
	if !containsString(evidence.AgentMetrics, "dispatch_count_total") ||
		!containsString(evidence.DoltMetrics, "dss_concurrent_queries") {
		t.Fatalf("metric evidence missing: agent=%v dolt=%v",
			evidence.AgentMetrics, evidence.DoltMetrics)
	}
}

func TestJaegerAgentServicesExcludesJaeger(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/services" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"data":["chatbot","rag0","jaeger-all-in-one"]}`))
	}))
	defer srv.Close()

	n, services, err := jaegerAgentServices(srv.URL)
	if err != nil {
		t.Fatalf("jaegerAgentServices: %v", err)
	}
	if n != 2 {
		t.Fatalf("agent service count = %d, want 2 (services=%v)", n, services)
	}
	for _, s := range services {
		if s == "jaeger-all-in-one" {
			t.Fatalf("jaeger internal service should be excluded, got %v", services)
		}
	}
}

func TestAssertSmokeSpansBelowThreshold(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":["chatbot","jaeger"]}`))
	}))
	defer srv.Close()

	// Only one agent service is present, so the >=2 assertion must fail after a
	// short retry budget rather than hang.
	if err := assertSmokeSpans(srv.URL, 2, 200*time.Millisecond); err == nil {
		t.Fatal("assertSmokeSpans should fail when fewer than minServices agent services report")
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
