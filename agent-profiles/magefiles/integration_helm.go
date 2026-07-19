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
)

const (
	helmRelease     = "smoke"
	helmKindCluster = "chatbot-mesh-smoke"
	helmImage       = "declarative-agents/agent-core:smoke"

	helmChatURL    = "http://127.0.0.1:18080/api/v1/chat"
	helmHealthURL  = "http://127.0.0.1:18081/api/lifecycle/health"
	helmJaegerBase = "http://127.0.0.1:16686"

	helmInstallTimeout = 5 * time.Minute
	helmReadyTimeout   = 90 * time.Second
	helmSpanTimeout    = 60 * time.Second
)

// HelmSmoke deploys the chatbot-mesh chart on a disposable kind cluster with the
// ci values and proves the mesh stands up, serves a chat turn, and exports spans
// from more than one service. It gates on docker, kind, helm, and kubectl and on
// an Ollama with the chatbot's configured models, recording a skip reason for
// each missing dependency rather than failing. Teardown (kind delete) runs in
// all paths.
//
// Scope: this is the deploy smoke bar (srd015 R1/R5, uc003 S1). The chatbot's
// in-cluster RAG client base URLs are co-generated from the ragUnits list in
// GH-314; until then the mounted rest.yaml points the RAG clients at loopback, so
// the mesh serves a turn (200 with a non-empty answer) but grounded-with-citation
// retrieval and the repoint/add swap paths land with that co-generation target.
// The span assertion needs each agent to report a distinct service.name, which
// the chart wires (chatbot and each rag unit) so the collector-to-Jaeger pipeline
// surfaces the mesh as more than one service.
func (Integration) HelmSmoke() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	repoRoot := filepath.Dir(profilesRoot)
	coreRoot := envOrDefault(agentCoreRootEnv, filepath.Join(repoRoot, "agent-core"))
	chartDir := filepath.Join(repoRoot, "deploy", "chatbot-mesh")
	if err := requireProfilePaths(profilesRoot,
		"agents/chatbot/profile.yaml", "agents/chatbot/rest.yaml",
		"agents/chroma/rag-server/profile.yaml",
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
	if !agentCoreCheckoutAvailable(coreRoot) {
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

	if err := kindCreateCluster(helmKindCluster); err != nil {
		return err
	}
	defer kindDeleteCluster(helmKindCluster)

	if err := kindLoadImage(helmKindCluster, helmImage); err != nil {
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
// programs into its profiles subtree (the PACKAGING.md step), so the ConfigMap
// carries the chatbot and rag-server profiles the deployment mounts. It returns
// the staged chart path and a cleanup function.
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
	programs := []struct{ src, rel string }{
		{"agents/chatbot", "profiles/agents/chatbot"},
		{"agents/chroma/rag-server", "profiles/agents/chroma/rag-server"},
	}
	for _, p := range programs {
		if err := copyDirContents(filepath.Join(profilesRoot, p.src), filepath.Join(dst, p.rel)); err != nil {
			cleanup()
			return "", nil, err
		}
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

func kindCreateCluster(name string) error {
	if kindClusterExists(name) {
		fmt.Printf("helmSmoke: reusing existing kind cluster %s\n", name)
		return nil
	}
	cmd := exec.Command("kind", "create", "cluster", "--name", name, "--wait", "120s")
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind create cluster %s: %w", name, err)
	}
	return nil
}

func kindClusterExists(name string) bool {
	out, err := exec.Command("kind", "get", "clusters").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

func kindDeleteCluster(name string) {
	cmd := exec.Command("kind", "delete", "cluster", "--name", name)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	_ = cmd.Run()
}

func kindLoadImage(cluster, image string) error {
	cmd := exec.Command("kind", "load", "docker-image", image, "--name", cluster)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind load docker-image %s: %w", image, err)
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
// with a non-empty answer). Grounded-with-citation retrieval is a co-generation
// criterion (GH-314); the deploy smoke bar is that the served machine_request
// endpoint routes a turn through the chatbot in cluster.
func assertSmokeChatServed(url string) error {
	body := `{"message":"Summarize the most relevant record you can retrieve."}`
	data, status, err := requestHTTP(http.MethodPost, url, body)
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
	return lastErr
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
	helmSwapCluster = "chatbot-mesh-swap"
)

// HelmSwap proves the two tiered-swap paths of the chatbot-mesh chart on a kind
// cluster (srd015 R3): repointing a RAG is a Service selector change that does not
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
	repoRoot := filepath.Dir(profilesRoot)
	coreRoot := envOrDefault(agentCoreRootEnv, filepath.Join(repoRoot, "agent-core"))
	chartDir := filepath.Join(repoRoot, "deploy", "chatbot-mesh")
	if _, err := os.Stat(filepath.Join(chartDir, "Chart.yaml")); err != nil {
		return fmt.Errorf("chatbot-mesh chart not found at %s: %w", chartDir, err)
	}
	for _, bin := range []string{"docker", "kind", "helm", "kubectl"} {
		if _, err := exec.LookPath(bin); err != nil {
			fmt.Printf("SKIP helmSwap: %s not found on PATH\n", bin)
			return nil
		}
	}
	if !agentCoreCheckoutAvailable(coreRoot) {
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

	if err := kindCreateCluster(helmSwapCluster); err != nil {
		return err
	}
	defer kindDeleteCluster(helmSwapCluster)
	if err := kindLoadImage(helmSwapCluster, helmImage); err != nil {
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
// no chatbot restart (srd015 R3.1).
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
// (srd015 R3.2).
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
