// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"gopkg.in/yaml.v3"
)

const (
	helmRelease         = "smoke"
	helmKindCluster     = "da-chatbot-mesh-smoke"
	helmImageRepository = "declarative-agents/agent-core"

	helmChatURL          = "http://127.0.0.1:18080/api/v1/chat"
	helmHealthURL        = "http://127.0.0.1:18081/api/lifecycle/health"
	helmInstallTimeout   = 5 * time.Minute
	helmImageLoadTimeout = 3 * time.Minute
	helmClusterWait      = 120 * time.Second
	helmReadyTimeout     = 90 * time.Second
	helmSpanTimeout      = 60 * time.Second

	helmFailureDiagnosticsTimeout = 30 * time.Second
	helmEvidenceCommandTimeout    = 10 * time.Second
)

type chatbotIntegrationImages struct {
	Revision string
	Runtime  string
	Applier  string
}

func resolveChatbotIntegrationImages(repoRoot string) (chatbotIntegrationImages, error) {
	commit := gitCommit(repoRoot)
	runtimeImage, revision, err := kindrig.CommitImage(helmImageRepository, commit)
	if err != nil {
		return chatbotIntegrationImages{}, fmt.Errorf("resolve runtime image revision: %w", err)
	}
	applierImage, _, err := kindrig.CommitImage(applierLiveImageRepository, commit)
	if err != nil {
		return chatbotIntegrationImages{}, fmt.Errorf("resolve applier image revision: %w", err)
	}
	return chatbotIntegrationImages{
		Revision: revision, Runtime: runtimeImage, Applier: applierImage,
	}, nil
}

// applicationChartDir returns the chatbot-mesh Helm chart under the application, which
// ships with the application rather than as a sibling deploy directory.
func applicationChartDir(profilesRoot string) string {
	return filepath.Join(profilesRoot, "helm")
}

// helmKindConfig is the checked-in cluster configuration the helm scenarios
// share; it pins the node image so every machine creates the same cluster
// (eng01). It sits in the source chart's ci directory beside the kind values,
// not in the staged copy, so staging cannot drift it.
func helmKindConfig(chartDir string) string {
	return filepath.Join(chartDir, "ci", "kind-config.yaml")
}

func loadKindImage(cluster, image string) error {
	ctx, cancel := context.WithTimeout(context.Background(), helmImageLoadTimeout)
	defer cancel()
	return kindrig.LoadImage(ctx, kindrig.DefaultRunContext, cluster, image)
}

// bindClusterKubeconfig pins every subsequent Helm/kubectl/port-forward
// subprocess to the named kind cluster's own kubeconfig. Because EnsureCluster
// reuses a pre-existing cluster without switching contexts, and the raw kubectl
// and helm invocations here inherit the ambient environment, an unrelated
// current context could otherwise be the target of a live mutation (GH-1341).
// The returned cleanup restores the prior KUBECONFIG and removes the temp file.
func bindClusterKubeconfig(cluster string) (func(), error) {
	path, cleanup, err := kindrig.Kubeconfig(kindrig.CaptureRun, cluster)
	if err != nil {
		return nil, err
	}
	restore := kindrig.BindKubeconfig(path)
	return func() {
		restore()
		cleanup()
	}, nil
}

// HelmSmoke deploys the chatbot-mesh chart on a disposable kind cluster with the
// ci values and proves the mesh stands up, serves a chat turn, and exports spans
// from more than one service. It gates on docker, kind, helm, and kubectl and on
// an Ollama with the chatbot's configured models, recording a skip reason for
// each missing dependency rather than failing. Teardown (kind delete) runs in
// all paths.
//
// Scope: this is the deploy smoke bar (srd003 R1/R5, uc rel03.0 S1). The span
// assertion needs each agent to report a distinct service.name, which the chart
// wires (chatbot and each rag unit) so the collector spool surfaces the mesh as
// more than one service.
func (Integration) HelmSmoke() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := demoCoreRoot(profilesRoot)
	chartDir := applicationChartDir(profilesRoot)
	if err := requireProfilePaths(profilesRoot,
		"agents/chatbot/profile.yaml", "agents/chatbot/rest.yaml",
		"agents/rag-server/profile.yaml",
	); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(chartDir, "Chart.yaml")); err != nil {
		return fmt.Errorf("chatbot-mesh chart not found at %s: %w", chartDir, err)
	}
	if reason := helmSmokeSkipReason(profilesRoot, coreRoot); reason != "" {
		fmt.Printf("SKIP helmSmoke: %s\n", reason)
		return nil
	}
	return runHelmSmoke(coreRoot, profilesRoot, chartDir)
}

// helmSmokeSkipReason reports why the smoke test cannot run, or "" when every
// dependency is present. Missing tooling, no agent-core checkout, and an Ollama
// without the configured models each yield a recorded skip rather than a failure.
func helmSmokeSkipReason(profilesRoot, coreRoot string) string {
	for _, bin := range []string{"docker", "kind", "helm", "kubectl"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Sprintf("%s not found on PATH", bin)
		}
	}
	if !agentCoreAvailable(coreRoot) {
		return fmt.Sprintf("agent-core checkout not found at %s (set core_root in demo.yaml)", coreRoot)
	}
	return chatbotOllamaSkipReason(profilesRoot)
}

type helmTelemetryIdentity struct {
	OTLPEndpoint string
	RunID        string
	Commit       string
	Started      time.Time
}

func newHelmTelemetryIdentity(repoRoot string) helmTelemetryIdentity {
	runID := generatedRunID("integration:helmSmoke")
	commit := gitCommit(repoRoot)
	endpoint := demoIntegrationOTLPEndpoint()
	if endpoint == "" {
		endpoint = "host.docker.internal:" + demoObservability().OTELGRPCPort
	} else {
		endpoint = strings.Replace(endpoint, "127.0.0.1", "host.docker.internal", 1)
		endpoint = strings.Replace(endpoint, "localhost", "host.docker.internal", 1)
	}
	return helmTelemetryIdentity{
		OTLPEndpoint: endpoint, RunID: runID, Commit: commit, Started: time.Now(),
	}
}

func collectorQueryBase() string {
	return "http://127.0.0.1:" + demoObservability().QueryPort
}

func collectorControlBase() string {
	return "http://127.0.0.1:" + demoObservability().ControlPort
}

func requireSharedObservability(timeout time.Duration) error {
	checks := []string{
		collectorControlBase() + "/api/lifecycle/health",
		collectorQueryBase() + "/query/traces",
		collectorQueryBase() + "/query/metrics",
	}
	for _, endpoint := range checks {
		if err := waitHTTPStatus(endpoint, http.StatusOK, timeout); err != nil {
			return err
		}
	}
	return nil
}

