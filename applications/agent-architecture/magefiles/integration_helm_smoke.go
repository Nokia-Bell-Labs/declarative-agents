// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"github.com/magefile/mage/mg"
)

const (
	smokeCluster        = "da-agent-architecture-smoke"
	smokeRelease        = "smoke"
	smokeNamespace      = "agent-architecture-smoke"
	smokeCollectorImage = kindrig.DefaultAgentCoreImage

	smokeClusterTimeout  = 3 * time.Minute
	smokeInstallTimeout  = 5 * time.Minute
	smokeReadyTimeout    = 2 * time.Minute
	smokeProbeTimeout    = 5 * time.Second
	smokeTraceTimeout    = 60 * time.Second
	smokeDiagTimeout     = 15 * time.Second
	smokeShutdownTimeout = 60 * time.Second
)

// smokeAgentCompletionExitCodes are the container exit codes that count as a
// clean agent shutdown: 0 for a machine that reached its Done state and 2 for
// one that reached a declared terminal (failed) state. This mirrors
// agent-core's run-completion contract used across the mage suites, so a
// lifecycle exit that ends the run either way is treated as clean while a
// crash (OOMKilled, SIGSEGV, etc.) is not.
var smokeAgentCompletionExitCodes = map[int]bool{0: true, 2: true}

// Integration groups bounded integration proofs.
type Integration mg.Namespace

type smokeEnvironment struct {
	kubeconfig string
}

func (environment smokeEnvironment) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = os.Environ()
	if environment.kubeconfig != "" {
		command.Env = append(command.Env, "KUBECONFIG="+environment.kubeconfig)
	}
	return command.CombinedOutput()
}

// HelmSmoke installs the packaged chart into a disposable kind cluster and
// proves the curator and collector reach real readiness, the curator serves its
// documentation interface, the collector retains a trace the curator exports
// under its service name, and a lifecycle-exit request stops the curator
// cleanly. Only missing host prerequisites skip; every terminal path cleans up
// (srd001 R3).
func (Integration) HelmSmoke() error {
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		// A missing catalog or core checkout is a skip, not a failure.
		fmt.Printf("SKIP helmSmoke: %v\n", err)
		return nil
	}
	if reason := smokeSkipReason(resolved); reason != "" {
		fmt.Printf("SKIP helmSmoke: %s\n", reason)
		return nil
	}
	return runHelmSmoke(resolved)
}

func smokeSkipReason(resolved roots) string {
	for _, binary := range []string{"docker", "kind", "helm", "kubectl", "go", "git"} {
		if _, err := exec.LookPath(binary); err != nil {
			return binary + " not found on PATH"
		}
	}
	for _, requirement := range []struct{ path, label string }{
		{filepath.Join(resolved.Catalog, filepath.FromSlash(canonicalProfile)), "canonical documentation-curator profile"},
		{filepath.Join(resolved.Catalog, filepath.FromSlash(collectorProfile)), "canonical collector profile"},
		{filepath.Join(resolved.Core, "go.mod"), "agent-core checkout"},
	} {
		if info, err := os.Stat(requirement.path); err != nil || info.IsDir() {
			return fmt.Sprintf("%s not found at %s", requirement.label, requirement.path)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), smokeProbeTimeout)
	defer cancel()
	if output, err := (smokeEnvironment{}).run(ctx, "docker", "info"); err != nil {
		return fmt.Sprintf("Docker unavailable: %v: %s", err, strings.TrimSpace(string(output)))
	}
	return ""
}

