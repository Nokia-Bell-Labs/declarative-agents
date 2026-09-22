// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

func prepareCodingHelmCluster(
	environment codingSmokeEnvironment,
	cluster string,
	roots integrationRoots,
	image codingHelmImage,
) error {
	// The caller owns the namespace. The workspace volume is cluster-scoped, so
	// a leftover from an interrupted run is cleared here.
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	_, _ = environment.run(ctx, "kubectl", "delete", "persistentvolume", codingWorkspaceVolume,
		"--ignore-not-found=true", "--wait=true", "--timeout=30s")
	cancel()
	ctx, cancel = context.WithTimeout(context.Background(), codingHelmProbeTimeout)
	output, err := codingSmokeEnvironment{}.run(ctx, "docker", "exec",
		cluster+"-control-plane", "sh", "-c",
		"rm -rf /tmp/coding-agent-workspace && mkdir -p /tmp/coding-agent-workspace && chmod 0777 /tmp/coding-agent-workspace")
	cancel()
	if err != nil {
		return fmt.Errorf("prepare kind workspace: %w: %s", err, strings.TrimSpace(string(output)))
	}
	// The caller holds the canonical image lease. This function only delivers
	// that immutable result into the cluster; it never creates an unowned tag.
	kindRun := func(ctx context.Context, args ...string) ([]byte, error) {
		return codingSmokeEnvironment{}.run(ctx, "kind", args...)
	}
	ctx, cancel = context.WithTimeout(context.Background(), codingHelmClusterTimeout)
	err = kindrig.LoadImage(ctx, kindRun, cluster, image.Reference)
	cancel()
	if err != nil {
		return err
	}
	for _, donor := range []struct {
		source string
		local  string
	}{
		{source: codingHelmGoDonorImage, local: codingHelmGoDonorLocal},
		{source: codingHelmLintDonorImage, local: codingHelmLintDonorLocal},
	} {
		if err := loadCodingDependencyImage(cluster, donor.source, donor.local); err != nil {
			return &codingHelmInfrastructureError{
				Step: "executor donor image load", Cause: err,
			}
		}
	}
	if err := runCodingSmokeCommand(environment, 30*time.Second,
		"kubectl", "apply", "--namespace", codingHelmNamespace, "-f",
		filepath.Join(roots.Application, "helm", "ci", "kind-workspace.yaml")); err != nil {
		return err
	}
	modelManifest, cleanup, err := codingModelMockManifest(roots, image.Reference)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := runCodingSmokeCommand(environment, 30*time.Second,
		"kubectl", "apply", "--namespace", codingHelmNamespace, "-f", modelManifest); err != nil {
		return err
	}
	return runCodingSmokeCommand(environment, codingHelmReadyTimeout,
		"kubectl", "rollout", "status", "deployment/coding-model",
		"-n", codingHelmNamespace, "--timeout=90s")
}

func loadCodingDependencyImage(cluster, source, localReference string) error {
	ctx, cancel := context.WithTimeout(context.Background(), codingHelmClusterTimeout)
	defer cancel()
	// The preflight verifies the digest-qualified source. A platform-specific
	// OCI import cannot retain the registry manifest-list digest, so index-name
	// assigns a rig-local reference that a Never-pull pod can resolve without
	// adding or deleting host tags.
	save := exec.CommandContext(ctx, "docker", "save", source)
	stream, err := save.StdoutPipe()
	if err != nil {
		return err
	}
	node := cluster + "-control-plane"
	load := exec.CommandContext(ctx, "docker", "exec", "-i", node,
		"ctr", "--namespace=k8s.io", "images", "import",
		"--platform=linux/"+runtime.GOARCH, "--snapshotter=overlayfs",
		"--index-name", localReference, "-")
	load.Stdin = stream
	var output bytes.Buffer
	load.Stdout, load.Stderr = &output, &output
	if err := load.Start(); err != nil {
		return err
	}
	if err := save.Run(); err != nil {
		_ = load.Process.Kill()
		_ = load.Wait()
		return fmt.Errorf("docker save %s: %w", source, err)
	}
	if err := load.Wait(); err != nil {
		return fmt.Errorf("import %s as %s: %w: %s",
			source, localReference, err, strings.TrimSpace(output.String()))
	}
	return nil
}

func installCodingHelmChart(
	environment codingSmokeEnvironment,
	archive, applicationRoot, image string,
) error {
	return installCodingHelmChartWithRunner(
		environment.run, archive, applicationRoot, image)
}

