// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	plannerRequestURL     = "http://127.0.0.1:18200/api/v1/coding"
	plannerControlURL     = "http://127.0.0.1:18201"
	executorControlURL    = "http://127.0.0.1:18211"
	criticRequestURL      = "http://127.0.0.1:18220/api/v1/review"
	criticControlURL      = "http://127.0.0.1:18221"
	servingReadyTimeout   = 30 * time.Second
	servingRequestTimeout = 20 * time.Minute
)

var servingRoles = []string{"planner", "executor", "critic"}

type servingAgent struct {
	role   string
	cmd    *exec.Cmd
	output *bytes.Buffer
	done   chan error
}

// ServingHealth starts the three real persistent profiles, proves every
// lifecycle endpoint, and drives the deterministic canonical critic request.
func (Integration) ServingHealth() error {
	roots, err := resolveIntegrationRoots()
	if err != nil {
		return err
	}
	if reason := baseIntegrationSkipReason(roots, "sh"); reason != "" {
		fmt.Printf("SKIP servingHealth: %s\n", reason)
		return nil
	}
	binary, cleanupBinary, err := buildAgent(roots.Core)
	if err != nil {
		return err
	}
	defer cleanupBinary()
	profiles, cleanupProfiles, err := stageServingProfileTree(roots)
	if err != nil {
		return err
	}
	defer cleanupProfiles()
	workspace, cleanupWorkspace, err := freshCandidateFixture(roots.Application, "accepted")
	if err != nil {
		return err
	}
	defer cleanupWorkspace()
	traceDir, err := os.MkdirTemp("", "coding-agent-serving-traces-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(traceDir) }()
	agents, err := startServingAgents(binary, roots.Core, profiles, workspace, traceDir, "")
	if err != nil {
		return err
	}
	defer func() {
		if agents != nil {
			_ = stopServingAgents(agents)
		}
	}()
	if err := waitServingHealth(); err != nil {
		return err
	}
	status, body, err := servingJSONRequest(criticRequestURL,
		`{"workspace_id":"shared"}`, "")
	if err != nil {
		return err
	}
	if status != http.StatusOK || !strings.Contains(body, `"verdict":"accepted"`) {
		return fmt.Errorf("critic serving response status=%d body=%s", status, body)
	}
	fmt.Println("integration:servingHealth PASS - planner, executor, and critic served real lifecycle health; canonical critic accepted the shared workspace")
	return nil
}

// ServingRemote proves one connected planner -> executor -> critic request with
// the real binary and canonical planner/executor/critic behavior. A deterministic
// Ollama-compatible boundary supplies only model responses; all profile, REST,
// filesystem, validation, critic, lifecycle, and trace behavior is production.
func (Integration) ServingRemote() error {
	roots, err := resolveIntegrationRoots()
	if err != nil {
		return err
	}
	if reason := baseIntegrationSkipReason(roots); reason != "" {
		fmt.Printf("SKIP servingRemote: %s\n", reason)
		return nil
	}
	binary, cleanupBinary, err := buildAgent(roots.Core)
	if err != nil {
		return err
	}
	defer cleanupBinary()
	profiles, cleanupProfiles, err := stageServingProfileTree(roots)
	if err != nil {
		return err
	}
	defer cleanupProfiles()
	workspace, cleanupWorkspace, err := freshWorkspace(roots.Application)
	if err != nil {
		return err
	}
	defer cleanupWorkspace()
	traceDir, err := os.MkdirTemp("", "coding-agent-serving-traces-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(traceDir) }()
	model := newServingModel()
	defer model.Close()
	agents, err := startServingAgents(binary, roots.Core, profiles, workspace, traceDir, model.URL)
	if err != nil {
		return err
	}
	defer func() {
		if agents != nil {
			_ = stopServingAgents(agents)
		}
	}()
	if err := waitServingHealth(); err != nil {
		return err
	}
	const traceID = "0af7651916cd43dd8448eb211c80319c"
	const traceparent = "00-" + traceID + "-b7ad6b7169203331-01"
	payload := `{"workspace_id":"shared","task":"Implement the Hello function described by doc/specs/software-requirements/srd001-greet.yaml. Run the tests and finish only when they pass."}`
	status, body, err := servingJSONRequest(plannerRequestURL, payload, traceparent)
	if err != nil {
		return err
	}
	if status != http.StatusOK || !strings.Contains(body, `"verdict":"accepted"`) {
		return fmt.Errorf("remote coding response status=%d body=%s%s",
			status, body, servingAgentDiagnostics(agents))
	}
	if err := requireGreetingAndTests(workspace); err != nil {
		return err
	}
	if err := stopServingAgents(agents); err != nil {
		return err
	}
	agents = nil
	for _, role := range servingRoles {
		data, readErr := os.ReadFile(filepath.Join(traceDir, role+".ndjson"))
		if readErr != nil {
			return fmt.Errorf("read %s trace: %w", role, readErr)
		}
		if !strings.Contains(string(data), traceID) {
			return fmt.Errorf("%s trace does not join request trace %s", role, traceID)
		}
	}
	fmt.Printf("integration:servingRemote PASS - planner -> executor -> critic accepted the shared workspace on connected trace %s\n", traceID)
	return nil
}

func servingAgentDiagnostics(agents []*servingAgent) string {
	var diagnostics strings.Builder
	for _, agent := range agents {
		if agent == nil || agent.output == nil {
			continue
		}
		if output := strings.TrimSpace(agent.output.String()); output != "" {
			fmt.Fprintf(&diagnostics, "\n%s output:\n%s", agent.role, output)
		}
	}
	return diagnostics.String()
}

func stageServingProfileTree(roots integrationRoots) (string, func(), error) {
	stage, err := os.MkdirTemp("", "coding-agent-serving-profiles-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(stage) }
	root := filepath.Join(stage, "profiles")
	manifest, err := readApplicationProfileManifest(filepath.Join(roots.Application, filepath.FromSlash(profileManifestPath)))
	if err != nil {
		cleanup()
		return "", nil, err
	}
	source, err := inspectPackageSource(roots.Profiles, manifest.Catalog.CompatibleRelease)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	if _, err := assembleProfileClosure(manifest, roots.Profiles, root, source); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("assemble #847 profile closure: %w", err)
	}
	if err := copyTree(
		filepath.Join(roots.Profiles, "agents", "applier"),
		filepath.Join(root, "applications", "catalog", "applier"),
	); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("stage canonical applier runtime projection: %w", err)
	}
	destination := filepath.Join(root, "applications", "coding-agent")
	if err := copyTree(filepath.Join(roots.Application, "agents"), destination); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("stage application actor profiles: %w", err)
	}
	return root, cleanup, nil
}