func runHelmSmoke(resolved roots) (result error) {
	// Both workloads run the locally built agent-core image (GH-1368); the
	// application builds no runtime image of its own.
	revision := mustGitRevision(resolved.Application)
	kindConfig := filepath.Join(resolved.Application, "helm", "ci", "kind-config.yaml")
	kindRun := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), smokeClusterTimeout)
		defer cancel()
		return smokeEnvironment{}.run(ctx, "kind", args...)
	}
	cluster, err := kindrig.EnsureCluster(kindRun, smokeCluster, kindConfig, 120*time.Second)
	if err != nil {
		return fmt.Errorf("helmSmoke kind cluster acquisition: %w", err)
	}
	kubeconfig, cleanupKubeconfig, err := smokeKubeconfig(smokeCluster)
	if err != nil {
		cluster.Release(kindRun)
		return fmt.Errorf("helmSmoke kubeconfig: %w", err)
	}
	defer cleanupKubeconfig()
	environment := smokeEnvironment{kubeconfig: kubeconfig}
	defer func() {
		cleanupHelmSmoke(environment, cluster, kindRun, result != nil)
	}()

	if err := prepareSmokeCluster(environment, cluster.Name, resolved); err != nil {
		return smokeFailure(environment.run, "cluster preparation", err)
	}
	archiveDir, err := os.MkdirTemp("", "agent-architecture-smoke-chart-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(archiveDir) }()
	archive, err := packageHelmChart(filepath.Join(resolved.Application, "helm"), resolved.Catalog, archiveDir)
	if err != nil {
		return fmt.Errorf("helmSmoke chart package: %w", err)
	}
	if err := installSmokeChart(environment, archive, resolved.Application); err != nil {
		return smokeFailure(environment.run, "Helm install", err)
	}
	// The collector is a persistent server, so its rollout stabilizes. The
	// canonical curator is a bounded demo agent (its await_curator_control word
	// times out after 30s to a terminal state), so it only reaches Available for
	// one control window per pod rather than staying up; the smoke proves R3.3
	// inside that window rather than requiring sustained availability.
	if err := runSmokeCommand(environment, smokeReadyTimeout, "kubectl", "rollout", "status",
		"deployment/"+smokeRelease+"-agent-architecture-collector", "-n", smokeNamespace, "--timeout=90s"); err != nil {
		return smokeFailure(environment.run, "collector readiness", err)
	}
	queryLocal, err := freeLocalPort()
	if err != nil {
		return err
	}
	collectorForward, err := forwardService(environment, smokeRelease+"-agent-architecture-collector", queryLocal+":18193")
	if err != nil {
		return smokeFailure(environment.run, "collector port-forward", err)
	}
	defer collectorForward.stop()
	queryURL := "http://127.0.0.1:" + queryLocal
	if err := waitHTTP200(queryURL+"/query/traces?page_size=1", smokeReadyTimeout); err != nil {
		return smokeFailure(environment.run, "collector query surface", err)
	}
	// Serve documentation, then stop the curator cleanly with a lifecycle-exit
	// request, all inside one control window; the exit flushes the exporter so
	// the collector retains the spans even after the curator process is gone.
	if err := exerciseCuratorWindow(environment); err != nil {
		return smokeFailure(environment.run, "curator serve and clean exit", err)
	}
	if err := verifyCuratorTrace(queryURL, curatorServiceName); err != nil {
		return smokeFailure(environment.run, "curator trace retention", err)
	}
	fmt.Printf("integration:helmSmoke PASS - revision %s installed the packaged chart; curator served documentation and exited cleanly, collector retained a %s trace\n",
		revision, curatorServiceName)
	return nil
}

// exerciseCuratorWindow catches a live curator pod: it waits for the curator
// Deployment to report Available, serves documentation to emit spans, and posts
// the declarative lifecycle-exit request that stops it cleanly (Done). Because
// the curator recycles every control window, it retries against a fresh window
// if it loses the race.
func exerciseCuratorWindow(environment smokeEnvironment) error {
	service := smokeRelease + "-agent-architecture-curator"
	var lastErr error
	for attempt := 1; attempt <= 4; attempt++ {
		if err := runSmokeCommand(environment, 60*time.Second, "kubectl", "wait",
			"--for=condition=Available", "deployment/"+service,
			"-n", smokeNamespace, "--timeout=45s"); err != nil {
			lastErr = err
			continue
		}
		controlLocal, err := freeLocalPort()
		if err != nil {
			return err
		}
		documentationLocal, err := freeLocalPort()
		if err != nil {
			return err
		}
		forward, err := forwardService(environment, service, controlLocal+":18082", documentationLocal+":18081")
		if err != nil {
			lastErr = err
			continue
		}
		controlURL := "http://127.0.0.1:" + controlLocal
		documentationURL := "http://127.0.0.1:" + documentationLocal
		// Record the addressed pod's identity and restart state *before* the
		// exit request so we can prove this process actually terminates rather
		// than trusting the HTTP acknowledgement alone.
		addressed, err := curatorPodIdentity(environment)
		if err != nil {
			forward.stop()
			lastErr = err
			continue
		}
		if err := runCuratorWindow(controlURL, documentationURL); err != nil {
			forward.stop()
			lastErr = err
			continue
		}
		forward.stop()
		// The clean-shutdown claim is only real if the addressed curator
		// process/pod terminates and releases its listeners; observe it.
		if err := observeCuratorShutdown(environment, addressed, smokeShutdownTimeout); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("curator control window not caught in 4 attempts: %w", lastErr)
}

func runCuratorWindow(controlURL, documentationURL string) error {
	if err := waitHTTP200(controlURL+"/api/lifecycle/health", 20*time.Second); err != nil {
		return err
	}
	if err := driveCuratorDocuments(documentationURL); err != nil {
		return err
	}
	return requestCuratorExit(controlURL)
}

const curatorServiceName = "knowledge-manager-curator"

func mustGitRevision(applicationRoot string) string {
	repositoryRoot := filepath.Clean(filepath.Join(applicationRoot, "..", ".."))
	cmd := exec.Command("git", "-C", repositoryRoot, "rev-parse", "HEAD")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "0000000000000000000000000000000000000000"
	}
	return strings.TrimSpace(string(output))
}

