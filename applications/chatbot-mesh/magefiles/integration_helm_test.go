// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
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

func TestKindDependencyImagesCoverEveryExternalPodImage(t *testing.T) {
	chartDir := filepath.Join("..", "helm")
	smoke, err := smokeDependencyImages(chartDir)
	if err != nil {
		t.Fatal(err)
	}
	wantSmoke := []string{
		"otel/opentelemetry-collector-contrib:0.127.0",
		"chromadb/chroma:1.5.3",
		"dolthub/dolt-sql-server:latest",
		"rancher/kubectl:v1.31.4",
		"busybox:1.36",
	}
	if !slices.Equal(smoke, wantSmoke) {
		t.Fatalf("smoke dependencies = %v, want %v", smoke, wantSmoke)
	}
	swap, err := swapDependencyImages(chartDir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(swap, wantSmoke) {
		t.Fatalf("swap dependencies = %v, want smoke closure %v", swap, wantSmoke)
	}
}

func TestHermeticDependencyPullUsesExactOllamaDigest(t *testing.T) {
	var calls []string
	run := func(name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	images := []string{"busybox:1.36", helmLLMOllamaImage}
	if err := pullIntegrationDependencyImages("helmLLMTier", images, run); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"docker pull --platform linux/" + runtime.GOARCH + " busybox:1.36",
		"docker pull --platform linux/" + runtime.GOARCH + " " + helmLLMOllamaSourceImage,
	}
	if !slices.Equal(calls, want) {
		t.Fatalf("dependency delivery calls:\n got: %v\nwant: %v", calls, want)
	}
	dockerfile := trustedOllamaDockerfile()
	for _, want := range []string{
		"FROM " + helmLLMOllamaSourceImage,
		">> /etc/ssl/certs/ca-certificates.crt",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("trusted Ollama Dockerfile missing %q:\n%s", want, dockerfile)
		}
	}
	for _, forbidden := range []string{"insecure", "tls-verify=false", "GIT_SSL_NO_VERIFY"} {
		if strings.Contains(strings.ToLower(dockerfile), strings.ToLower(forbidden)) {
			t.Errorf("trusted Ollama Dockerfile disables TLS with %q:\n%s",
				forbidden, dockerfile)
		}
	}
}

