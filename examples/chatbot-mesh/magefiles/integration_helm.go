// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

const (
	helmRelease     = "smoke"
	helmKindCluster = "da-chatbot-mesh-smoke"
	helmImage       = "declarative-agents/agent-core:smoke"

	helmChatURL    = "http://127.0.0.1:18080/api/v1/chat"
	helmHealthURL  = "http://127.0.0.1:18081/api/lifecycle/health"
	helmJaegerBase = "http://127.0.0.1:16686"

	helmInstallTimeout = 5 * time.Minute
	helmClusterWait    = 120 * time.Second
	helmReadyTimeout   = 90 * time.Second
	helmSpanTimeout    = 60 * time.Second
)

// exampleChartDir returns the chatbot-mesh Helm chart under the example, which
// ships with the example rather than as a sibling deploy directory.
func exampleChartDir(profilesRoot string) string {
	return filepath.Join(profilesRoot, "helm")
}

// helmKindConfig is the checked-in cluster configuration the helm scenarios
// share; it pins the node image so every machine creates the same cluster
// (eng01). It sits in the source chart's ci directory beside the kind values,
// not in the staged copy, so staging cannot drift it.
func helmKindConfig(chartDir string) string {
	return filepath.Join(chartDir, "ci", "kind-config.yaml")
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
// wires (chatbot and each rag unit) so the collector-to-Jaeger pipeline surfaces
// the mesh as more than one service.
func (Integration) HelmSmoke() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault(agentCoreRootEnv, siblingPath(profilesRoot, "agent-core"))
	chartDir := exampleChartDir(profilesRoot)
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
		return fmt.Sprintf("agent-core checkout not found at %s (set %s)", coreRoot, agentCoreRootEnv)
	}
	return chatbotOllamaSkipReason(profilesRoot)
}