func smokeKubeconfig(cluster string) (string, func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), smokeProbeTimeout)
	defer cancel()
	output, err := smokeEnvironment{}.run(ctx, "kind", "get", "kubeconfig", "--name", cluster)
	if err != nil {
		return "", nil, fmt.Errorf("kind get kubeconfig: %w: %s", err, strings.TrimSpace(string(output)))
	}
	dir, err := os.MkdirTemp("", "agent-architecture-kubeconfig-*")
	if err != nil {
		return "", nil, err
	}
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, output, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, err
	}
	return path, func() { _ = os.RemoveAll(dir) }, nil
}

func prepareSmokeCluster(environment smokeEnvironment, cluster string, resolved roots) error {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	_, _ = environment.run(ctx, "kubectl", "delete", "namespace", smokeNamespace,
		"--ignore-not-found=true", "--wait=true", "--timeout=30s")
	cancel()
	// The curator and collector both run agent-core (GH-1368); build and load one
	// image for both rather than a separate per-app runtime.
	if err := kindrig.BuildAgentCoreImage(resolved.Core, smokeCollectorImage); err != nil {
		return fmt.Errorf("build agent-core image: %w", err)
	}
	kindLoad := func(ctx context.Context, args ...string) ([]byte, error) {
		return smokeEnvironment{}.run(ctx, "kind", args...)
	}
	loadCtx, cancelLoad := context.WithTimeout(context.Background(), smokeClusterTimeout)
	err := kindrig.LoadImage(loadCtx, kindLoad, cluster, smokeCollectorImage)
	cancelLoad()
	if err != nil {
		return err
	}
	return runSmokeCommand(environment, 30*time.Second, "kubectl", "create", "namespace", smokeNamespace)
}

func installSmokeChart(environment smokeEnvironment, archive, applicationRoot string) error {
	repository, tag := splitImageRef(smokeCollectorImage)
	ctx, cancel := context.WithTimeout(context.Background(), smokeInstallTimeout)
	defer cancel()
	output, err := environment.run(ctx, "helm",
		"install", smokeRelease, archive,
		"--namespace", smokeNamespace,
		"--values", filepath.Join(applicationRoot, "helm", "ci", "kind-values.yaml"),
		"--set", "image.repository="+repository,
		"--set", "image.tag="+tag,
		"--set", "collector.image.repository="+repository,
		"--set", "collector.image.tag="+tag,
		// No --wait: the bounded curator never stays ready, so waiting on the
		// whole release would always time out. Readiness is asserted per workload
		// below.
		"--timeout", smokeInstallTimeout.String(),
	)
	if err != nil {
		return fmt.Errorf("helm install: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func splitImageRef(image string) (string, string) {
	index := strings.LastIndex(image, ":")
	if index < 0 || strings.Contains(image[index:], "/") {
		return image, "latest"
	}
	return image[:index], image[index+1:]
}

func runSmokeCommand(environment smokeEnvironment, timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	output, err := environment.run(ctx, name, args...)
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

type portForward struct {
	command *exec.Cmd
}

// forwardService starts a kubectl port-forward for the given local:target pairs
// against a Service. The Service must have a ready endpoint, so a curator
// forward is opened only after the curator Deployment reports Available.
func forwardService(environment smokeEnvironment, service string, pairs ...string) (*portForward, error) {
	args := append([]string{"port-forward", "-n", smokeNamespace, "service/" + service}, pairs...)
	command := exec.Command("kubectl", args...)
	command.Env = append(os.Environ(), "KUBECONFIG="+environment.kubeconfig)
	command.Stdout, command.Stderr = os.Stderr, os.Stderr
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start port-forward %s: %w", service, err)
	}
	return &portForward{command: command}, nil
}

func (forward *portForward) stop() {
	if forward == nil || forward.command == nil {
		return
	}
	if forward.command.Process != nil {
		_ = forward.command.Process.Kill()
	}
	_ = forward.command.Wait()
}

func freeLocalPort() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer func() { _ = listener.Close() }()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return "", err
	}
	return port, nil
}