func stageTelemetryKindConfig(basePath string, telemetry helmTelemetryIdentity) (string, func(), error) {
	data, err := os.ReadFile(basePath)
	if err != nil {
		return "", nil, fmt.Errorf("read kind config: %w", err)
	}
	var config struct {
		Kind                 string           `yaml:"kind"`
		APIVersion           string           `yaml:"apiVersion"`
		Nodes                []map[string]any `yaml:"nodes"`
		KubeadmConfigPatches []string         `yaml:"kubeadmConfigPatches,omitempty"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", nil, fmt.Errorf("parse kind config: %w", err)
	}
	if len(config.Nodes) == 0 {
		return "", nil, fmt.Errorf("kind config has no nodes")
	}
	dir, err := os.MkdirTemp("", "chatbot-mesh-kind-telemetry-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	tracingPath := filepath.Join(dir, "tracing.yaml")
	tracing := fmt.Sprintf(`apiVersion: apiserver.config.k8s.io/v1beta1
kind: TracingConfiguration
endpoint: %s
samplingRatePerMillion: 1000000
`, telemetry.OTLPEndpoint)
	if err := os.WriteFile(tracingPath, []byte(tracing), 0o644); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write API-server tracing config: %w", err)
	}
	mounts, _ := config.Nodes[0]["extraMounts"].([]any)
	config.Nodes[0]["extraMounts"] = append(mounts, map[string]any{
		"hostPath": tracingPath, "containerPath": "/etc/kubernetes/tracing.yaml",
		"readOnly": true,
	})
	resourceAttrs := integrationResourceAttributes(
		"integration:helmSmoke", telemetry.RunID, telemetry.Commit)
	config.KubeadmConfigPatches = append(config.KubeadmConfigPatches,
		fmt.Sprintf(`apiVersion: kubeadm.k8s.io/v1beta4
kind: ClusterConfiguration
apiServer:
  extraArgs:
    - name: tracing-config-file
      value: /etc/kubernetes/tracing.yaml
  extraEnvs:
    - name: OTEL_RESOURCE_ATTRIBUTES
      value: %q
  extraVolumes:
    - name: tracing-config
      hostPath: /etc/kubernetes/tracing.yaml
      mountPath: /etc/kubernetes/tracing.yaml
      readOnly: true
      pathType: File
`, resourceAttrs),
		fmt.Sprintf(`apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
tracing:
  endpoint: %s
  samplingRatePerMillion: 1000000
`, telemetry.OTLPEndpoint),
	)
	generated, err := yaml.Marshal(config)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("marshal kind tracing config: %w", err)
	}
	generatedPath := filepath.Join(dir, "kind-config.yaml")
	if err := os.WriteFile(generatedPath, generated, 0o644); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write generated kind config: %w", err)
	}
	return generatedPath, cleanup, nil
}

func runHelmSmoke(coreRoot, profilesRoot, chartDir string) (result error) {
	images, err := resolveChatbotIntegrationImages(profilesRoot)
	if err != nil {
		return err
	}
	telemetry := newHelmTelemetryIdentity(profilesRoot)
	if err := requireSharedObservability(helmReadyTimeout); err != nil {
		return fmt.Errorf("shared observability stack is required: %w", err)
	}
	fmt.Printf("helmSmoke: building runtime image %s from %s\n", images.Runtime, coreRoot)
	if err := buildSmokeRuntimeImage(coreRoot, images.Runtime); err != nil {
		return err
	}
	stagedChart, cleanupChart, err := stageSmokeChart(chartDir, profilesRoot)
	if err != nil {
		return err
	}
	defer cleanupChart()
	assets, cleanupAssets, err := externalizeUIAssets(stagedChart, helmRelease)
	if err != nil {
		return err
	}
	defer cleanupAssets()
	chartArchive, cleanupArchive, err := packageApplierChart(stagedChart)
	if err != nil {
		return err
	}
	defer cleanupArchive()
	dependencyImages, err := smokeDependencyImages(chartDir)
	if err != nil {
		return err
	}
	for _, image := range dependencyImages {
		fmt.Printf("helmSmoke: preloading dependency image %s\n", image)
		if out, pullErr := exec.Command("docker", "pull", "--platform", "linux/"+runtime.GOARCH, image).CombinedOutput(); pullErr != nil {
			return fmt.Errorf("pull smoke dependency %s: %w: %s", image, pullErr, strings.TrimSpace(string(out)))
		}
	}

	kindConfig, cleanupKindConfig, err := stageTelemetryKindConfig(helmKindConfig(chartDir), telemetry)
	if err != nil {
		return err
	}
	defer cleanupKindConfig()
	cluster, err := kindrig.EnsureCluster(kindrig.DefaultRun, helmKindCluster, kindConfig, helmClusterWait)
	if err != nil {
		return err
	}
	released := false
	defer func() {
		if !released {
			cluster.ReleaseAfter(kindrig.DefaultRun, result != nil, kindrig.FailureEvidence{
				Directory: filepath.Join(
					profilesRoot, "build", "kind-evidence",
					helmKindCluster+"-"+images.Revision+"-"+
						time.Now().UTC().Format("20060102T150405.000000000Z")),
				Namespaces: []string{"default"},
				Run:        kindrig.CommandRunner(runHelmSmokeCommand),
			})
		}
	}()

	unbindKubeconfig, err := bindClusterKubeconfig(helmKindCluster)
	if err != nil {
		return err
	}
	defer unbindKubeconfig()

	if err := provisionExternalUIAssets(assets); err != nil {
		return err
	}
	if err := loadKindImage(helmKindCluster, images.Runtime); err != nil {
		return err
	}
	for _, image := range dependencyImages {
		if err := loadSmokeDependencyImage(helmKindCluster, image); err != nil {
			return err
		}
	}
	if err := helmInstallSmoke(stagedChart, chartArchive, images.Runtime, telemetry, assets); err != nil {
		return err
	}
	if err := assertHelmIntegrationRelease(helmRelease, chartArchive, assets, 1); err != nil {
		return err
	}
	if err := assertExternalUIAssetsMounted(helmRelease, assets, helmReadyTimeout); err != nil {
		return err
	}

	stop, err := kubectlPortForward("svc/"+helmRelease+"-chatbot-mesh-chatbot", 18080, 18081)
	if err != nil {
		return err
	}
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	if err := waitHTTPStatus(helmHealthURL, http.StatusOK, helmReadyTimeout); err != nil {
		return fmt.Errorf("chatbot control health not ready: %w", err)
	}
	// Checked before the turn that is supposed to produce its spans, so a
	// collector that cannot start is reported as itself (GH-736).
	if err := assertCollectorAvailable(helmRelease, helmReadyTimeout); err != nil {
		return err
	}
	if err := assertSmokeChatServed(helmChatURL); err != nil {
		return err
	}
	if err := assertCollectorSpoolIdentity(helmRelease, telemetry.RunID,
		[]string{"chatbot", "rag0"}, helmSpanTimeout); err != nil {
		return err
	}
	if err := assertNoCollectorSelfIngest(helmRelease, telemetry.RunID); err != nil {
		return err
	}
	if err := verifySharedMetricsEvidence(
		collectorQueryBase(), telemetry, helmSpanTimeout); err != nil {
		return err
	}
	stop()
	stop = nil
	cluster.ReleaseAfter(kindrig.DefaultRun, false, kindrig.FailureEvidence{})
	released = true
	if err := verifySharedMetricsEvidence(
		collectorQueryBase(), telemetry, helmSpanTimeout); err != nil {
		return err
	}

	fmt.Printf("integration:helmSmoke PASS - revision %s spool and collector retained agent and Dolt metric evidence for run %s after cluster cleanup\n",
		images.Revision, telemetry.RunID)
	return nil
}

type smokeImage struct {
	Repository string `yaml:"repository"`
	Tag        string `yaml:"tag"`
}

const smokeUtilityImage = "busybox:1.36"

// smokeDependencyImages returns every external image used by the kind smoke
// topology when its Ollama tier is disabled. Keeping the complete set on the
// host-pull/kind-load path prevents kind's containerd from reaching a registry
// directly, which fails behind TLS-intercepting proxies that only the host
// container engine trusts (GH-1321).
func smokeDependencyImages(chartDir string) ([]string, error) {
	var values struct {
		Collector struct {
			Image smokeImage `yaml:"image"`
		} `yaml:"collector"`
		Chroma struct {
			Image smokeImage `yaml:"image"`
		} `yaml:"chroma"`
		Dolt struct {
			Image smokeImage `yaml:"image"`
		} `yaml:"dolt"`
		Observer struct {
			Proxy struct {
				Image smokeImage `yaml:"image"`
			} `yaml:"proxy"`
		} `yaml:"observer"`
	}
	if err := readIntegrationYAML(filepath.Join(chartDir, "values.yaml"), "chart values", &values); err != nil {
		return nil, err
	}
	refs := []smokeImage{
		values.Collector.Image,
		values.Chroma.Image,
		values.Dolt.Image,
		values.Observer.Proxy.Image,
	}
	images := make([]string, 0, len(refs)+1)
	for _, image := range refs {
		if image.Repository == "" || image.Tag == "" {
			return nil, fmt.Errorf("smoke dependency image requires repository and tag")
		}
		images = append(images, image.Repository+":"+image.Tag)
	}
	return append(images, smokeUtilityImage), nil
}

// swapDependencyImages deliberately closes over the same external pod-image set
// as helmSmoke. Swap changes topology, not its dependency boundary.
func swapDependencyImages(chartDir string) ([]string, error) {
	return smokeDependencyImages(chartDir)
}

func pullIntegrationDependencyImages(
	scenario string,
	images []string,
	run helmLLMCommandRunner,
) error {
	for _, image := range images {
		source := image
		if image == helmLLMOllamaImage {
			source = helmLLMOllamaSourceImage
		}
		fmt.Printf("%s: preloading dependency image %s\n", scenario, source)
		out, err := run("docker", "pull", "--platform", "linux/"+runtime.GOARCH, source)
		if err != nil {
			return fmt.Errorf("pull %s dependency %s: %w: %s",
				scenario, source, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func buildTrustedOllamaImage(image string) error {
	caBundle, err := hostTrustedCABundle()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "chatbot-mesh-ollama-trust-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := os.WriteFile(
		filepath.Join(dir, "host-ca.pem"), caBundle, 0o644); err != nil {
		return fmt.Errorf("write trusted Ollama CA bundle: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "Dockerfile"),
		[]byte(trustedOllamaDockerfile()), 0o644); err != nil {
		return fmt.Errorf("write trusted Ollama Dockerfile: %w", err)
	}
	args := []string{
		"build", "--platform", "linux/" + runtime.GOARCH,
		"-t", image, ".",
	}
	cmd := exec.Command("docker", args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build trusted Ollama image %s: %w", image, err)
	}
	return nil
}

func trustedOllamaDockerfile() string {
	return "FROM " + helmLLMOllamaSourceImage + "\n" +
		"COPY host-ca.pem /tmp/host-ca.pem\n" +
		"RUN cat /tmp/host-ca.pem >> /etc/ssl/certs/ca-certificates.crt && rm /tmp/host-ca.pem\n"
}

func hostTrustedCABundle() ([]byte, error) {
	//nolint:forbidigo // SSL_CERT_FILE is the OS and Go convention naming the host CA bundle; honoring it is reading a third-party contract, not repository configuration (GH-1481).
	if path := strings.TrimSpace(os.Getenv("SSL_CERT_FILE")); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read SSL_CERT_FILE for trusted Ollama image: %w", err)
		}
		return data, nil
	}
	if runtime.GOOS == "darwin" {
		out, err := exec.Command(
			"security", "find-certificate", "-a", "-p",
			"/Library/Keychains/System.keychain").CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf(
				"export macOS system trust for Ollama: %w: %s",
				err, strings.TrimSpace(string(out)))
		}
		if len(bytes.TrimSpace(out)) == 0 {
			return nil, fmt.Errorf("export macOS system trust for Ollama: empty certificate bundle")
		}
		return out, nil
	}
	for _, path := range []string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/pki/tls/certs/ca-bundle.crt",
	} {
		data, err := os.ReadFile(path)
		if err == nil && len(bytes.TrimSpace(data)) > 0 {
			return data, nil
		}
	}
	return nil, fmt.Errorf(
		"host CA bundle unavailable; set SSL_CERT_FILE to the trusted PEM bundle")
}

func loadIntegrationDependencyImages(cluster string, images []string) error {
	for _, image := range images {
		if err := loadSmokeDependencyImage(cluster, image); err != nil {
			return err
		}
	}
	return nil
}

func loadSmokeDependencyImage(cluster, image string) error {
	save := exec.Command("docker", "save", image)
	stream, err := save.StdoutPipe()
	if err != nil {
		return err
	}
	node := cluster + "-control-plane"
	load := exec.Command("docker", "exec", "-i", node, "ctr", "--namespace=k8s.io",
		"images", "import", "--platform=linux/"+runtime.GOARCH, "--snapshotter=overlayfs", "-")
	load.Stdin = stream
	var output bytes.Buffer
	load.Stdout, load.Stderr = &output, &output
	if err := load.Start(); err != nil {
		return err
	}
	if err := save.Run(); err != nil {
		_ = load.Process.Kill()
		_ = load.Wait()
		return fmt.Errorf("save smoke dependency %s: %w", image, err)
	}
	if err := load.Wait(); err != nil {
		return fmt.Errorf("load smoke dependency %s: %w: %s", image, err, strings.TrimSpace(output.String()))
	}
	return nil
}

// buildSmokeRuntimeImage builds the linux agent binary from the local agent-core
// checkout and bakes it into a minimal runtime image tagged for kind, so the
// smoke test runs the code under test rather than a published image. The image
// mirrors the production runtime contract (agent on PATH, core tools under
// AGENT_CORE_HOME) but invokes agent directly; the chart passes the same args the
// production agent-entrypoint would forward.
func buildSmokeRuntimeImage(coreRoot, image string) error {
	ctxDir, err := os.MkdirTemp("", "chatbot-mesh-image-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(ctxDir) }()

	build := exec.Command("go", "build", "-tags", "production", "-trimpath",
		"-ldflags=-s -w", "-o", filepath.Join(ctxDir, "agent"), "./cmd/agent")
	build.Dir = coreRoot
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux")
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("build linux agent: %w", err)
	}
	if err := copyDirContents(filepath.Join(coreRoot, "tools"), filepath.Join(ctxDir, "tools")); err != nil {
		return err
	}
	// Sibling of kindrig's agentCoreDockerfile; keep the tool contract in sync
	// (jq and ripgrep match the production agent-core transform tools, GH-1368).
	dockerfile := "FROM alpine:3.22\n" +
		"RUN apk add --no-cache ca-certificates bash jq ripgrep\n" +
		"COPY agent /usr/local/bin/agent\n" +
		"COPY tools /opt/agent-core/tools\n" +
		"ENV AGENT_CORE_HOME=/opt/agent-core HOME=/tmp PATH=/usr/local/bin:/usr/bin:/bin\n" +
		"ENTRYPOINT [\"agent\"]\n"
	if err := os.WriteFile(filepath.Join(ctxDir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		return fmt.Errorf("write smoke Dockerfile: %w", err)
	}
	docker := exec.Command("docker", smokeRuntimeBuildArgs(image)...)
	docker.Dir = ctxDir
	docker.Stdout, docker.Stderr = os.Stderr, os.Stderr
	if err := docker.Run(); err != nil {
		return fmt.Errorf("docker build %s: %w", image, err)
	}
	return nil
}

func smokeRuntimeBuildArgs(image string) []string {
	return []string{"build", "-t", image, "."}
}

// stageSmokeChart copies the classified chart source to a temp directory and
// stages the exact manifest-derived closure and provenance used by packaging.
func stageSmokeChart(chartDir, profilesRoot string) (string, func(), error) {
	catalogRoot, err := resolveCatalogRoot("chatbot-mesh Helm staging", profilesRoot)
	if err != nil {
		return "", nil, err
	}
	staged, err := os.MkdirTemp("", "chatbot-mesh-chart-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(staged) }
	dst := filepath.Join(staged, "chatbot-mesh")
	if err := stageChatbotChartSource(chartDir, dst); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := stageChatbotComposition(dst, profilesRoot, catalogRoot); err != nil {
		cleanup()
		return "", nil, err
	}
	return dst, cleanup, nil
}

func copyDirContents(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	cmd := exec.Command("cp", "-a", src+"/.", dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("copy %s -> %s: %s: %w", src, dst, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func helmInstallSmoke(
	chartPath, chartArchive, image string,
	telemetry helmTelemetryIdentity,
	assets []externalUIAsset,
) error {
	return helmInstallSmokeWithRunner(
		chartPath, chartArchive, image, telemetry, assets, runHelmSmokeCommand)
}

func helmInstallSmokeWithRunner(
	chartPath, chartArchive, image string,
	telemetry helmTelemetryIdentity,
	assets []externalUIAsset,
	run helmLLMCommandRunner,
) error {
	valueArgs := helmSmokeValueArgs(chartPath, image, telemetry, assets)
	measured, err := measureHelmReleaseBudget(
		helmRelease, chartPath, chartArchive, valueArgs)
	if err != nil {
		return err
	}
	fmt.Printf("helmSmoke: release budget PASS - %s\n", measured.String())
	args := append([]string{"install", helmRelease, chartPath}, valueArgs...)
	args = append(args, "--wait", "--timeout", helmInstallTimeout.String())
	out, err := run("helm", args...)
	if len(out) > 0 {
		_, _ = os.Stderr.Write(out)
	}
	if err != nil {
		return fmt.Errorf("helm install %s: %w: %s", helmRelease, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func helmSmokeValueArgs(
	chartPath, image string,
	telemetry helmTelemetryIdentity,
	assets []externalUIAsset,
) []string {
	repo, tag := splitImageRef(image)
	args := []string{
		"--values", filepath.Join(chartPath, "ci", "kind-values.yaml"),
		"--set", "image.repository=" + repo,
		"--set-string", "image.tag=" + tag,
		"--set", "image.pullPolicy=Never",
		"--set", "llm.externalURL=http://host.docker.internal:11434",
		"--set-string", "collector.externalOTLPEndpoint=" + telemetry.OTLPEndpoint,
		"--set-string", "collector.integrationResource.target=integration:helmSmoke",
		"--set-string", "collector.integrationResource.commit=" + telemetry.Commit,
		"--set-string", "collector.integrationResource.runID=" + telemetry.RunID,
	}
	return append(args, externalUIAssetValueArgs(assets)...)
}

func provisionExternalUIAssets(assets []externalUIAsset) error {
	for _, asset := range assets {
		if err := provisionExternalUIAsset(asset); err != nil {
			return err
		}
	}
	return nil
}

// assertHelmIntegrationRelease checks every stored revision against the release
// budget, then proves that the revision just created exists. The second check is
// significant for the swap gate, where a successful-looking upgrade must create
// v2 rather than leave only the install Secret.
func assertHelmIntegrationRelease(
	releaseName, chartArchive string,
	assets []externalUIAsset,
	revision int,
) error {
	if err := assertHelmReleaseSecrets(releaseName, chartArchive, assets); err != nil {
		return err
	}
	name := fmt.Sprintf("sh.helm.release.v1.%s.v%d", releaseName, revision)
	out, err := exec.Command("kubectl", "get", "secret", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Helm release %s revision %d Secret missing: %w: %s",
			releaseName, revision, err, strings.TrimSpace(string(out)))
	}
	return nil
}

var runHelmSmokeCommand helmLLMCommandRunner = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// splitImageRef splits repo:tag on the last colon so a registry port in the repo
// is preserved.
func splitImageRef(image string) (repo, tag string) {
	if i := strings.LastIndex(image, ":"); i >= 0 && !strings.Contains(image[i:], "/") {
		return image[:i], image[i+1:]
	}
	return image, "latest"
}

// kubectlPortForward forwards each remote port to the same local port and returns
// a stop function. Fixed local ports keep the assertion URLs constant; the smoke
// test owns the loopback ports for its duration.
func kubectlPortForward(target string, ports ...int) (func(), error) {
	args := []string{"port-forward", target}
	for _, p := range ports {
		args = append(args, fmt.Sprintf("%d:%d", p, p))
	}
	cmd := exec.Command("kubectl", args...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("kubectl port-forward %s: %w", target, err)
	}
	return func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}, nil
}

// assertSmokeChatServed posts one chat turn and asserts the mesh answered (200
// with a non-empty answer). The deploy smoke bar is that the served
// machine_request endpoint routes a turn through the chatbot in cluster.
func assertSmokeChatServed(url string) error {
	body := `{"message":"Summarize the most relevant record you can retrieve."}`
	data, status, err := requestInference(http.MethodPost, url, body, "in-cluster chat turn")
	if err != nil {
		return fmt.Errorf("chat request: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("chat turn status %d: %s", status, strings.TrimSpace(string(data)))
	}
	var resp struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("decode chat response: %w: %s", err, strings.TrimSpace(string(data)))
	}
	if strings.TrimSpace(resp.Answer) == "" {
		return fmt.Errorf("chat turn returned an empty answer: %s", strings.TrimSpace(string(data)))
	}
	return nil
}

// assertCollectorAvailable waits for the collector Deployment to report
// available. Nothing else in the smoke touches the collector, so before GH-736
// a collector that never started surfaced only as an empty spool after the span
// timeout -- a symptom two hops from the cause.
func assertCollectorAvailable(release string, timeout time.Duration) error {
	target := "deploy/" + release + "-chatbot-mesh-collector"
	out, err := exec.Command("kubectl", "wait", "--for=condition=available", target,
		fmt.Sprintf("--timeout=%ds", int(timeout.Seconds()))).CombinedOutput()
	if err == nil {
		return nil
	}
	return fmt.Errorf("collector never became available: %v: %s%s",
		err, strings.TrimSpace(string(out)), collectorDiagnostics(release))
}

// collectorDiagnostics returns the collector's pod line and last log lines, so
// a span failure names the hop that dropped them instead of leaving the reader
// to guess between "the agents never exported" and "the collector never ran"
// (GH-736 R4).
func collectorDiagnostics(release string) string {
	selector := "app.kubernetes.io/component=collector,app.kubernetes.io/instance=" + release
	var b strings.Builder
	if out, err := exec.Command("kubectl", "get", "pods", "-l", selector, "--no-headers").CombinedOutput(); err == nil {
		b.WriteString("\n  collector pod: " + strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("kubectl", "logs", "-l", selector, "--tail=5").CombinedOutput(); err == nil {
		if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
			b.WriteString("\n  collector logs: " + trimmed)
		}
	}
	return b.String()
}

var spoolTraceIDPattern = regexp.MustCompile(`"TraceID":"([0-9a-f]+)"`)

func assertCollectorSpoolIdentity(release, runID string, services []string, timeout time.Duration) error {
	target := "deploy/" + release + "-chatbot-mesh-collector"
	path := "/work/traces/collector.ndjson"
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		out, err := exec.Command("kubectl", "exec", target, "--", "sh", "-c",
			"test -s "+path+" && cat "+path).CombinedOutput()
		last = string(out)
		if err == nil && strings.Contains(last, runID) {
			all := true
			for _, service := range services {
				if !strings.Contains(last, service) {
					all = false
					break
				}
			}
			if all {
				ids := spoolTraceIDPattern.FindAllStringSubmatch(last, -1)
				fmt.Printf("helmSmoke: collector spool contains %d trace ids for run %s services %v\n",
					len(ids), runID, services)
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("collector spool lacks run %q and services %v%s",
		runID, services, collectorDiagnostics(release))
}

func assertNoCollectorSelfIngest(release, runID string) error {
	target := "deploy/" + release + "-chatbot-mesh-collector"
	path := "/work/traces/collector.ndjson"
	out, err := exec.Command("kubectl", "exec", target, "--", "sh", "-c",
		"test -s "+path+" && cat "+path).CombinedOutput()
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, `"collector"`) && strings.Contains(line, runID) {
			return fmt.Errorf("collector self-ingested spans for run %s", runID)
		}
	}
	return nil
}

func verifySharedMetricsEvidence(
	queryBase string,
	telemetry helmTelemetryIdentity,
	timeout time.Duration,
) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		evidence, err := collectSharedMetricsEvidence(queryBase, telemetry)
		if err == nil {
			fmt.Printf(
				"helmSmoke: retained evidence agent_metrics=%s dolt_metrics=%s\n",
				strings.Join(evidence.AgentMetrics, ","),
				strings.Join(evidence.DoltMetrics, ","),
			)
			return nil
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	return lastErr
}

type sharedMetricsEvidence struct {
	AgentMetrics []string
	DoltMetrics  []string
}

// collectSharedMetricsEvidence reads the collector agent's metric query surface
// (srd042 R9, GH-1207) rather than Prometheus. The agent dispatch metric is
// found by the services that emit it (chatbot and rag0) rather than a fixed
// name, since the OTLP metric name is not Prometheus-mangled; Dolt metrics keep
// the dss_ prefix and the run identity rides in each record's resource
// attributes.
func collectSharedMetricsEvidence(
	queryBase string,
	telemetry helmTelemetryIdentity,
) (sharedMetricsEvidence, error) {
	evidence := sharedMetricsEvidence{}
	summaries, err := collectorMetricSummaries(queryBase)
	if err != nil {
		return evidence, err
	}
	var agentMetric string
	for _, summary := range summaries {
		if metricServicesInclude(summary.Services, "chatbot", "rag0") {
			agentMetric = summary.Name
			evidence.AgentMetrics = append(evidence.AgentMetrics, summary.Name)
		}
		if strings.HasPrefix(summary.Name, "dss_") && metricServicesInclude(summary.Services, "dolt") {
			evidence.DoltMetrics = append(evidence.DoltMetrics, summary.Name)
		}
	}
	if agentMetric == "" {
		return evidence, fmt.Errorf("collector metrics missing an agent metric emitted by chatbot and rag0 for run %s", telemetry.RunID)
	}
	// In agent mode the declarative collector has no Prometheus scrape, so the
	// in-cluster Dolt metric path is retired (GH-1366); Dolt dss_* metrics are
	// recorded when present (opt-in contrib) but no longer required.
	found, err := collectorMetricHasRunID(queryBase, agentMetric, telemetry.RunID)
	if err != nil {
		return evidence, err
	}
	if !found {
		return evidence, fmt.Errorf("collector metric %s records lack run id %s", agentMetric, telemetry.RunID)
	}
	sort.Strings(evidence.AgentMetrics)
	sort.Strings(evidence.DoltMetrics)
	return evidence, nil
}

func collectorMetricHasRunID(queryBase, metricName, runID string) (bool, error) {
	const pageSize = 20
	for offset := 0; ; offset += pageSize {
		query := url.Values{
			"page_size": {strconv.Itoa(pageSize)},
			"offset":    {strconv.Itoa(offset)},
		}
		data, status, err := requestHTTP(http.MethodGet,
			queryBase+"/query/metrics/"+url.PathEscape(metricName)+"?"+query.Encode(), "")
		if err != nil {
			return false, err
		}
		if status != http.StatusOK {
			return false, fmt.Errorf("collector /query/metrics/%s status %d: %s",
				metricName, status, strings.TrimSpace(string(data)))
		}
		var page struct {
			Records  []json.RawMessage `json:"records"`
			Total    int               `json:"total"`
			Offset   int               `json:"offset"`
			PageSize int               `json:"page_size"`
		}
		if err := json.Unmarshal(data, &page); err != nil {
			return false, fmt.Errorf("decode collector metric %s page: %w", metricName, err)
		}
		for _, record := range page.Records {
			if strings.Contains(string(record), runID) {
				return true, nil
			}
		}
		if offset+len(page.Records) >= page.Total {
			return false, nil
		}
		if len(page.Records) == 0 {
			return false, fmt.Errorf("collector metric %s returned an empty page before total %d",
				metricName, page.Total)
		}
	}
}

type collectorMetricSummary struct {
	Name     string   `json:"name"`
	Services []string `json:"services"`
}

func collectorMetricSummaries(queryBase string) ([]collectorMetricSummary, error) {
	data, status, err := requestHTTP(http.MethodGet, queryBase+"/query/metrics?page_size=100", "")
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("collector /query/metrics status %d: %s",
			status, strings.TrimSpace(string(data)))
	}
	var listed struct {
		Metrics []collectorMetricSummary `json:"metrics"`
	}
	if err := json.Unmarshal(data, &listed); err != nil {
		return nil, fmt.Errorf("decode collector metric list: %w", err)
	}
	return listed.Metrics, nil
}

func metricServicesInclude(services []string, required ...string) bool {
	for _, want := range required {
		if !containsString(services, want) {
			return false
		}
	}
	return true
}

const (
	helmSwapRelease = "swap"
	helmSwapCluster = "da-chatbot-mesh-swap"
)

// HelmSwap proves the two tiered-swap paths of the chatbot-mesh chart on a kind
// cluster (srd003 R3): repointing a RAG is a Service selector change that does not
// roll the chatbot (R3.1), and adding a RAG re-renders the co-generated profile and
// rolls the chatbot (R3.2). It asserts the infrastructure contracts (pod identity,
// workload existence, the co-generated ConfigMap breadth) via kubectl and helm,
// and holds a deterministic mock-model answer open during the rollout to prove the
// old pod drains the active HTTP turn before it exits. It needs docker, kind, helm,
// and kubectl but no installed Ollama models. Teardown (kind delete) runs in all
// paths.
func (Integration) HelmSwap() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := demoCoreRoot(profilesRoot)
	chartDir := applicationChartDir(profilesRoot)
	if _, err := os.Stat(filepath.Join(chartDir, "Chart.yaml")); err != nil {
		return fmt.Errorf("chatbot-mesh chart not found at %s: %w", chartDir, err)
	}
	for _, bin := range []string{"docker", "kind", "helm", "kubectl"} {
		if _, err := exec.LookPath(bin); err != nil {
			fmt.Printf("SKIP helmSwap: %s not found on PATH\n", bin)
			return nil
		}
	}
	if !agentCoreAvailable(coreRoot) {
		fmt.Printf("SKIP helmSwap: agent-core checkout not found at %s (set core_root in demo.yaml)\n", coreRoot)
		return nil
	}
	return runHelmSwap(coreRoot, profilesRoot, chartDir)
}

func runHelmSwap(coreRoot, profilesRoot, chartDir string) (result error) {
	images, err := resolveChatbotIntegrationImages(profilesRoot)
	if err != nil {
		return err
	}
	fmt.Printf("helmSwap: building runtime image %s\n", images.Runtime)
	if err := buildSmokeRuntimeImage(coreRoot, images.Runtime); err != nil {
		return err
	}
	llmMock, err := startHelmSwapLLMMock()
	if err != nil {
		return err
	}
	defer llmMock.close()
	stagedChart, cleanupChart, err := stageSmokeChart(chartDir, profilesRoot)
	if err != nil {
		return err
	}
	defer cleanupChart()
	assets, cleanupAssets, err := externalizeUIAssets(stagedChart, helmSwapRelease)
	if err != nil {
		return err
	}
	defer cleanupAssets()
	chartArchive, cleanupArchive, err := packageApplierChart(stagedChart)
	if err != nil {
		return err
	}
	defer cleanupArchive()
	dependencyImages, err := swapDependencyImages(chartDir)
	if err != nil {
		return err
	}
	if err := pullIntegrationDependencyImages(
		"helmSwap", dependencyImages, runHelmSmokeCommand); err != nil {
		return err
	}

	swapCluster, err := kindrig.EnsureCluster(kindrig.DefaultRun, helmSwapCluster, helmKindConfig(chartDir), helmClusterWait)
	if err != nil {
		return err
	}
	unbindKubeconfig, err := bindClusterKubeconfig(helmSwapCluster)
	if err != nil {
		swapCluster.Release(kindrig.DefaultRun)
		return err
	}
	evidenceDir := helmScenarioEvidenceDirectory(
		profilesRoot, helmSwapCluster, images.Revision)
	defer func() {
		failed := result != nil
		if failed {
			diagnostics := captureHelmFailureDiagnostics(
				evidenceDir, helmSwapRelease, runHelmDiagnosticCommand,
				helmFailureDiagnosticsTimeout)
			result = fmt.Errorf("%w\n%s", result, diagnostics)
		}
		swapCluster.ReleaseAfter(kindrig.DefaultRun, failed, kindrig.FailureEvidence{
			Directory:  evidenceDir,
			Namespaces: []string{"default"},
			Run:        boundedHelmEvidenceRunner(helmEvidenceCommandTimeout),
		})
		unbindKubeconfig()
	}()
	if err := provisionExternalUIAssets(assets); err != nil {
		return err
	}
	if err := loadKindImage(helmSwapCluster, images.Runtime); err != nil {
		return err
	}
	if err := loadIntegrationDependencyImages(
		helmSwapCluster, dependencyImages); err != nil {
		return err
	}

	// Deploy three units so the rollout can remove the middle identity without
	// changing the later unit's name.
	initial := append(llmMock.helmArgs(),
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
	)
	if err := helmSwapDeploy(
		stagedChart, chartArchive, images.Runtime, "install", initial, assets); err != nil {
		return err
	}
	if err := assertHelmIntegrationRelease(
		helmSwapRelease, chartArchive, assets, 1); err != nil {
		return err
	}
	if err := assertExternalUIAssetsMounted(
		helmSwapRelease, assets, helmReadyTimeout); err != nil {
		return err
	}

	if err := assertSwapRepoint(); err != nil {
		return err
	}
	if err := assertSwapReplaceMiddleRag(
		stagedChart, chartArchive, images.Runtime, llmMock, assets); err != nil {
		return err
	}
	fmt.Printf("integration:helmSwap PASS - revision %s repoint left the chatbot pod unchanged; replacing middle unit rag1 with rag4 preserved rag2 identity and an in-flight turn, rolled the chatbot, and served a turn from the replacement pod\n", images.Revision)
	return nil
}

// helmSwapDeploy installs or upgrades the release with the given extra --set args.
func helmSwapDeploy(
	chartPath, chartArchive, image, verb string,
	extra []string,
	assets []externalUIAsset,
) error {
	return helmSwapDeployWithRunner(
		chartPath, chartArchive, image, verb, extra, assets, runHelmSmokeCommand)
}

func helmSwapDeployWithRunner(
	chartPath, chartArchive, image, verb string,
	extra []string,
	assets []externalUIAsset,
	run helmLLMCommandRunner,
) error {
	valueArgs := helmSwapValueArgs(chartPath, image, extra, assets)
	measured, err := measureHelmReleaseBudget(
		helmSwapRelease, chartPath, chartArchive, valueArgs)
	if err != nil {
		return err
	}
	fmt.Printf("helmSwap %s: release budget PASS - %s\n", verb, measured.String())
	args := append([]string{verb, helmSwapRelease, chartPath}, valueArgs...)
	args = append(args, "--wait", "--timeout", helmInstallTimeout.String())
	out, err := run("helm", args...)
	if len(out) > 0 {
		_, _ = os.Stderr.Write(out)
	}
	if err != nil {
		return fmt.Errorf("helm %s %s: %w: %s",
			verb, helmSwapRelease, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func helmSwapValueArgs(
	chartPath, image string,
	extra []string,
	assets []externalUIAsset,
) []string {
	repo, tag := splitImageRef(image)
	args := []string{
		"--values", filepath.Join(chartPath, "ci", "kind-values.yaml"),
		"--set", "image.repository=" + repo,
		"--set-string", "image.tag=" + tag,
		"--set", "image.pullPolicy=Never",
	}
	args = append(args, extra...)
	return append(args, externalUIAssetValueArgs(assets)...)
}

// assertSwapRepoint patches the rag0 Service selector and asserts the chatbot pod
// is unchanged: repointing a RAG source touches no agent configuration and requires
// no chatbot restart (srd003 R3.1).
func assertSwapRepoint() error {
	before, err := chatbotPodName()
	if err != nil {
		return err
	}
	svc := helmSwapRelease + "-chatbot-mesh-rag0"
	patch := `{"spec":{"selector":{"chatbot-mesh/rag-unit":"rag0","repointed":"true"}}}`
	// Helm 4 uses server-side apply for upgrades. Keep this intentional live
	// selector drift under Helm's field manager so the following upgrade can
	// reconcile it instead of conflicting with kubectl-patch ownership.
	cmd := exec.Command("kubectl", "patch", "service", svc,
		"--field-manager=helm", "-p", patch)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("repoint patch of %s: %w", svc, err)
	}
	after, err := chatbotPodName()
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("repoint rolled the chatbot pod (%s -> %s); a Service selector change must not restart the chatbot (R3.1)", before, after)
	}
	// The next proof changes the Helm values rather than retaining this manual
	// drift. Restore the chart-owned selector after observing the unchanged pod;
	// otherwise Helm 4's server-side apply correctly rejects the later upgrade
	// because the live selector still differs from the release manifest.
	restore := `{"spec":{"selector":{"repointed":null}}}`
	out, err := exec.Command("kubectl", "patch", "service", svc,
		"--field-manager=helm", "-p", restore).CombinedOutput()
	if err != nil {
		return fmt.Errorf("restore selector of %s after repoint proof: %w: %s",
			svc, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// assertSwapReplaceMiddleRag removes rag1 from the middle of rag0/rag1/rag2 and
// adds rag4, then proves rag2 kept its identity, the active turn drained, and the
// chatbot Deployment rolled to the replacement config (srd003 R2/R3.2).
func assertSwapReplaceMiddleRag(
	chartPath, chartArchive, image string,
	llmMock *helmSwapLLMMock,
	assets []externalUIAsset,
) error {
	genBefore, err := chatbotDeploymentGeneration()
	if err != nil {
		return err
	}
	stopForward, err := kubectlPortForward("svc/"+helmSwapRelease+"-chatbot-mesh-chatbot", 18080, 18081)
	if err != nil {
		return err
	}
	defer func() {
		if stopForward != nil {
			stopForward()
		}
	}()
	if err := waitHTTPStatus(helmHealthURL, http.StatusOK, helmReadyTimeout); err != nil {
		return fmt.Errorf("chatbot did not become reachable before warm swap: %w", err)
	}
	turnResult := make(chan error, 1)
	go func() { turnResult <- assertSmokeChatServed(helmChatURL) }()
	if err := llmMock.waitForAnswer(helmReadyTimeout); err != nil {
		return err
	}

	extra := append(llmMock.helmArgs(),
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
	)
	if err := helmSwapDeploy(
		chartPath, chartArchive, image, "upgrade", extra, assets); err != nil {
		return err
	}
	if err := assertHelmIntegrationRelease(
		helmSwapRelease, chartArchive, assets, 2); err != nil {
		return err
	}
	if err := assertExternalUIAssetsMounted(
		helmSwapRelease, assets, helmReadyTimeout); err != nil {
		return err
	}
	if err := <-turnResult; err != nil {
		return fmt.Errorf("middle-RAG replacement dropped the in-flight chat turn: %w", err)
	}
	stopForward()
	stopForward = nil

	stopReplacementForward, err := kubectlPortForward("svc/"+helmSwapRelease+"-chatbot-mesh-chatbot", 18080, 18081)
	if err != nil {
		return err
	}
	defer stopReplacementForward()
	if err := waitHTTPStatus(helmHealthURL, http.StatusOK, helmReadyTimeout); err != nil {
		return fmt.Errorf("replacement chatbot did not become reachable: %w", err)
	}
	if err := assertSmokeChatServed(helmChatURL); err != nil {
		return fmt.Errorf("replacement chatbot did not serve a turn: %w", err)
	}
	if err := kubectlResourceExists("deployment", helmSwapRelease+"-chatbot-mesh-rag4"); err != nil {
		return fmt.Errorf("middle-RAG replacement did not stand up the rag4 Deployment: %w", err)
	}
	if out, err := exec.Command("kubectl", "get", "deployment", helmSwapRelease+"-chatbot-mesh-rag1").CombinedOutput(); err == nil {
		return fmt.Errorf("middle-RAG replacement left the removed rag1 Deployment present: %s", strings.TrimSpace(string(out)))
	}
	cm, err := kubectlConfigMapKey(helmSwapRelease+"-chatbot-mesh-profiles", "agents__chatbot__rest.yaml")
	if err != nil {
		return err
	}
	for _, identity := range []string{"rag0", "rag2", "rag4"} {
		host := helmSwapRelease + "-chatbot-mesh-" + identity
		if !strings.Contains(cm, identity+": http://"+host+":18087") ||
			!strings.Contains(cm, "- "+host) {
			return fmt.Errorf("middle-RAG replacement did not preserve %s in chatbot rest.yaml (R2/R3.2)", identity)
		}
	}
	if strings.Contains(cm, "rag1:") ||
		strings.Contains(cm, helmSwapRelease+"-chatbot-mesh-rag1") {
		return fmt.Errorf("middle-RAG replacement left positional or removed client rag1 in chatbot rest.yaml (R2)")
	}
	genAfter, err := chatbotDeploymentGeneration()
	if err != nil {
		return err
	}
	if genAfter <= genBefore {
		return fmt.Errorf("add-RAG did not roll the chatbot Deployment (generation %d -> %d); the profile change must trigger a rollout (R3.2)", genBefore, genAfter)
	}
	return nil
}

func chatbotPodName() (string, error) {
	out, err := exec.Command("kubectl", "get", "pods",
		"-l", "app.kubernetes.io/component=chatbot",
		"-o", "jsonpath={.items[0].metadata.name}").Output()
	if err != nil {
		return "", fmt.Errorf("get chatbot pod: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func chatbotDeploymentGeneration() (int, error) {
	out, err := exec.Command("kubectl", "get", "deployment", helmSwapRelease+"-chatbot-mesh-chatbot",
		"-o", "jsonpath={.metadata.generation}").Output()
	if err != nil {
		return 0, fmt.Errorf("get chatbot deployment generation: %w", err)
	}
	var gen int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &gen); err != nil {
		return 0, fmt.Errorf("parse chatbot generation %q: %w", out, err)
	}
	return gen, nil
}

func kubectlResourceExists(kind, name string) error {
	cmd := exec.Command("kubectl", "get", kind, name)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

func kubectlConfigMapKey(name, key string) (string, error) {
	out, err := exec.Command("kubectl", "get", "configmap", name,
		"-o", "jsonpath={.data."+strings.ReplaceAll(key, ".", "\\.")+"}").Output()
	if err != nil {
		return "", fmt.Errorf("get configmap %s key %s: %w", name, key, err)
	}
	return string(out), nil
}

const (
	helmLLMRelease = "llm"
	helmLLMCluster = "da-chatbot-mesh-llm"

	helmLLMChatURL   = "http://127.0.0.1:18080/api/v1/chat"
	helmLLMHealthURL = "http://127.0.0.1:18081/api/lifecycle/health"
	helmLLMTagsURL   = "http://127.0.0.1:11434/api/tags"

	// Model pulls run on CPU inside kind. Installation deliberately does not use
	// --wait: the integration observes the agent readiness transition around the
	// suspended preload Job before allowing models to pull.
	helmLLMInstallTimeout       = 20 * time.Minute
	helmLLMStartupTimeout       = 5 * time.Minute
	helmLLMModelPreloadTimeout  = 20 * time.Minute
	helmLLMWorkloadReadyTimeout = 5 * time.Minute

	helmLLMOllamaImage       = "declarative-agents/ollama:0.32.5-kind-trusted"
	helmLLMOllamaSourceImage = "ollama/ollama:0.32.5@sha256:4dea9fb511947e24a84237bb636b0203abcb2ff0d3fbc7b4ff865deb91362131"
)

// helmLLMModels are the CPU-only small models the kind LLM-tier values pull; the
// assertion confirms /api/tags reports each one after preload.
var helmLLMModels = []string{"all-minilm", "qwen2.5:0.5b"}

func llmDependencyImages(chartDir string) ([]string, error) {
	images, err := smokeDependencyImages(chartDir)
	if err != nil {
		return nil, err
	}
	var values struct {
		Ollama struct {
			Image struct {
				Repository string `yaml:"repository"`
				Tag        string `yaml:"tag"`
				PullPolicy string `yaml:"pullPolicy"`
			} `yaml:"image"`
		} `yaml:"ollama"`
	}
	if err := readIntegrationYAML(
		filepath.Join(chartDir, "ci", "kind-llm-values.yaml"),
		"kind LLM values", &values); err != nil {
		return nil, err
	}
	image := values.Ollama.Image.Repository + ":" + values.Ollama.Image.Tag
	if image != helmLLMOllamaImage {
		return nil, fmt.Errorf(
			"kind LLM Ollama image = %q, want exact trusted image %q",
			image, helmLLMOllamaImage)
	}
	if values.Ollama.Image.PullPolicy != "Never" {
		return nil, fmt.Errorf(
			"kind LLM Ollama pullPolicy = %q, want Never for kind-loaded image",
			values.Ollama.Image.PullPolicy)
	}
	return append(images, image), nil
}

// HelmLLMTier deploys the chart with the in-cluster LLM tier enabled on a kind
// cluster and proves the tier stands up: the Ollama StatefulSet becomes ready, the
// preload Job pulls the configured models once, /api/tags reports them, and the
// chatbot serves a turn wired to the in-cluster endpoint (srd003 R6). CPU-only
// small models keep it runnable without a GPU, a recorded divergence from GPU
// production sizing (R6.4). It gates on docker, kind, helm, and kubectl, recording
// a skip for each missing dependency; unlike helmSmoke it needs no external Ollama
// because the tier under test is the in-cluster one. Teardown (kind delete) runs in
// all paths.
func (Integration) HelmLLMTier() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := demoCoreRoot(profilesRoot)
	chartDir := applicationChartDir(profilesRoot)
	if _, err := os.Stat(filepath.Join(chartDir, "Chart.yaml")); err != nil {
		return fmt.Errorf("chatbot-mesh chart not found at %s: %w", chartDir, err)
	}
	for _, bin := range []string{"docker", "kind", "helm", "kubectl"} {
		if _, err := exec.LookPath(bin); err != nil {
			fmt.Printf("SKIP helmLLMTier: %s not found on PATH\n", bin)
			return nil
		}
	}
	if !agentCoreAvailable(coreRoot) {
		fmt.Printf("SKIP helmLLMTier: agent-core checkout not found at %s (set core_root in demo.yaml)\n", coreRoot)
		return nil
	}
	return runHelmLLMTier(coreRoot, profilesRoot, chartDir)
}

func runHelmLLMTier(coreRoot, profilesRoot, chartDir string) (result error) {
	images, err := resolveChatbotIntegrationImages(profilesRoot)
	if err != nil {
		return err
	}
	fmt.Printf("helmLLMTier: building runtime image %s\n", images.Runtime)
	if err := buildSmokeRuntimeImage(coreRoot, images.Runtime); err != nil {
		return err
	}
	stagedChart, cleanupChart, err := stageSmokeChart(chartDir, profilesRoot)
	if err != nil {
		return err
	}
	defer cleanupChart()
	assets, cleanupAssets, err := externalizeUIAssets(stagedChart, helmLLMRelease)
	if err != nil {
		return err
	}
	defer cleanupAssets()
	chartArchive, cleanupArchive, err := packageApplierChart(stagedChart)
	if err != nil {
		return err
	}
	defer cleanupArchive()
	dependencyImages, err := llmDependencyImages(chartDir)
	if err != nil {
		return err
	}
	if err := pullIntegrationDependencyImages(
		"helmLLMTier", dependencyImages, runHelmLLMCommand); err != nil {
		return err
	}
	fmt.Printf("helmLLMTier: building trusted Ollama runtime %s from %s\n",
		helmLLMOllamaImage, helmLLMOllamaSourceImage)
	if err := buildTrustedOllamaImage(helmLLMOllamaImage); err != nil {
		return err
	}

	llmCluster, err := kindrig.EnsureCluster(kindrig.DefaultRun, helmLLMCluster, helmKindConfig(chartDir), helmClusterWait)
	if err != nil {
		return err
	}
	unbindKubeconfig, err := bindClusterKubeconfig(helmLLMCluster)
	if err != nil {
		llmCluster.Release(kindrig.DefaultRun)
		return err
	}
	evidenceDir := helmScenarioEvidenceDirectory(
		profilesRoot, helmLLMCluster, images.Revision)
	defer func() {
		failed := result != nil
		if failed {
			diagnostics := captureHelmFailureDiagnostics(
				evidenceDir, helmLLMRelease, runHelmDiagnosticCommand,
				helmFailureDiagnosticsTimeout)
			result = fmt.Errorf("%w\n%s", result, diagnostics)
		}
		llmCluster.ReleaseAfter(kindrig.DefaultRun, failed, kindrig.FailureEvidence{
			Directory:  evidenceDir,
			Namespaces: []string{"default"},
			Run:        boundedHelmEvidenceRunner(helmEvidenceCommandTimeout),
		})
		unbindKubeconfig()
	}()
	if err := provisionExternalUIAssets(assets); err != nil {
		return err
	}
	if err := loadKindImage(helmLLMCluster, images.Runtime); err != nil {
		return err
	}
	if err := loadIntegrationDependencyImages(
		helmLLMCluster, dependencyImages); err != nil {
		return err
	}
	if err := helmInstallLLM(
		stagedChart, chartArchive, images.Runtime, assets); err != nil {
		return err
	}
	if err := assertHelmIntegrationRelease(
		helmLLMRelease, chartArchive, assets, 1); err != nil {
		return err
	}

	// Ollama must serve before the suspended preload Job can be resumed; agent
	// readiness remains blocked until the transition proof below completes.
	workloads, err := beginObservedLLMPreload(runHelmLLMCommand)
	if err != nil {
		return err
	}

	stopTags, err := kubectlPortForward("svc/"+helmLLMRelease+"-chatbot-mesh-ollama", 11434)
	if err != nil {
		return err
	}
	if err := assertLLMModelsLoaded(
		helmLLMTagsURL, helmLLMModels, helmLLMModelPreloadTimeout); err != nil {
		stopTags()
		return err
	}
	stopTags()
	if err := finishLLMPreloadTransition(runHelmLLMCommand, workloads); err != nil {
		return err
	}
	if err := assertExternalUIAssetsMounted(
		helmLLMRelease, assets, helmLLMWorkloadReadyTimeout); err != nil {
		return err
	}

	stop, err := kubectlPortForward("svc/"+helmLLMRelease+"-chatbot-mesh-chatbot", 18080, 18081)
	if err != nil {
		return err
	}
	defer stop()
	if err := waitHTTPStatus(helmLLMHealthURL, http.StatusOK, helmReadyTimeout); err != nil {
		return fmt.Errorf("chatbot control health not ready: %w", err)
	}
	if err := assertSmokeChatServed(helmLLMChatURL); err != nil {
		return fmt.Errorf("chatbot did not serve a turn against the in-cluster LLM: %w", err)
	}

	fmt.Printf("integration:helmLLMTier PASS - revision %s the chart stood up the in-cluster Ollama tier, the preload Job pulled the configured models, /api/tags reported them, and the chatbot served a turn against the in-cluster endpoint\n", images.Revision)
	return nil
}

func helmInstallLLM(
	chartPath, chartArchive, image string,
	assets []externalUIAsset,
) error {
	return helmInstallLLMWithRunner(
		chartPath, chartArchive, image, assets, runHelmLLMCommand)
}

func helmInstallLLMWithRunner(
	chartPath, chartArchive, image string,
	assets []externalUIAsset,
	run helmLLMCommandRunner,
) error {
	valueArgs := helmLLMValueArgs(chartPath, image, assets)
	measured, err := measureHelmReleaseBudget(
		helmLLMRelease, chartPath, chartArchive, valueArgs)
	if err != nil {
		return err
	}
	fmt.Printf("helmLLMTier: release budget PASS - %s\n", measured.String())
	args := append([]string{"install", helmLLMRelease, chartPath}, valueArgs...)
	args = append(args, "--timeout", helmLLMInstallTimeout.String())
	out, err := run("helm", args...)
	if len(out) > 0 {
		_, _ = os.Stderr.Write(out)
	}
	if err != nil {
		return fmt.Errorf("helm install %s: %w: %s",
			helmLLMRelease, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func helmLLMValueArgs(
	chartPath, image string,
	assets []externalUIAsset,
) []string {
	repo, tag := splitImageRef(image)
	args := []string{
		"--values", filepath.Join(chartPath, "ci", "kind-llm-values.yaml"),
		"--set", "image.repository=" + repo,
		"--set-string", "image.tag=" + tag,
		"--set", "image.pullPolicy=Never",
		"--set", "ollama.preload.suspend=true",
	}
	return append(args, externalUIAssetValueArgs(assets)...)
}

type helmLLMCommandRunner func(name string, args ...string) ([]byte, error)

var runHelmLLMCommand helmLLMCommandRunner = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

type llmWorkloadList struct {
	Items []struct {
		Metadata struct {
			Name   string            `json:"name"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Spec struct {
			Replicas int `json:"replicas"`
		} `json:"spec"`
		Status struct {
			ReadyReplicas int `json:"readyReplicas"`
		} `json:"status"`
	} `json:"items"`
}

