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
	"runtime"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

const codingHelmAgentDockerfile = `FROM golang:1.26-alpine
COPY agent /usr/local/bin/agent
COPY golangci-lint /usr/local/bin/golangci-lint
COPY tools /opt/agent-core/tools
RUN addgroup -S -g 10001 agent && adduser -S -u 10001 -G agent agent && mkdir -p /work && chown 10001:10001 /work
ENV AGENT_CORE_HOME=/opt/agent-core HOME=/tmp PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
USER 10001:10001
ENTRYPOINT ["agent"]
`

func prepareCodingHelmCluster(
	environment codingSmokeEnvironment,
	cluster string,
	roots integrationRoots,
) error {
	// Clear only smoke-owned objects when reusing a developer cluster.
	for _, command := range [][]string{
		{"delete", "namespace", codingHelmNamespace, "--ignore-not-found=true", "--wait=true", "--timeout=30s"},
		{"delete", "pv", "coding-agent-kind-workspace", "--ignore-not-found=true", "--wait=true", "--timeout=30s"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		_, _ = environment.run(ctx, "kubectl", command...)
		cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), codingHelmProbeTimeout)
	output, err := codingSmokeEnvironment{}.run(ctx, "docker", "exec",
		cluster+"-control-plane", "sh", "-c",
		"rm -rf /tmp/coding-agent-workspace && mkdir -p /tmp/coding-agent-workspace && chmod 0777 /tmp/coding-agent-workspace")
	cancel()
	if err != nil {
		return fmt.Errorf("prepare kind workspace: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := buildCodingHelmAgentImage(roots.Core, codingHelmAgentImage); err != nil {
		return err
	}
	if err := buildCodingHelmModelImage(codingHelmModelImage); err != nil {
		return err
	}
	for _, image := range []string{codingHelmAgentImage, codingHelmModelImage} {
		ctx, cancel := context.WithTimeout(context.Background(), codingHelmClusterTimeout)
		output, err := codingSmokeEnvironment{}.run(
			ctx, "kind", "load", "docker-image", image, "--name", cluster)
		cancel()
		if err != nil {
			return fmt.Errorf("load image %s: %w: %s", image, err, strings.TrimSpace(string(output)))
		}
	}
	for _, image := range []string{codingHelmCollectorImage, codingHelmJaegerImage} {
		if err := loadCodingDependencyImage(cluster, image); err != nil {
			return &codingHelmInfrastructureError{
				Step: "dependency image load", Cause: err,
			}
		}
	}
	if err := runCodingSmokeCommand(environment, 30*time.Second,
		"kubectl", "create", "namespace", codingHelmNamespace); err != nil {
		return err
	}
	if err := runCodingSmokeCommand(environment, 30*time.Second,
		"kubectl", "apply", "-f",
		filepath.Join(roots.Application, "helm", "ci", "kind-workspace.yaml")); err != nil {
		return err
	}
	modelManifest, cleanup, err := codingModelManifest()
	if err != nil {
		return err
	}
	defer cleanup()
	if err := runCodingSmokeCommand(environment, 30*time.Second,
		"kubectl", "apply", "-f", modelManifest); err != nil {
		return err
	}
	return runCodingSmokeCommand(environment, codingHelmReadyTimeout,
		"kubectl", "rollout", "status", "deployment/coding-model",
		"-n", codingHelmNamespace, "--timeout=90s")
}

func loadCodingDependencyImage(cluster, image string) error {
	ctx, cancel := context.WithTimeout(context.Background(), codingHelmClusterTimeout)
	defer cancel()
	save := exec.CommandContext(ctx, "docker", "save", image)
	stream, err := save.StdoutPipe()
	if err != nil {
		return err
	}
	node := cluster + "-control-plane"
	load := exec.CommandContext(ctx, "docker", "exec", "-i", node,
		"ctr", "--namespace=k8s.io", "images", "import",
		"--platform=linux/"+runtime.GOARCH, "--snapshotter=overlayfs", "-")
	load.Stdin = stream
	var output bytes.Buffer
	load.Stdout, load.Stderr = &output, &output
	if err := load.Start(); err != nil {
		return err
	}
	if err := save.Run(); err != nil {
		_ = load.Process.Kill()
		_ = load.Wait()
		return fmt.Errorf("docker save %s: %w", image, err)
	}
	if err := load.Wait(); err != nil {
		return fmt.Errorf("import %s: %w: %s", image, err, strings.TrimSpace(output.String()))
	}
	return nil
}

func buildCodingHelmAgentImage(coreRoot, image string) error {
	contextDir, err := os.MkdirTemp("", "coding-agent-kind-image-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(contextDir)
	build := exec.Command("go", "build", "-tags", "production", "-trimpath",
		"-ldflags=-s -w", "-o", filepath.Join(contextDir, "agent"), "./cmd/agent")
	build.Dir = coreRoot
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux")
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build Linux agent: %w: %s", err, strings.TrimSpace(string(output)))
	}
	lintContext, lintCancel := context.WithTimeout(context.Background(), codingHelmClusterTimeout)
	lintGoPath, err := os.MkdirTemp("", "coding-agent-lint-gopath-*")
	if err != nil {
		return err
	}
	defer removeCodingLintGoPath(lintGoPath)
	lint := exec.CommandContext(lintContext, "go", "install",
		"github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8")
	lint.Env = append(os.Environ(),
		"CGO_ENABLED=0", "GOOS=linux", "GOPATH="+lintGoPath, "GOBIN=")
	lintOutput, lintErr := lint.CombinedOutput()
	lintCancel()
	if lintErr != nil {
		return fmt.Errorf("build Linux golangci-lint: %w: %s",
			lintErr, strings.TrimSpace(string(lintOutput)))
	}
	lintBinary := filepath.Join(
		lintGoPath, "bin", "linux_"+runtime.GOARCH, "golangci-lint")
	if err := os.Rename(lintBinary, filepath.Join(contextDir, "golangci-lint")); err != nil {
		return fmt.Errorf("stage Linux golangci-lint: %w", err)
	}
	if err := copyTree(filepath.Join(coreRoot, "tools"), filepath.Join(contextDir, "tools")); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte(codingHelmAgentDockerfile), 0o644); err != nil {
		return err
	}
	return runLocalDockerBuild(contextDir, image)
}