func runHelmSmoke(coreRoot, profilesRoot, chartDir string) error {
	fmt.Printf("helmSmoke: building runtime image %s from %s\n", helmImage, coreRoot)
	if err := buildSmokeRuntimeImage(coreRoot, helmImage); err != nil {
		return err
	}
	stagedChart, cleanupChart, err := stageSmokeChart(chartDir, profilesRoot)
	if err != nil {
		return err
	}
	defer cleanupChart()

	cluster, err := kindrig.EnsureCluster(kindrig.DefaultRun, helmKindCluster, helmKindConfig(chartDir), helmClusterWait)
	if err != nil {
		return err
	}
	defer cluster.Release(kindrig.DefaultRun)

	if err := kindrig.LoadImage(helmKindCluster, helmImage); err != nil {
		return err
	}
	if err := helmInstallSmoke(stagedChart, helmImage); err != nil {
		return err
	}

	stop, err := kubectlPortForward("svc/"+helmRelease+"-chatbot-mesh-chatbot", 18080, 18081)
	if err != nil {
		return err
	}
	defer stop()
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

	stopJaeger, err := kubectlPortForward("svc/"+helmRelease+"-chatbot-mesh-jaeger", 16686)
	if err != nil {
		return err
	}
	defer stopJaeger()
	if err := assertSmokeSpans(helmJaegerBase, 2, helmSpanTimeout); err != nil {
		return err
	}

	fmt.Println("integration:helmSmoke PASS - chart deployed the mesh on kind, the chatbot served a turn, and Jaeger reported spans from more than one service")
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
	defer os.RemoveAll(ctxDir)

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
	dockerfile := "FROM alpine:3.22\n" +
		"RUN apk add --no-cache ca-certificates bash\n" +
		"COPY agent /usr/local/bin/agent\n" +
		"COPY tools /opt/agent-core/tools\n" +
		"ENV AGENT_CORE_HOME=/opt/agent-core HOME=/tmp PATH=/usr/local/bin:/usr/bin:/bin\n" +
		"ENTRYPOINT [\"agent\"]\n"
	if err := os.WriteFile(filepath.Join(ctxDir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		return fmt.Errorf("write smoke Dockerfile: %w", err)
	}
	docker := exec.Command("docker", "build", "-t", image, ".")
	docker.Dir = ctxDir
	docker.Stdout, docker.Stderr = os.Stderr, os.Stderr
	if err := docker.Run(); err != nil {
		return fmt.Errorf("docker build %s: %w", image, err)
	}
	return nil
}

// stageSmokeChart copies the chart to a temp directory and stages the agent
// programs and the ux artifacts into its profiles subtree (the PACKAGING.md
// step), so the ConfigMap carries the agent profiles and the SPA bundle the
// chatbot serves. It returns the staged chart path and a cleanup function.
func stageSmokeChart(chartDir, profilesRoot string) (string, func(), error) {
	staged, err := os.MkdirTemp("", "chatbot-mesh-chart-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(staged) }
	dst := filepath.Join(staged, "chatbot-mesh")
	if err := copyDirContents(chartDir, dst); err != nil {
		cleanup()
		return "", nil, err
	}
	for _, p := range chartProfilePrograms() {
		if err := stageProfilePath(filepath.Join(profilesRoot, p.src), filepath.Join(dst, p.rel)); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	return dst, cleanup, nil
}

// chartProfileProgram is one source-to-staged mapping copied into the packaged
// chart's profiles subtree before helm package/install. src names either a
// directory or a single file.
type chartProfileProgram struct{ src, rel string }

// chartProfilePrograms is the single authoritative list of agent programs and
// ux artifacts staged into the chart's profiles ConfigMap. It MUST cover every
// agent profile mounted by an enabled Deployment (see helm/templates/*.yaml);
// the executor (srd006) Deployment mounts agents/executor/profile.yaml, so
// omitting it here left an enabled executor with no profile to start (GH-485).
// TestStagedProfilesCoverEnabledDeployments enforces the coverage.
//
// The ux contributes two entries rather than its whole tree, because every file
// staged here becomes a ConfigMap key and a projected mount item in every agent
// pod (profiles-configmap.yaml and profilesVolume both glob profiles/**). The
// chart consumes exactly two things from the ux: ux.yaml, the UI descriptor, and
// ux/app/dist, the built bundle the chatbot's static_assets binding serves at
// /ui (agents/chatbot/rest.yaml). Staging the tree also carried the panel
// sources, the tsconfig, and a 60 KiB package-lock.json into every pod, and it
// swept in node_modules whenever a developer had run npm install -- which helm
// rejects outright, since esbuild's binary is over the 5 MiB per-file chart
// limit (GH-702).
func chartProfilePrograms() []chartProfileProgram {
	return []chartProfileProgram{
		{"agents/chatbot", "profiles/agents/chatbot"},
		{"agents/rag-server", "profiles/agents/rag-server"},
		{"agents/coordinator", "profiles/agents/coordinator"},
		{"agents/creator", "profiles/agents/creator"},
		{"agents/executor", "profiles/agents/executor"},
		{"ux/ux.yaml", "profiles/ux/ux.yaml"},
		{"ux/app/dist", "profiles/ux/app/dist"},
	}
}

// stageProfilePath copies one staging entry, whether it names a directory or a
// single file.
func stageProfilePath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stage %s: %w", src, err)
	}
	if info.IsDir() {
		return copyDirContents(src, dst)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(dst), err)
	}
	cmd := exec.Command("cp", "-a", src, dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("copy %s -> %s: %s: %w", src, dst, strings.TrimSpace(string(out)), err)
	}
	return nil
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

func helmInstallSmoke(chartPath, image string) error {
	repo, tag := splitImageRef(image)
	cmd := exec.Command("helm", "install", helmRelease, chartPath,
		"--values", filepath.Join(chartPath, "ci", "kind-values.yaml"),
		"--set", "image.repository="+repo,
		"--set", "image.tag="+tag,
		"--set", "image.pullPolicy=Never",
		"--set", "llm.externalURL=http://host.docker.internal:11434",
		"--wait", "--timeout", helmInstallTimeout.String(),
	)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm install %s: %w", helmRelease, err)
	}
	return nil
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
// a collector that never started surfaced only as an empty Jaeger service list
// after the span timeout -- a symptom two hops from the cause.
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

// assertSmokeSpans queries Jaeger for the services that have reported spans and
// asserts at least minServices agent services appear, retrying while the export
// pipeline flushes. Jaeger's own service name is not counted.
func assertSmokeSpans(jaegerBase string, minServices int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		n, services, err := jaegerAgentServices(jaegerBase)
		if err != nil {
			lastErr = err
		} else if n >= minServices {
			fmt.Printf("helmSmoke: Jaeger reported %d agent services: %s\n", n, strings.Join(services, ", "))
			return nil
		} else {
			lastErr = fmt.Errorf("jaeger reported %d agent services (%v), want >= %d", n, services, minServices)
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("%w%s", lastErr, collectorDiagnostics(helmRelease))
}

// jaegerAgentServices returns the services Jaeger has traces for, excluding
// Jaeger's own internal service.
func jaegerAgentServices(jaegerBase string) (int, []string, error) {
	data, status, err := requestHTTP(http.MethodGet, jaegerBase+"/api/services", "")
	if err != nil {
		return 0, nil, err
	}
	if status != http.StatusOK {
		return 0, nil, fmt.Errorf("jaeger /api/services status %d", status)
	}
	var resp struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, nil, fmt.Errorf("decode jaeger services: %w", err)
	}
	var agents []string
	for _, s := range resp.Data {
		if s == "jaeger-all-in-one" || s == "jaeger" {
			continue
		}
		agents = append(agents, s)
	}
	return len(agents), agents, nil
}

const (
	helmSwapRelease = "swap"
	helmSwapCluster = "da-chatbot-mesh-swap"
)

// HelmSwap proves the two tiered-swap paths of the chatbot-mesh chart on a kind
// cluster (srd003 R3): repointing a RAG is a Service selector change that does not
// roll the chatbot (R3.1), and adding a RAG re-renders the co-generated profile and
// rolls the chatbot (R3.2). It asserts the infrastructure contracts (pod identity,
// workload existence, the co-generated ConfigMap breadth) via kubectl and helm, so
// it needs docker, kind, helm, and kubectl but not Ollama: pod readiness is the
// health endpoint, and grounded retrieval is the deploy-smoke's concern. Teardown
// (kind delete) runs in all paths.
func (Integration) HelmSwap() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault(agentCoreRootEnv, siblingPath(profilesRoot, "agent-core"))
	chartDir := exampleChartDir(profilesRoot)
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
		fmt.Printf("SKIP helmSwap: agent-core checkout not found at %s (set %s)\n", coreRoot, agentCoreRootEnv)
		return nil
	}
	return runHelmSwap(coreRoot, profilesRoot, chartDir)
}

