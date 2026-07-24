// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
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
		{"declarative-agents/agent-core:smoke", "declarative-agents/agent-core", "smoke"},
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
	if err := helmInstallSmokeWithRunner("/chart", helmImage, telemetry, run); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(command, " ")
	for _, want := range []string{
		"collector.externalOTLPEndpoint=host.docker.internal:4317",
		"collector.integrationResource.target=integration:helmSmoke",
		"collector.integrationResource.commit=abc123",
		"collector.integrationResource.runID=run-123",
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