func removeCodingLintGoPath(root string) {
	_ = filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
		if err == nil {
			_ = os.Chmod(path, 0o700)
		}
		return nil
	})
	_ = os.RemoveAll(root)
}

func buildCodingHelmModelImage(image string) error {
	contextDir, err := os.MkdirTemp("", "coding-model-kind-image-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(contextDir)
	source := `package main
import ("encoding/json"; "net/http"; "strings")
func main() {
  http.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
    json.NewEncoder(w).Encode(map[string]any{"models":[]map[string]string{{"name":"qwen3.6:35b-mlx"}}})
  })
  http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
  http.HandleFunc("/api/chat", chat)
  http.ListenAndServe(":11434", nil)
}
func chat(w http.ResponseWriter, r *http.Request) {
  var request struct { Messages []struct { Content string ` + "`json:\"content\"`" + ` } ` + "`json:\"messages\"`" + ` }
  if json.NewDecoder(r.Body).Decode(&request) != nil { http.Error(w, "bad request", 400); return }
  planner := false
  edited := false
  for _, message := range request.Messages {
    if strings.Contains(message.Content, "implementation planner for a Go software project") || strings.Contains(message.Content, "software planning assistant") { planner = true }
    if strings.Contains(message.Content, "\"replacements\":1") || strings.Contains(message.Content, "\"tool\":\"edit\"") { edited = true }
  }
  content := "title: Implement greeting\nfiles:\n  - path: greet.go\n    action: modify\nrequirements:\n  - id: R1\n    text: Return required greeting\nacceptance_criteria:\n  - id: AC1\n    text: go test passes\n"
  if !planner {
    if !edited {
      content = "[tool_call]{\"tool\":\"edit\",\"parameters\":{\"path\":\"greet.go\",\"old_string\":\"func Hello(name string) string {\\n\\treturn \\\"\\\"\\n}\",\"new_string\":\"func Hello(name string) string {\\n\\treturn \\\"Hello, \\\" + name + \\\"!\\\"\\n}\"}}[/tool_call]"
    } else {
      content = "[tool_call]{\"tool\":\"done\",\"parameters\":{\"summary\":\"implemented greeting\"}}[/tool_call]"
    }
  }
  json.NewEncoder(w).Encode(map[string]any{"message":map[string]string{"role":"assistant","content":content},"eval_count":1,"prompt_eval_count":1})
}
`
	if err := os.WriteFile(filepath.Join(contextDir, "main.go"), []byte(source), 0o644); err != nil {
		return err
	}
	build := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w",
		"-o", filepath.Join(contextDir, "model"), filepath.Join(contextDir, "main.go"))
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux")
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build deterministic model: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"),
		[]byte("FROM scratch\nCOPY model /model\nUSER 65532:65532\nENTRYPOINT [\"/model\"]\n"), 0o644); err != nil {
		return err
	}
	return runLocalDockerBuild(contextDir, image)
}