func TestHelmFailureEvidenceIsBoundedAndNamesRootCauses(t *testing.T) {
	t.Run("diagnostic causes", func(t *testing.T) {
		run := func(_ context.Context, name string, args ...string) ([]byte, error) {
			command := name + " " + strings.Join(args, " ")
			switch {
			case strings.Contains(command, "get events"):
				return []byte("FailedScheduling: insufficient memory\nFailedMount: PVC is Pending\nErrImagePull: x509 certificate signed by unknown authority"), nil
			case strings.Contains(command, `-o json`):
				return []byte(`{"status":{"initContainerStatuses":[{"name":"wait-for-llm-models","state":{"waiting":{"reason":"PodInitializing"}}}],"containerStatuses":[{"name":"ollama","state":{"waiting":{"reason":"ImagePullBackOff"}}}]}}`), nil
			case strings.Contains(command, "describe pods"):
				return []byte("Readiness probe failed: connection refused"), nil
			case strings.Contains(command, "logs"):
				return []byte("pulling qwen2.5:0.5b\nError: model pull failed"), nil
			default:
				return []byte("diagnostic output"), nil
			}
		}
		dir := t.TempDir()
		report := captureHelmFailureDiagnostics(
			dir, helmLLMRelease, run, time.Second)
		for _, want := range []string{
			"bounded diagnostics",
			"FailedScheduling",
			"PVC is Pending",
			"ErrImagePull",
			"ImagePullBackOff",
			"Readiness probe failed",
			"model pull failed",
			"initContainerStatuses",
		} {
			if !strings.Contains(report, want) {
				t.Errorf("failure evidence missing %q:\n%s", want, report)
			}
		}
		data, err := os.ReadFile(filepath.Join(dir, "bounded-diagnostics.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "container and init status") {
			t.Fatalf("persisted evidence omits container/init status:\n%s", data)
		}
	})

	t.Run("overall deadline", func(t *testing.T) {
		calls := 0
		run := func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			calls++
			<-ctx.Done()
			return nil, ctx.Err()
		}
		started := time.Now()
		report := collectHelmFailureDiagnostics(
			helmSwapRelease, run, 5*time.Millisecond)
		if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
			t.Fatalf("diagnostics took %s, want bounded completion", elapsed)
		}
		if calls != len(helmFailureDiagnosticCommands(helmSwapRelease)) {
			t.Fatalf("diagnostic calls = %d, want %d",
				calls, len(helmFailureDiagnosticCommands(helmSwapRelease)))
		}
		if !strings.Contains(report, "deadline exceeded") {
			t.Fatalf("bounded diagnostics omit deadline evidence:\n%s", report)
		}
	})
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
	// Agent mode relays both signals through the declarative collector to the host
	// ingress and tags each agent's telemetry with the integration run identity via
	// OTEL_RESOURCE_ATTRIBUTES; metrics ride the same agent collector as traces
	// (GH-1207, GH-1366).
	for _, want := range []string{
		"COLLECTOR_RELAY_ENDPOINT",
		`value: "host.docker.internal:4317"`,
		"COLLECTOR_MODE",
		"OTEL_RESOURCE_ATTRIBUTES",
		"test.repository=Nokia-Bell-Labs/declarative-agents",
		"test.module=applications/chatbot-mesh",
		"test.target=integration:helm",
		"vcs.ref.head.revision=unknown",
		"test.run.id=local-kind",
		"CHROMA_OPEN_TELEMETRY__ENDPOINT",
		`http://t-chatbot-mesh-collector:4317`,
		"CHROMA_OPEN_TELEMETRY__SERVICE_NAME",
		`value: "rag0-chroma"`,
	} {
		if !strings.Contains(render, want) {
			t.Errorf("kind collector render missing %q", want)
		}
	}
	// The agent collector owns metric intake, so no contrib collector, no separate
	// metrics gateway, and no in-cluster Dolt Prometheus scrape are rendered.
	for _, notWant := range []string{
		"opentelemetry-collector-contrib",
		"collector-metrics",
		"prometheus/dolt:",
		"otlp/external:",
		"resource/integration:",
	} {
		if strings.Contains(render, notWant) {
			t.Errorf("kind agent-mode render unexpectedly contains %q", notWant)
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
	chart, chartArchive, assets := stageThinIntegrationChart(t, helmRelease)
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
	if err := helmInstallSmokeWithRunner(
		chart, chartArchive, image, telemetry, assets, run); err != nil {
		t.Fatal(err)
	}
	valueArgs := helmSmokeValueArgs(chart, image, telemetry, assets)
	wantCommand := append([]string{"helm", "install", helmRelease, chart}, valueArgs...)
	wantCommand = append(wantCommand, "--wait", "--timeout", helmInstallTimeout.String())
	if !slices.Equal(command, wantCommand) {
		t.Fatalf("helm command:\n got: %#v\nwant: %#v", command, wantCommand)
	}
	measured, err := measureHelmReleaseBudget(
		helmRelease, chart, chartArchive, valueArgs)
	if err != nil {
		t.Fatalf("smoke release budget: %v", err)
	}
	if measured.ProjectedSecretBytes > helmReleaseBudget {
		t.Fatalf("smoke projected release = %d, budget = %d",
			measured.ProjectedSecretBytes, helmReleaseBudget)
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
	assertExternalAssetArgs(t, joined, assets)
}

func TestHelmSmokeInstallReturnsCapturedOutput(t *testing.T) {
	chart, chartArchive, assets := stageThinIntegrationChart(t, helmRelease)
	err := helmInstallSmokeWithRunner(
		chart,
		chartArchive,
		"declarative-agents/agent-core:smoke-output",
		helmTelemetryIdentity{
			OTLPEndpoint: "host.docker.internal:4317",
			RunID:        "run-output",
			Commit:       "abc123",
		},
		assets,
		func(string, ...string) ([]byte, error) {
			return []byte("controlled smoke Helm output"), errors.New("controlled failure")
		},
	)
	if err == nil || !strings.Contains(err.Error(), "controlled smoke Helm output") {
		t.Fatalf("smoke install error = %v, want captured Helm output", err)
	}
}

func TestHelmSwapInstallAndUpgradeUseThinReleaseArgs(t *testing.T) {
	chart, chartArchive, assets := stageThinIntegrationChart(t, helmSwapRelease)
	image := "declarative-agents/agent-core:swap-budget"
	tests := []struct {
		verb  string
		extra []string
	}{
		{
			verb: "install",
			extra: []string{
				"--set", "llm.externalURL=http://host.docker.internal:12345",
				"--set", "llm.port=12345",
				"--set", "ragUnits[1].name=rag1",
				"--set", "ragUnits[1].description=Second integration corpus",
				"--set", "ragUnits[1].collection=corpus1",
				"--set", "ragUnits[1].embeddingModel=qwen3-embedding:8b",
				"--set", "ragUnits[1].replicas=1",
				"--set", "ragUnits[2].name=rag2",
				"--set", "ragUnits[2].description=Third integration corpus",
				"--set", "ragUnits[2].collection=corpus2",
				"--set", "ragUnits[2].embeddingModel=qwen3-embedding:8b",
				"--set", "ragUnits[2].replicas=1",
			},
		},
		{
			verb: "upgrade",
			extra: []string{
				"--set", "llm.externalURL=http://host.docker.internal:12345",
				"--set", "llm.port=12345",
				"--set", "ragUnits[1].name=rag2",
				"--set", "ragUnits[1].description=Replacement integration corpus",
				"--set", "ragUnits[1].collection=corpus2",
				"--set", "ragUnits[1].embeddingModel=qwen3-embedding:8b",
				"--set", "ragUnits[1].replicas=1",
				"--set", "ragUnits[2].name=rag4",
				"--set", "ragUnits[2].description=Additional integration corpus",
				"--set", "ragUnits[2].collection=corpus4",
				"--set", "ragUnits[2].embeddingModel=qwen3-embedding:8b",
				"--set", "ragUnits[2].replicas=1",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.verb, func(t *testing.T) {
			var command []string
			err := helmSwapDeployWithRunner(
				chart, chartArchive, image, tc.verb, tc.extra, assets,
				func(name string, args ...string) ([]byte, error) {
					command = append([]string{name}, args...)
					return nil, nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			valueArgs := helmSwapValueArgs(chart, image, tc.extra, assets)
			want := append([]string{"helm", tc.verb, helmSwapRelease, chart}, valueArgs...)
			want = append(want, "--wait", "--timeout", helmInstallTimeout.String())
			if !slices.Equal(command, want) {
				t.Fatalf("%s command:\n got: %#v\nwant: %#v", tc.verb, command, want)
			}
			measured, err := measureHelmReleaseBudget(
				helmSwapRelease, chart, chartArchive, valueArgs)
			if err != nil {
				t.Fatalf("%s release budget: %v", tc.verb, err)
			}
			if measured.ProjectedSecretBytes > helmReleaseBudget {
				t.Fatalf("%s projected release = %d, budget = %d",
					tc.verb, measured.ProjectedSecretBytes, helmReleaseBudget)
			}
			assertExternalAssetArgs(t, strings.Join(command, " "), assets)
		})
	}
}

func TestHelmSwapReturnsCapturedOutput(t *testing.T) {
	chart, chartArchive, assets := stageThinIntegrationChart(t, helmSwapRelease)
	err := helmSwapDeployWithRunner(
		chart,
		chartArchive,
		"declarative-agents/agent-core:swap-output",
		"upgrade",
		[]string{"--set", "llm.externalURL=http://host.docker.internal:12345"},
		assets,
		func(string, ...string) ([]byte, error) {
			return []byte("controlled swap Helm output"), errors.New("controlled failure")
		},
	)
	if err == nil || !strings.Contains(err.Error(), "controlled swap Helm output") {
		t.Fatalf("swap upgrade error = %v, want captured Helm output", err)
	}
}

func stageThinIntegrationChart(
	t *testing.T,
	releaseName string,
) (string, string, []externalUIAsset) {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	staged, cleanupChart, err := stageSmokeChart(chartDir, filepath.Dir(chartDir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanupChart)
	assets, cleanupAssets, err := externalizeUIAssets(staged, releaseName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanupAssets)
	archive, cleanupArchive, err := packageApplierChart(staged)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanupArchive)
	return staged, archive, assets
}

func assertExternalAssetArgs(t *testing.T, command string, assets []externalUIAsset) {
	t.Helper()
	for _, asset := range assets {
		for _, want := range []string{
			asset.Component + ".uiArchiveConfigMap=" + asset.ConfigMapName,
			asset.Component + ".uiArchiveChecksum=" + asset.Checksum,
		} {
			if !strings.Contains(command, want) {
				t.Errorf("helm command missing external asset argument %q: %s", want, command)
			}
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

// TestCollectSharedMetricsEvidenceAgentModeNeedsNoDolt proves the GH-1366
// retirement: with the contrib Dolt scrape gone in agent mode, evidence passes on
// the agent metric alone and surfaces no Dolt metrics.
func TestCollectSharedMetricsEvidenceAgentModeNeedsNoDolt(t *testing.T) {
	started := time.Now().Add(-time.Minute)
	const runID = "run-agent"
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/query/metrics" {
			_ = json.NewEncoder(w).Encode(map[string]any{"metrics": []map[string]any{
				{"name": "dispatch_count", "services": []string{"chatbot", "rag0"}},
			}})
			return
		}
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
		t.Fatalf("agent-mode metric evidence should not require Dolt metrics: %v", err)
	}
	if !containsString(evidence.AgentMetrics, "dispatch_count") {
		t.Fatalf("missing agent metric: %v", evidence.AgentMetrics)
	}
	if len(evidence.DoltMetrics) != 0 {
		t.Fatalf("agent mode should surface no Dolt metrics, got %v", evidence.DoltMetrics)
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