func startServingAgents(binary, coreRoot, profilesRoot, workspace, traceDir, ollamaURL string) ([]*servingAgent, error) {
	agents := make([]*servingAgent, 0, len(servingRoles))
	for _, role := range servingRoles {
		var output bytes.Buffer
		profile := filepath.Join(profilesRoot, "applications", "coding-agent", role, "profile.yaml")
		cmd := exec.Command(binary,
			"--profile", profile,
			"--directory", workspace,
			"--core-root", coreRoot,
			"--otel-log-file", filepath.Join(traceDir, role+".ndjson"),
			"--otel-service-name", role,
		)
		cmd.Dir = workspace
		cmd.Env = os.Environ()
		if ollamaURL != "" {
			cmd.Env = append(cmd.Env, "OLLAMA_URL="+ollamaURL)
		}
		cmd.Stdout, cmd.Stderr = &output, &output
		if err := cmd.Start(); err != nil {
			_ = stopServingAgents(agents)
			return nil, fmt.Errorf("start %s server: %w", role, err)
		}
		agent := &servingAgent{role: role, cmd: cmd, output: &output, done: make(chan error, 1)}
		go func() { agent.done <- cmd.Wait() }()
		agents = append(agents, agent)
	}
	return agents, nil
}

type servingModel struct {
	*httptest.Server
	mu            sync.Mutex
	executorCalls int
}