func runLocalDockerBuild(contextDir, image string) error {
	ctx, cancel := context.WithTimeout(context.Background(), codingHelmClusterTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "docker", "build", "--pull=false", "-t", image, ".")
	command.Dir = contextDir
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("docker build %s: %w: %s", image, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func codingModelManifest() (string, func(), error) {
	dir, err := os.MkdirTemp("", "coding-model-manifest-*")
	if err != nil {
		return "", nil, err
	}
	path := filepath.Join(dir, "model.yaml")
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata: {name: coding-model, namespace: coding-agent-smoke}
spec:
  replicas: 1
  selector: {matchLabels: {app: coding-model}}
  template:
    metadata: {labels: {app: coding-model}}
    spec:
      automountServiceAccountToken: false
      securityContext: {runAsNonRoot: true, seccompProfile: {type: RuntimeDefault}}
      containers:
        - name: model
          image: declarative-agents/coding-model-smoke:local
          imagePullPolicy: Never
          securityContext: {allowPrivilegeEscalation: false, readOnlyRootFilesystem: true, capabilities: {drop: [ALL]}}
          ports: [{name: http, containerPort: 11434}]
          readinessProbe: {httpGet: {path: /healthz, port: http}, initialDelaySeconds: 1, periodSeconds: 2}
---
apiVersion: v1
kind: Service
metadata: {name: coding-model, namespace: coding-agent-smoke}
spec:
  selector: {app: coding-model}
  ports: [{name: http, port: 11434, targetPort: http}]
`
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, err
	}
	return path, func() { _ = os.RemoveAll(dir) }, nil
}

func installCodingHelmChart(
	environment codingSmokeEnvironment,
	archive, applicationRoot string,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), codingHelmInstallTimeout)
	defer cancel()
	output, err := environment.run(ctx, "helm",
		"install", codingHelmRelease, archive,
		"--namespace", codingHelmNamespace,
		"--values", filepath.Join(applicationRoot, "helm", "ci", "kind-values.yaml"),
		"--wait", "--timeout", codingHelmInstallTimeout.String(),
	)
	if err != nil {
		return fmt.Errorf("helm install: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func verifyCodingHelmRollouts(environment codingSmokeEnvironment) error {
	for _, component := range []string{"planner", "executor", "critic", "collector", "jaeger"} {
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
		"nc -z -w 5 smoke-coding-agent-collector 4317 && nc -z -w 5 smoke-coding-agent-jaeger 4317")
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
}

func startCodingHelmForwards(
	environment codingSmokeEnvironment,
) (*codingPortForwards, error) {
	targets := []struct {
		service string
		ports   []string
	}{
		{codingHelmRelease + "-coding-agent-planner", []string{"18200:18200", "18201:18201"}},
		{codingHelmRelease + "-coding-agent-executor", []string{"18211:18211"}},
		{codingHelmRelease + "-coding-agent-critic", []string{"18221:18221"}},
		{codingHelmRelease + "-coding-agent-jaeger", []string{"18686:16686"}},
	}
	forwards := &codingPortForwards{}
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

func verifyCodingHealthEndpoints() error {
	for role, endpoint := range map[string]string{
		"planner":  "http://127.0.0.1:18201/api/lifecycle/health",
		"executor": "http://127.0.0.1:18211/api/lifecycle/health",
		"critic":   "http://127.0.0.1:18221/api/lifecycle/health",
	} {
		if err := waitServingHTTP(endpoint, codingHelmReadyTimeout); err != nil {
			return fmt.Errorf("%s lifecycle health: %w", role, err)
		}
	}
	return waitServingHTTP(codingHelmJaegerURL+"/", codingHelmReadyTimeout)
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

func verifyCodingTrace() error {
	deadline := time.Now().Add(codingHelmTraceTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		services, err := codingJaegerServices()
		if err != nil {
			lastErr = err
			time.Sleep(time.Second)
			continue
		}
		var observed []string
		for _, service := range services {
			traces, queryErr := codingJaegerTraces(service)
			if queryErr != nil {
				lastErr = queryErr
				continue
			}
			for _, trace := range traces {
				observed = append(observed, service+":"+trace.TraceID)
				if trace.TraceID != codingHelmTraceID {
					continue
				}
				services := map[string]bool{}
				for _, process := range trace.Processes {
					services[process.ServiceName] = true
				}
				missing := []string{}
				for _, service := range []string{"coding-planner", "coding-executor", "coding-critic"} {
					if !services[service] {
						missing = append(missing, service)
					}
				}
				if len(missing) == 0 {
					return nil
				}
				lastErr = fmt.Errorf("trace %s missing services %v (found %v)",
					codingHelmTraceID, missing, services)
			}
		}
		if len(services) == 0 {
			lastErr = fmt.Errorf("Jaeger reports no services")
		} else if len(observed) == 0 {
			lastErr = fmt.Errorf("Jaeger services %v report no traces", services)
		} else if lastErr == nil {
			lastErr = fmt.Errorf("Jaeger observed traces %v, want %s", observed, codingHelmTraceID)
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("connected trace %s not retained: %w", codingHelmTraceID, lastErr)
}

type codingJaegerTrace struct {
	TraceID   string `json:"traceID"`
	Processes map[string]struct {
		ServiceName string `json:"serviceName"`
	} `json:"processes"`
}

func codingJaegerServices() ([]string, error) {
	var response struct {
		Data []string `json:"data"`
	}
	if err := codingJaegerGet(
		codingHelmJaegerURL+"/api/services", &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

func codingJaegerTraces(service string) ([]codingJaegerTrace, error) {
	var response struct {
		Data []codingJaegerTrace `json:"data"`
	}
	endpoint := codingHelmJaegerURL + "/api/traces?service=" +
		url.QueryEscape(service) + "&limit=20&start=0&end=" +
		fmt.Sprint(time.Now().Add(time.Hour).UnixMicro())
	if err := codingJaegerGet(endpoint, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

func codingJaegerGet(endpoint string, target any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Jaeger GET %s status %d", endpoint, response.StatusCode)
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func cleanupCodingHelmSmoke(
	environment codingSmokeEnvironment,
	cluster kindrig.Cluster,
	kindRun kindrig.Runner,
) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	_, _ = environment.run(ctx, "helm", "uninstall", codingHelmRelease,
		"-n", codingHelmNamespace, "--wait", "--timeout=20s")
	cancel()
	ctx, cancel = context.WithTimeout(context.Background(), 40*time.Second)
	_, _ = environment.run(ctx, "kubectl", "delete", "namespace", codingHelmNamespace,
		"--ignore-not-found=true", "--wait=true", "--timeout=30s")
	cancel()
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	_, _ = environment.run(ctx, "kubectl", "delete", "pv",
		"coding-agent-kind-workspace", "--ignore-not-found=true", "--wait=false")
	cancel()
	cluster.Release(kindRun)
}