func installCodingHelmChartWithRunner(
	run codingSmokeRunner,
	archive, applicationRoot, image string,
) error {
	repository, tag := splitCodingImageRef(image)
	ctx, cancel := context.WithTimeout(context.Background(), codingHelmInstallTimeout)
	defer cancel()
	output, err := run(ctx, "helm",
		"install", codingHelmRelease, archive,
		"--namespace", codingHelmNamespace,
		"--values", filepath.Join(applicationRoot, "helm", "ci", "kind-values.yaml"),
		"--set", "image.repository="+repository,
		"--set-string", "image.tag="+tag,
		"--wait", "--timeout", codingHelmInstallTimeout.String(),
	)
	if err != nil {
		return fmt.Errorf("helm install: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func splitCodingImageRef(image string) (string, string) {
	index := strings.LastIndex(image, ":")
	if index < 0 || strings.Contains(image[index:], "/") {
		return image, "latest"
	}
	return image[:index], image[index+1:]
}

// verifyCodingHelmRollouts waits for every role Deployment the install created.
// The smoke install enables planner, executor, critic, and collector; the live
// applier tier passes "applier" as an extra so its Deployment is waited on too,
// without the smoke waiting on a Deployment it never installs.
func verifyCodingHelmRollouts(environment codingSmokeEnvironment, extra ...string) error {
	components := append([]string{"planner", "executor", "critic", "collector"}, extra...)
	for _, component := range components {
		if err := runCodingSmokeCommand(environment, codingHelmReadyTimeout,
			"kubectl", "rollout", "status",
			"deployment/"+codingHelmRelease+"-coding-agent-"+component,
			"-n", codingHelmNamespace, "--timeout=90s"); err != nil {
			return err
		}
	}
	return runCodingSmokeCommand(environment, 30*time.Second,
		"kubectl", "exec", "-n", codingHelmNamespace,
		"deployment/"+codingHelmRelease+"-coding-agent-planner",
		"--", "sh", "-c",
		"nc -z -w 5 smoke-coding-agent-collector 4317")
}

type codingAgentImageEvidence struct {
	ImageID    string
	Containers []string
}

type codingAgentPodList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Spec struct {
			Containers []struct {
				Name  string   `json:"name"`
				Image string   `json:"image"`
				Args  []string `json:"args"`
			} `json:"containers"`
		} `json:"spec"`
		Status struct {
			ContainerStatuses []struct {
				Name    string `json:"name"`
				ImageID string `json:"imageID"`
			} `json:"containerStatuses"`
		} `json:"status"`
	} `json:"items"`
}

// verifyCodingAgentImageIdentity proves the live half of srd005 R9. The same
// classifier as chart conformance identifies agent workloads by --profile;
// donors and infrastructure containers are therefore excluded. Every selected
// main container must declare the canonical reference and report one identical
// runtime image ID, including the catalog mock and (for applierLive) applier.
func verifyCodingAgentImageIdentity(
	environment codingSmokeEnvironment,
	expectedReference string,
	includeApplier bool,
) (codingAgentImageEvidence, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := environment.run(ctx, "kubectl", "get", "pods",
		"-n", codingHelmNamespace, "-o", "json")
	if err != nil {
		return codingAgentImageEvidence{}, fmt.Errorf(
			"list pods for image identity: %w: %s", err, strings.TrimSpace(string(output)))
	}
	var pods codingAgentPodList
	if err := json.Unmarshal(output, &pods); err != nil {
		return codingAgentImageEvidence{}, fmt.Errorf("decode pods for image identity: %w", err)
	}
	expectedCount := 5 // planner, executor, critic, collector, canonical mock
	if includeApplier {
		expectedCount++
	}
	evidence, err := validateCodingAgentImageIdentity(pods, expectedReference, expectedCount)
	if err != nil {
		return codingAgentImageEvidence{}, err
	}
	fmt.Printf("one-agent-image: %d live agent containers use %s (%s)\n",
		len(evidence.Containers), expectedReference, evidence.ImageID)
	return evidence, nil
}