func waitHTTP200(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 3 * time.Second}
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("GET %s status %d", url, resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("no HTTP 200 from %s within %s: %w", url, timeout, lastErr)
}

// driveCuratorDocuments hits the curator's documentation interface, proving it
// serves and driving a machine_request whose spans the curator exports to the
// collector.
func driveCuratorDocuments(documentationURL string) error {
	if err := waitHTTP200(documentationURL+"/api/v1/docs", 15*time.Second); err != nil {
		return err
	}
	// A few requests give the exporter something to batch and flush.
	client := &http.Client{Timeout: 3 * time.Second}
	for index := 0; index < 3; index++ {
		resp, err := client.Get(documentationURL + "/api/v1/docs")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil
}

type collectorTraceSummary struct {
	TraceID string `json:"trace_id"`
}

type collectorTraceList struct {
	Traces []collectorTraceSummary `json:"traces"`
	Total  int                     `json:"total"`
}

type collectorSpan struct {
	Service string `json:"service"`
}

type collectorTraceDetail struct {
	TraceID   string          `json:"trace_id"`
	Spans     []collectorSpan `json:"spans"`
	SpanCount int             `json:"span_count"`
}

// verifyCuratorTrace polls the collector query surface until a retained trace
// carries a span under the curator service name (srd001 R3.4).
func verifyCuratorTrace(queryURL, service string) error {
	deadline := time.Now().Add(smokeTraceTimeout)
	client := &http.Client{Timeout: 3 * time.Second}
	var lastErr error
	for time.Now().Before(deadline) {
		list, err := getCollectorTraceList(client, queryURL+"/query/traces?page_size=50")
		if err != nil {
			lastErr = err
			time.Sleep(time.Second)
			continue
		}
		for _, summary := range list.Traces {
			detail, err := getCollectorTraceDetail(client, queryURL+"/query/traces/"+summary.TraceID)
			if err != nil {
				continue
			}
			for _, span := range detail.Spans {
				if span.Service == service {
					return nil
				}
			}
		}
		lastErr = fmt.Errorf("no retained trace carries service %q (traces so far: %d)", service, list.Total)
		time.Sleep(time.Second)
	}
	return fmt.Errorf("collector did not retain a %q trace within %s: %w", service, smokeTraceTimeout, lastErr)
}

func getCollectorTraceList(client *http.Client, url string) (*collectorTraceList, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s status %d", url, resp.StatusCode)
	}
	var list collectorTraceList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode trace list: %w", err)
	}
	return &list, nil
}

func getCollectorTraceDetail(client *http.Client, url string) (*collectorTraceDetail, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s status %d", url, resp.StatusCode)
	}
	var detail collectorTraceDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, fmt.Errorf("decode trace detail: %w", err)
	}
	return &detail, nil
}

// requestCuratorExit posts the declarative lifecycle-exit request and accepts
// the curator's acknowledgement (srd001 R3.3).
func requestCuratorExit(controlURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(controlURL+"/api/lifecycle/exit", "application/json",
		strings.NewReader(`{"reason":"helm smoke"}`))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("lifecycle exit status %d", resp.StatusCode)
	}
	return nil
}