func newServingModel() *servingModel {
	model := &servingModel{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
		writeServingJSON(w, map[string]any{"models": []map[string]string{{"name": canonicalModel}}})
	})
	mux.HandleFunc("/api/chat", model.chat)
	model.Server = httptest.NewServer(mux)
	return model
}

func (model *servingModel) chat(w http.ResponseWriter, request *http.Request) {
	var body struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	planner := false
	for _, message := range body.Messages {
		if strings.Contains(message.Content, "implementation planner for a Go software project") ||
			strings.Contains(message.Content, "software planning assistant") {
			planner = true
			break
		}
	}
	content := `title: Implement greeting
files:
  - path: greet.go
    action: modify
requirements:
  - id: R1
    text: Return the required greeting
acceptance_criteria:
  - id: AC1
    text: go test ./... passes
`
	if !planner {
		model.mu.Lock()
		model.executorCalls++
		call := model.executorCalls
		model.mu.Unlock()
		if call == 1 {
			content = `[tool_call]{"tool":"edit","parameters":{"path":"greet.go","old_string":"func Hello(name string) string {\n\treturn \"\"\n}","new_string":"func Hello(name string) string {\n\treturn \"Hello, \" + name + \"!\"\n}"}}[/tool_call]`
		} else {
			content = `[tool_call]{"tool":"done","parameters":{"summary":"implemented greeting and ready for validation"}}[/tool_call]`
		}
	}
	writeServingJSON(w, map[string]any{
		"message":           map[string]string{"role": "assistant", "content": content},
		"eval_count":        1,
		"prompt_eval_count": 1,
	})
}

func writeServingJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func stopServingAgents(agents []*servingAgent) error {
	var failures []string
	for _, agent := range agents {
		if agent == nil || agent.cmd == nil || agent.cmd.Process == nil {
			continue
		}
		control := map[string]string{
			"planner":  plannerControlURL,
			"executor": executorControlURL,
			"critic":   criticControlURL,
		}[agent.role]
		_, _, _ = servingJSONRequestWithTimeout(
			control+"/api/lifecycle/exit", `{"reason":"integration cleanup"}`, "", 2*time.Second)
		select {
		case err := <-agent.done:
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v\n%s", agent.role, err, agent.output.String()))
			}
		case <-time.After(10 * time.Second):
			_ = agent.cmd.Process.Kill()
			<-agent.done
			failures = append(failures, agent.role+": graceful exit timed out\n"+agent.output.String())
		}
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "\n"))
	}
	return nil
}

func waitServingHealth() error {
	for role, endpoint := range map[string]string{
		"planner":  plannerControlURL,
		"executor": executorControlURL,
		"critic":   criticControlURL,
	} {
		if err := waitServingHTTP(endpoint+"/api/lifecycle/health", servingReadyTimeout); err != nil {
			return fmt.Errorf("%s health: %w", role, err)
		}
	}
	return nil
}

func waitServingHTTP(endpoint string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		resp, err := (&http.Client{Timeout: time.Second}).Get(endpoint)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			last = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			last = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("wait for %s: %w", endpoint, last)
}

func servingJSONRequest(endpoint, payload, traceparent string) (int, string, error) {
	return servingJSONRequestWithTimeout(endpoint, payload, traceparent, servingRequestTimeout)
}

func servingJSONRequestWithTimeout(endpoint, payload, traceparent string, timeout time.Duration) (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(payload))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if traceparent != "" {
		req.Header.Set("traceparent", traceparent)
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", err
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return resp.StatusCode, string(data), fmt.Errorf("decode response: %w", err)
	}
	return resp.StatusCode, strings.TrimSpace(string(data)), nil
}