func runHelmSwap(coreRoot, profilesRoot, chartDir string) error {
	fmt.Printf("helmSwap: building runtime image %s\n", helmImage)
	if err := buildSmokeRuntimeImage(coreRoot, helmImage); err != nil {
		return err
	}
	stagedChart, cleanupChart, err := stageSmokeChart(chartDir, profilesRoot)
	if err != nil {
		return err
	}
	defer cleanupChart()

	swapCluster, err := kindrig.EnsureCluster(kindrig.DefaultRun, helmSwapCluster, helmKindConfig(chartDir), helmClusterWait)
	if err != nil {
		return err
	}
	defer swapCluster.Release(kindrig.DefaultRun)
	if err := kindrig.LoadImage(helmSwapCluster, helmImage); err != nil {
		return err
	}

	// Deploy with one RAG unit (the ci values default).
	if err := helmSwapDeploy(stagedChart, "install", nil); err != nil {
		return err
	}

	if err := assertSwapRepoint(); err != nil {
		return err
	}
	if err := assertSwapAddRag(stagedChart); err != nil {
		return err
	}
	fmt.Println("integration:helmSwap PASS - repoint left the chatbot pod unchanged; adding a RAG re-rendered the co-generated profile, rolled the chatbot, and stood up the new RAG unit")
	return nil
}

// helmSwapDeploy installs or upgrades the release with the given extra --set args.
func helmSwapDeploy(chartPath, verb string, extra []string) error {
	repo, tag := splitImageRef(helmImage)
	args := []string{verb, helmSwapRelease, chartPath,
		"--values", filepath.Join(chartPath, "ci", "kind-values.yaml"),
		"--set", "image.repository=" + repo,
		"--set", "image.tag=" + tag,
		"--set", "image.pullPolicy=Never",
		"--wait", "--timeout", helmInstallTimeout.String(),
	}
	args = append(args, extra...)
	cmd := exec.Command("helm", args...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm %s %s: %w", verb, helmSwapRelease, err)
	}
	return nil
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
	cmd := exec.Command("kubectl", "patch", "service", svc, "-p", patch)
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
	return nil
}

// assertSwapAddRag upgrades the release to two RAG units and asserts the new RAG
// workload stands up, the co-generated chatbot profile ConfigMap now carries the
// second RAG client, and the chatbot Deployment rolled to a new generation
// (srd003 R3.2).
func assertSwapAddRag(chartPath string) error {
	genBefore, err := chatbotDeploymentGeneration()
	if err != nil {
		return err
	}
	extra := []string{
		"--set", "ragUnits[1].name=rag1",
		"--set", "ragUnits[1].collection=corpus2",
		"--set", "ragUnits[1].embeddingModel=qwen3-embedding:8b",
		"--set", "ragUnits[1].replicas=1",
	}
	if err := helmSwapDeploy(chartPath, "upgrade", extra); err != nil {
		return err
	}
	if err := kubectlResourceExists("deployment", helmSwapRelease+"-chatbot-mesh-rag1"); err != nil {
		return fmt.Errorf("add-RAG did not stand up the rag1 Deployment: %w", err)
	}
	cm, err := kubectlConfigMapKey(helmSwapRelease+"-chatbot-mesh-profiles", "agents__chatbot__rest.yaml")
	if err != nil {
		return err
	}
	if !strings.Contains(cm, "rag1:") {
		return fmt.Errorf("add-RAG did not re-render the chatbot rest.yaml with the rag1 client (R2/R3.2)")
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
	helmLLMInstallTimeout = 20 * time.Minute
	helmLLMReadyTimeout   = 3 * time.Minute
)

// helmLLMModels are the CPU-only small models the kind LLM-tier values pull; the
// assertion confirms /api/tags reports each one after preload.
var helmLLMModels = []string{"all-minilm", "qwen2.5:0.5b"}

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
	coreRoot := envOrDefault(agentCoreRootEnv, siblingPath(profilesRoot, "agent-core"))
	chartDir := exampleChartDir(profilesRoot)
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
		fmt.Printf("SKIP helmLLMTier: agent-core checkout not found at %s (set %s)\n", coreRoot, agentCoreRootEnv)
		return nil
	}
	return runHelmLLMTier(coreRoot, profilesRoot, chartDir)
}