// curatorPod is the identity and restart state of the addressed curator pod,
// captured before the exit request so shutdown can be attributed to it.
type curatorPod struct {
	name     string
	uid      string
	restarts int
}

// kubePodList / kubePod / kubeContainerStatus mirror the subset of the
// kubectl pod JSON needed to attribute a clean shutdown to the addressed pod.
type kubePodList struct {
	Items []kubePod `json:"items"`
}

type kubePod struct {
	Metadata struct {
		Name string `json:"name"`
		UID  string `json:"uid"`
	} `json:"metadata"`
	Status struct {
		Phase             string                `json:"phase"`
		ContainerStatuses []kubeContainerStatus `json:"containerStatuses"`
	} `json:"status"`
}

type kubeContainerStatus struct {
	Name         string `json:"name"`
	RestartCount int    `json:"restartCount"`
	State        struct {
		Terminated *kubeTerminated `json:"terminated,omitempty"`
	} `json:"state"`
	LastState struct {
		Terminated *kubeTerminated `json:"terminated,omitempty"`
	} `json:"lastState"`
}

type kubeTerminated struct {
	ExitCode int    `json:"exitCode"`
	Reason   string `json:"reason"`
}

// curatorPodIdentity resolves the currently running curator pod and records its
// UID and application-container restart count.
func curatorPodIdentity(environment smokeEnvironment) (curatorPod, error) {
	ctx, cancel := context.WithTimeout(context.Background(), smokeProbeTimeout)
	defer cancel()
	out, err := environment.run(ctx, "kubectl", "get", "pods",
		"-l", "app.kubernetes.io/component=curator", "-n", smokeNamespace, "-o", "json")
	if err != nil {
		return curatorPod{}, fmt.Errorf("get curator pods: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return selectCuratorPod(out)
}

// selectCuratorPod picks the running curator pod from a kubectl pod-list JSON
// document and returns its identity and application-container restart count. It
// is pure so the selection logic is unit-tested without a cluster.
func selectCuratorPod(raw []byte) (curatorPod, error) {
	var list kubePodList
	if err := json.Unmarshal(raw, &list); err != nil {
		return curatorPod{}, fmt.Errorf("decode curator pod list: %w", err)
	}
	for _, pod := range list.Items {
		if pod.Status.Phase != "Running" {
			continue
		}
		container := curatorContainer(pod.Status.ContainerStatuses)
		return curatorPod{name: pod.Metadata.Name, uid: pod.Metadata.UID, restarts: container.RestartCount}, nil
	}
	return curatorPod{}, fmt.Errorf("no running curator pod found")
}

// curatorContainer selects the application container from a pod's statuses,
// preferring a curator/agent-named container and falling back to the first.
func curatorContainer(statuses []kubeContainerStatus) kubeContainerStatus {
	for _, status := range statuses {
		if strings.Contains(status.Name, "curator") || strings.Contains(status.Name, "agent") {
			return status
		}
	}
	if len(statuses) > 0 {
		return statuses[0]
	}
	return kubeContainerStatus{}
}

// observeCuratorShutdown blocks until the addressed curator pod is observed to
// terminate cleanly, or the timeout elapses. Disappearance of the pod identity,
// a restart of the addressed container with an accepted exit code, or a fully
// terminated container all satisfy the clean-shutdown contract; a terminated
// container with a crash exit code fails immediately.
func observeCuratorShutdown(environment smokeEnvironment, prior curatorPod, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), smokeProbeTimeout)
		out, err := environment.run(ctx, "kubectl", "get", "pod", prior.name,
			"-n", smokeNamespace, "-o", "json")
		cancel()
		if err != nil {
			// The addressed pod's identity is gone: it terminated and the
			// Deployment is recreating it, so its former listeners are released.
			if isKubectlNotFound(out) {
				return nil
			}
			lastErr = fmt.Errorf("get curator pod %s: %w: %s", prior.name, err, strings.TrimSpace(string(out)))
			time.Sleep(2 * time.Second)
			continue
		}
		done, err := curatorShutdownObserved(out, prior)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("curator pod %s did not terminate within %s after the accepted exit request",
			prior.name, timeout)
	}
	return lastErr
}