func validateCodingAgentImageIdentity(
	pods codingAgentPodList,
	expectedReference string,
	expectedCount int,
) (codingAgentImageEvidence, error) {
	var evidence codingAgentImageEvidence
	for _, pod := range pods.Items {
		statuses := make(map[string]string, len(pod.Status.ContainerStatuses))
		for _, status := range pod.Status.ContainerStatuses {
			statuses[status.Name] = status.ImageID
		}
		for _, container := range pod.Spec.Containers {
			if !codingArgsContain(container.Args, "--profile") {
				continue
			}
			identity := pod.Metadata.Name + "/" + container.Name
			if container.Image != expectedReference {
				return codingAgentImageEvidence{}, fmt.Errorf(
					"agent container %s image = %q, want %q",
					identity, container.Image, expectedReference)
			}
			imageID := statuses[container.Name]
			if imageID == "" {
				return codingAgentImageEvidence{}, fmt.Errorf(
					"agent container %s has no runtime image ID", identity)
			}
			if evidence.ImageID == "" {
				evidence.ImageID = imageID
			} else if imageID != evidence.ImageID {
				return codingAgentImageEvidence{}, fmt.Errorf(
					"agent container %s image ID = %q, want shared %q",
					identity, imageID, evidence.ImageID)
			}
			evidence.Containers = append(evidence.Containers, identity)
		}
	}
	if len(evidence.Containers) != expectedCount {
		return codingAgentImageEvidence{}, fmt.Errorf(
			"found %d live agent containers, want %d: %v",
			len(evidence.Containers), expectedCount, evidence.Containers)
	}
	return evidence, nil
}