func beginLLMPreloadTransition(run helmLLMCommandRunner) ([]string, error) {
	job := helmLLMRelease + "-chatbot-mesh-ollama-preload"
	jobData, err := run("kubectl", "get", "job/"+job, "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("inspect suspended preload Job: %w: %s", err, strings.TrimSpace(string(jobData)))
	}
	var jobState struct {
		Spec struct {
			Suspend bool `json:"suspend"`
		} `json:"spec"`
		Status struct {
			Succeeded int `json:"succeeded"`
		} `json:"status"`
	}
	if err := json.Unmarshal(jobData, &jobState); err != nil {
		return nil, fmt.Errorf("decode preload Job: %w", err)
	}
	if !jobState.Spec.Suspend || jobState.Status.Succeeded != 0 {
		return nil, fmt.Errorf("preload observation requires suspended incomplete Job; suspend=%t succeeded=%d",
			jobState.Spec.Suspend, jobState.Status.Succeeded)
	}

	data, err := run("kubectl", "get", "deployment",
		"-l", "app.kubernetes.io/instance="+helmLLMRelease, "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("inspect pre-preload agent readiness: %w: %s", err, strings.TrimSpace(string(data)))
	}
	var list llmWorkloadList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("decode agent workload readiness: %w", err)
	}
	var names []string
	for _, item := range list.Items {
		component := item.Metadata.Labels["app.kubernetes.io/component"]
		if component != "chatbot" && component != "rag-server" {
			continue
		}
		names = append(names, item.Metadata.Name)
		if item.Spec.Replicas <= 0 || item.Status.ReadyReplicas >= item.Spec.Replicas {
			return nil, fmt.Errorf("agent workload %s became ready before preload: ready=%d desired=%d",
				item.Metadata.Name, item.Status.ReadyReplicas, item.Spec.Replicas)
		}
	}
	if len(names) < 2 {
		return nil, fmt.Errorf("expected chatbot and RAG workloads before preload, found %v", names)
	}
	fmt.Printf("helmLLMTier: agents unready while preload is suspended: %s\n", strings.Join(names, ", "))
	patch := `{"spec":{"suspend":false}}`
	if out, err := run("kubectl", "patch", "job/"+job, "--type=merge", "-p", patch); err != nil {
		return nil, fmt.Errorf("resume preload Job: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if out, err := run("kubectl", "wait", "--for=condition=complete", "job/"+job,
		"--timeout", helmLLMModelPreloadTimeout.String()); err != nil {
		return nil, fmt.Errorf("preload Job did not complete: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return names, nil
}

func beginObservedLLMPreload(run helmLLMCommandRunner) ([]string, error) {
	if err := waitForLLMStartup(run); err != nil {
		return nil, fmt.Errorf("ollama StatefulSet did not become ready: %w", err)
	}
	return beginLLMPreloadTransition(run)
}

func finishLLMPreloadTransition(run helmLLMCommandRunner, workloads []string) error {
	for _, name := range workloads {
		if out, err := run("kubectl", "rollout", "status", "deployment/"+name,
			"--timeout", helmLLMWorkloadReadyTimeout.String()); err != nil {
			return fmt.Errorf("agent workload %s did not become ready after model preload: %w: %s",
				name, err, strings.TrimSpace(string(out)))
		}
	}
	fmt.Printf("helmLLMTier: agents ready after configured models became present: %s\n", strings.Join(workloads, ", "))
	return nil
}

func waitForLLMStartup(run helmLLMCommandRunner) error {
	name := helmLLMRelease + "-chatbot-mesh-ollama"
	out, err := run("kubectl", "rollout", "status", "statefulset/"+name,
		"--timeout", helmLLMStartupTimeout.String())
	if err != nil {
		return fmt.Errorf("rollout status statefulset/%s --timeout %s: %w: %s",
			name, helmLLMStartupTimeout, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// assertLLMModelsLoaded polls Ollama's /api/tags until every configured model is
// reported, so the preload contract (models present before the agents query) is
// checked directly rather than inferred from readiness.
func assertLLMModelsLoaded(tagsURL string, models []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		data, status, err := requestHTTP(http.MethodGet, tagsURL, "")
		if err != nil {
			lastErr = err
		} else if status != http.StatusOK {
			lastErr = fmt.Errorf("ollama /api/tags status %d", status)
		} else {
			body := string(data)
			missing := ""
			for _, m := range models {
				if !strings.Contains(body, m) {
					missing = m
					break
				}
			}
			if missing == "" {
				fmt.Printf("helmLLMTier: /api/tags reports all configured models: %s\n", strings.Join(models, ", "))
				return nil
			}
			lastErr = fmt.Errorf("ollama /api/tags missing model %q: %s", missing, strings.TrimSpace(body))
		}
		time.Sleep(3 * time.Second)
	}
	return lastErr
}

type helmDiagnosticRunner func(context.Context, string, ...string) ([]byte, error)

var runHelmDiagnosticCommand helmDiagnosticRunner = func(
	ctx context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

type helmDiagnosticCommand struct {
	label string
	name  string
	args  []string
}

func helmFailureDiagnosticCommands(release string) []helmDiagnosticCommand {
	selector := "app.kubernetes.io/instance=" + release
	return []helmDiagnosticCommand{
		{label: "Helm status", name: "helm", args: []string{"status", release}},
		{label: "pod and workload status", name: "kubectl", args: []string{
			"get", "deploy,statefulset,job,pods,pvc", "-l", selector, "-o", "wide",
		}},
		{label: "persistent volume claims", name: "kubectl", args: []string{
			"get", "pvc", "-o", "wide",
		}},
		{label: "events", name: "kubectl", args: []string{
			"get", "events", "--sort-by=.metadata.creationTimestamp",
		}},
		{label: "container and init status", name: "kubectl", args: []string{
			"get", "pods", "-l", selector, "-o", "json",
		}},
		{label: "pod scheduling and probes", name: "kubectl", args: []string{
			"describe", "pods", "-l", selector,
		}},
		{label: "current container and init logs", name: "kubectl", args: []string{
			"logs", "-l", selector, "--all-containers=true", "--prefix=true", "--tail=120",
		}},
		{label: "previous container and init logs", name: "kubectl", args: []string{
			"logs", "-l", selector, "--all-containers=true", "--prefix=true", "--previous", "--tail=120",
		}},
	}
}

func collectHelmFailureDiagnostics(
	release string,
	run helmDiagnosticRunner,
	timeout time.Duration,
) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var report strings.Builder
	fmt.Fprintf(&report, "%s bounded diagnostics:", release)
	for _, diagnostic := range helmFailureDiagnosticCommands(release) {
		out, err := run(ctx, diagnostic.name, diagnostic.args...)
		fmt.Fprintf(&report, "\n\n== %s ==\n%s",
			diagnostic.label, strings.TrimSpace(string(out)))
		if err != nil {
			fmt.Fprintf(&report, "\n[diagnostic failed: %v]", err)
		} else if ctx.Err() != nil {
			fmt.Fprintf(&report, "\n[diagnostic failed: %v]", ctx.Err())
		}
	}
	return report.String()
}

func captureHelmFailureDiagnostics(
	directory string,
	release string,
	run helmDiagnosticRunner,
	timeout time.Duration,
) string {
	report := collectHelmFailureDiagnostics(release, run, timeout)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return report + fmt.Sprintf(
			"\n[write bounded diagnostics failed: create %s: %v]", directory, err)
	}
	path := filepath.Join(directory, "bounded-diagnostics.txt")
	if err := os.WriteFile(path, []byte(report+"\n"), 0o644); err != nil {
		return report + fmt.Sprintf(
			"\n[write bounded diagnostics failed: %v]", err)
	}
	return report + "\n[evidence: " + path + "]"
}

func helmScenarioEvidenceDirectory(root, cluster, revision string) string {
	return filepath.Join(root, "build", "kind-evidence",
		cluster+"-"+revision+"-"+time.Now().UTC().Format("20060102T150405.000000000Z"))
}

func boundedHelmEvidenceRunner(timeout time.Duration) kindrig.CommandRunner {
	return func(name string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return runHelmDiagnosticCommand(ctx, name, args...)
	}
}