// curatorShutdownObserved interprets a single-pod kubectl JSON document against
// the previously addressed pod. It returns (true, nil) when the addressed
// process is proven to have terminated cleanly, (false, nil) when it is still
// running, and a non-nil error when it terminated with a crash exit code. It is
// pure so the attribution logic is unit-tested without a cluster.
func curatorShutdownObserved(raw []byte, prior curatorPod) (bool, error) {
	var pod kubePod
	if err := json.Unmarshal(raw, &pod); err != nil {
		return false, fmt.Errorf("decode curator pod %s: %w", prior.name, err)
	}
	if prior.uid != "" && pod.Metadata.UID != "" && pod.Metadata.UID != prior.uid {
		// Same name, new UID: the addressed pod was replaced, i.e. gone.
		return true, nil
	}
	container := curatorContainer(pod.Status.ContainerStatuses)
	// A restart proves the addressed process exited; inspect how it exited.
	if container.RestartCount > prior.restarts {
		if terminated := container.LastState.Terminated; terminated != nil {
			return curatorTerminationClean(terminated)
		}
		return true, nil
	}
	// No restart, but the container may have terminated for good (Succeeded).
	if terminated := container.State.Terminated; terminated != nil {
		return curatorTerminationClean(terminated)
	}
	return false, nil
}

// curatorTerminationClean maps a terminated container state onto the clean
// shutdown contract.
func curatorTerminationClean(terminated *kubeTerminated) (bool, error) {
	if smokeAgentCompletionExitCodes[terminated.ExitCode] {
		return true, nil
	}
	return false, fmt.Errorf("curator terminated uncleanly: exit code %d (%s)",
		terminated.ExitCode, terminated.Reason)
}

// isKubectlNotFound reports whether kubectl output indicates the resource is
// absent, which for the addressed pod means it terminated and was reaped.
func isKubectlNotFound(out []byte) bool {
	return strings.Contains(string(out), "NotFound") || strings.Contains(string(out), "not found")
}

func smokeFailure(run func(context.Context, string, ...string) ([]byte, error), step string, cause error) error {
	diagnostics := collectSmokeDiagnostics(run)
	return fmt.Errorf("helmSmoke %s failure: %w\n%s", step, cause, diagnostics)
}

func collectSmokeDiagnostics(run func(context.Context, string, ...string) ([]byte, error)) string {
	ctx, cancel := context.WithTimeout(context.Background(), smokeDiagTimeout)
	defer cancel()
	commands := []struct {
		label string
		args  []string
	}{
		{"helm status", []string{"status", smokeRelease, "-n", smokeNamespace}},
		{"workloads", []string{"get", "deploy,pods,cm,svc", "-n", smokeNamespace, "-o", "wide"}},
		{"events", []string{"get", "events", "-n", smokeNamespace, "--sort-by=.metadata.creationTimestamp"}},
		{"curator logs", []string{"logs", "-n", smokeNamespace, "-l", "app.kubernetes.io/component=curator", "--tail=80"}},
		{"collector logs", []string{"logs", "-n", smokeNamespace, "-l", "app.kubernetes.io/component=collector", "--tail=80"}},
	}
	var report strings.Builder
	report.WriteString("helmSmoke bounded diagnostics:")
	for _, command := range commands {
		name := "kubectl"
		if command.label == "helm status" {
			name = "helm"
		}
		output, err := run(ctx, name, command.args...)
		fmt.Fprintf(&report, "\n\n== %s ==\n%s", command.label, strings.TrimSpace(string(output)))
		if err != nil {
			fmt.Fprintf(&report, "\n[diagnostic failed: %v]", err)
		}
	}
	return report.String()
}

func cleanupHelmSmoke(environment smokeEnvironment, cluster kindrig.Cluster, kindRun kindrig.Runner, failed bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	_, _ = environment.run(ctx, "helm", "uninstall", smokeRelease, "-n", smokeNamespace, "--wait", "--timeout=20s")
	cancel()
	ctx, cancel = context.WithTimeout(context.Background(), 40*time.Second)
	_, _ = environment.run(ctx, "kubectl", "delete", "namespace", smokeNamespace,
		"--ignore-not-found=true", "--wait=true", "--timeout=30s")
	cancel()
	cluster.Release(kindRun)
}