func codingArgsContain(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func seedCodingWorkspace(environment codingSmokeEnvironment, applicationRoot string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	output, err := environment.run(ctx, "kubectl", "get", "pods",
		"-n", codingHelmNamespace,
		"-l", "app.kubernetes.io/component=executor",
		"-o", "jsonpath={.items[0].metadata.name}")
	cancel()
	if err != nil {
		return fmt.Errorf("find executor pod: %w: %s", err, strings.TrimSpace(string(output)))
	}
	pod := strings.TrimSpace(string(output))
	source := filepath.Join(applicationRoot, "testdata", "integration", "coding-loop", "workspace")
	for _, name := range []string{"go.mod", "greet.go", "greet_test.go"} {
		if err := runCodingSmokeCommand(environment, 30*time.Second,
			"kubectl", "cp", filepath.Join(source, name),
			codingHelmNamespace+"/"+pod+":/work/"+name); err != nil {
			return err
		}
	}
	return runCodingSmokeCommand(environment, 30*time.Second,
		"kubectl", "exec", "-n", codingHelmNamespace, pod, "--",
		"sh", "-c", "test -f /work/go.mod && test -f /work/greet.go && test -f /work/greet_test.go")
}

func runCodingSmokeCommand(
	environment codingSmokeEnvironment,
	timeout time.Duration,
	name string,
	args ...string,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	output, err := environment.run(ctx, name, args...)
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s",
			name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

type codingPortForwards struct {
	commands []*exec.Cmd
	// queryURL reaches the in-cluster collector query surface through an
	// ephemeral local port, so the smoke never collides with (or silently
	// queries) the observability rig's fixed 18193 (GH-1165).
	queryURL string
}

// startCodingHelmForwards opens the role and collector forwards the smoke drives.
// The live applier tier passes includeApplier so the applier's apply (18230) and
// control (18231) ports are forwarded too; the smoke install never enables the
// applier, so it passes false and no forward waits on a service that is absent.
func startCodingHelmForwards(
	environment codingSmokeEnvironment,
	includeApplier bool,
) (*codingPortForwards, error) {
	queryPort, err := freeLocalPort()
	if err != nil {
		return nil, fmt.Errorf("allocate collector query forward port: %w", err)
	}
	targets := []struct {
		service string
		ports   []string
	}{
		{codingHelmRelease + "-coding-agent-planner", []string{"18200:18200", "18201:18201"}},
		{codingHelmRelease + "-coding-agent-executor", []string{"18211:18211"}},
		{codingHelmRelease + "-coding-agent-critic", []string{"18221:18221"}},
		{codingHelmRelease + "-coding-agent-collector", []string{queryPort + ":18193"}},
	}
	if includeApplier {
		targets = append(targets, struct {
			service string
			ports   []string
		}{codingHelmRelease + "-coding-agent-applier", []string{"18230:18230", "18231:18231"}})
	}
	forwards := &codingPortForwards{queryURL: "http://127.0.0.1:" + queryPort}
	for _, target := range targets {
		args := []string{"port-forward", "-n", codingHelmNamespace, "service/" + target.service}
		args = append(args, target.ports...)
		command := exec.Command("kubectl", args...)
		command.Env = append(os.Environ(), "KUBECONFIG="+environment.kubeconfig)
		command.Stdout, command.Stderr = os.Stderr, os.Stderr
		if err := command.Start(); err != nil {
			forwards.stop()
			return nil, fmt.Errorf("start port-forward %s: %w", target.service, err)
		}
		forwards.commands = append(forwards.commands, command)
	}
	return forwards, nil
}

func (forwards *codingPortForwards) stop() {
	if forwards == nil {
		return
	}
	for _, command := range forwards.commands {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
	}
	forwards.commands = nil
}

func verifyCodingHealthEndpoints(queryURL string) error {
	for role, endpoint := range map[string]string{
		"planner":  "http://127.0.0.1:18201/api/lifecycle/health",
		"executor": "http://127.0.0.1:18211/api/lifecycle/health",
		"critic":   "http://127.0.0.1:18221/api/lifecycle/health",
	} {
		if err := waitServingHTTP(endpoint, codingHelmReadyTimeout); err != nil {
			return fmt.Errorf("%s lifecycle health: %w", role, err)
		}
	}
	return waitServingHTTP(queryURL+"/query/traces?page_size=1", codingHelmReadyTimeout)
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

func submitCodingHelmRequest() error {
	payload := `{"workspace_id":"kind-shared","task":"Implement the Hello function described by doc/specs/software-requirements/srd001-greet.yaml. Run the tests and finish only when they pass."}`
	var status int
	var body string
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		status, body, err = servingJSONRequestWithTimeout(
			plannerRequestURL, payload, codingHelmTraceparent, codingHelmRequestTimeout)
		if err == nil && status == http.StatusOK &&
			strings.Contains(body, `"verdict":"accepted"`) &&
			strings.Contains(body, codingHelmTraceID) {
			return nil
		}
		if err == nil && status == http.StatusInternalServerError &&
			strings.Contains(body, `"terminal_signal":"CommandError"`) &&
			attempt < 3 {
			time.Sleep(time.Second)
			continue
		}
		break
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("planner response status=%d body=%s", status, body)
}

func verifyCodingWorkspaceAndVerdict(environment codingSmokeEnvironment) error {
	checks := [][]string{
		{"exec", "-n", codingHelmNamespace,
			"deployment/" + codingHelmRelease + "-coding-agent-executor",
			"--", "grep", "-F", `return "Hello, " + name + "!"`, "/work/greet.go"},
		{"exec", "-n", codingHelmNamespace,
			"deployment/" + codingHelmRelease + "-coding-agent-executor",
			"--", "sh", "-c", "cd /work && go test ./..."},
		{"exec", "-n", codingHelmNamespace,
			"deployment/" + codingHelmRelease + "-coding-agent-critic",
			"--", "grep", "-F", `"verdict":"accepted"`, "/work/critic-verdict.json"},
	}
	for _, args := range checks {
		if err := runCodingSmokeCommand(
			environment, 60*time.Second, "kubectl", args...); err != nil {
			return err
		}
	}
	return nil
}

func verifyCodingTrace(queryURL string) error {
	endpoint := queryURL + "/query/traces/" + codingHelmTraceID
	deadline := time.Now().Add(codingHelmTraceTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		trace, err := codingCollectorGetTrace(endpoint)
		if err != nil {
			lastErr = err
			time.Sleep(time.Second)
			continue
		}
		services := map[string]bool{}
		for _, span := range trace.Spans {
			if span.ServiceName != "" {
				services[span.ServiceName] = true
			}
		}
		var missing []string
		for _, service := range []string{"coding-planner", "coding-executor", "coding-critic"} {
			if !services[service] {
				missing = append(missing, service)
			}
		}
		if len(missing) == 0 {
			return nil
		}
		lastErr = fmt.Errorf("trace %s has %d spans, missing services %v (found %v)",
			codingHelmTraceID, trace.SpanCount, missing, services)
		time.Sleep(time.Second)
	}
	return fmt.Errorf("connected trace %s not retained: %w", codingHelmTraceID, lastErr)
}

type codingCollectorTrace struct {
	TraceID   string                `json:"trace_id"`
	Spans     []codingCollectorSpan `json:"spans"`
	SpanCount int                   `json:"span_count"`
}

type codingCollectorSpan struct {
	ServiceName string `json:"service"`
}

func codingCollectorGetTrace(endpoint string) (*codingCollectorTrace, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("collector GET %s status %d", endpoint, response.StatusCode)
	}
	var trace codingCollectorTrace
	if err := json.NewDecoder(response.Body).Decode(&trace); err != nil {
		return nil, fmt.Errorf("collector trace decode: %w", err)
	}
	return &trace, nil
}

func codingHelmEvidenceDir(applicationRoot, revision string) string {
	run := time.Now().UTC().Format("20060102T150405.000000000Z")
	return filepath.Join(applicationRoot, "build", "kind-evidence",
		codingHelmNamespace+"-"+revision+"-"+run)
}