func runHelmLLMTier(coreRoot, profilesRoot, chartDir string) error {
	fmt.Printf("helmLLMTier: building runtime image %s\n", helmImage)
	if err := buildSmokeRuntimeImage(coreRoot, helmImage); err != nil {
		return err
	}
	stagedChart, cleanupChart, err := stageSmokeChart(chartDir, profilesRoot)
	if err != nil {
		return err
	}
	defer cleanupChart()

	llmCluster, err := kindrig.EnsureCluster(kindrig.DefaultRun, helmLLMCluster, helmKindConfig(chartDir), helmClusterWait)
	if err != nil {
		return err
	}
	defer llmCluster.Release(kindrig.DefaultRun)
	if err := kindrig.LoadImage(helmLLMCluster, helmImage); err != nil {
		return err
	}
	if err := helmInstallLLM(stagedChart); err != nil {
		return err
	}

	// Ollama must serve before the suspended preload Job can be resumed; agent
	// readiness remains blocked until the transition proof below completes.
	if err := kubectlRolloutStatus("statefulset", helmLLMRelease+"-chatbot-mesh-ollama", helmLLMReadyTimeout); err != nil {
		return fmt.Errorf("ollama StatefulSet did not become ready: %w", err)
	}
	workloads, err := beginLLMPreloadTransition(runHelmLLMCommand)
	if err != nil {
		return err
	}

	stopTags, err := kubectlPortForward("svc/"+helmLLMRelease+"-chatbot-mesh-ollama", 11434)
	if err != nil {
		return err
	}
	if err := assertLLMModelsLoaded(helmLLMTagsURL, helmLLMModels, helmLLMReadyTimeout); err != nil {
		stopTags()
		return err
	}
	stopTags()
	if err := finishLLMPreloadTransition(runHelmLLMCommand, workloads); err != nil {
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

	fmt.Println("integration:helmLLMTier PASS - the chart stood up the in-cluster Ollama tier, the preload Job pulled the configured models, /api/tags reported them, and the chatbot served a turn against the in-cluster endpoint")
	return nil
}

func helmInstallLLM(chartPath string) error {
	return helmInstallLLMWithRunner(chartPath, runHelmLLMCommand)
}

func helmInstallLLMWithRunner(chartPath string, run helmLLMCommandRunner) error {
	repo, tag := splitImageRef(helmImage)
	args := []string{"install", helmLLMRelease, chartPath,
		"--values", filepath.Join(chartPath, "ci", "kind-llm-values.yaml"),
		"--set", "image.repository=" + repo,
		"--set", "image.tag=" + tag,
		"--set", "image.pullPolicy=Never",
		"--set", "ollama.preload.suspend=true",
		"--timeout", helmLLMInstallTimeout.String(),
	}
	out, err := run("helm", args...)
	if len(out) > 0 {
		_, _ = os.Stderr.Write(out)
	}
	if err != nil {
		return fmt.Errorf("helm install %s: %w", helmLLMRelease, err)
	}
	return nil
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
		"--timeout", helmLLMReadyTimeout.String()); err != nil {
		return nil, fmt.Errorf("preload Job did not complete: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return names, nil
}

func finishLLMPreloadTransition(run helmLLMCommandRunner, workloads []string) error {
	for _, name := range workloads {
		if out, err := run("kubectl", "rollout", "status", "deployment/"+name,
			"--timeout", helmLLMReadyTimeout.String()); err != nil {
			return fmt.Errorf("agent workload %s did not become ready after model preload: %w: %s",
				name, err, strings.TrimSpace(string(out)))
		}
	}
	fmt.Printf("helmLLMTier: agents ready after configured models became present: %s\n", strings.Join(workloads, ", "))
	return nil
}

func kubectlRolloutStatus(kind, name string, timeout time.Duration) error {
	cmd := exec.Command("kubectl", "rollout", "status", kind+"/"+name, "--timeout", timeout.String())
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

func waitJobComplete(name string, timeout time.Duration) error {
	cmd := exec.Command("kubectl", "wait", "--for=condition=complete", "job/"+name, "--timeout", timeout.String())
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
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
