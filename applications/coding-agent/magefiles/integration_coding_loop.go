// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/magefile/mage/mg"
)

// Integration owns the coding application's optional live-model proofs.
type Integration mg.Namespace

// ExecutorLive runs the canonical executor to a successful terminal over a
// fresh copy of the greet workspace.
func (Integration) ExecutorLive() error {
	roots, err := resolveIntegrationRoots()
	if err != nil {
		return err
	}
	if err := validateReleaseDeadlines(); err != nil {
		return err
	}
	if reason := liveSkipReason(roots); reason != "" {
		fmt.Printf("SKIP executorLive: %s\n", reason)
		return nil
	}
	roots, cleanupProfiles, err := packageIntegrationRoots(roots)
	if err != nil {
		return err
	}
	defer cleanupProfiles()
	binary, cleanupBinary, err := buildAgent(roots.Core)
	if err != nil {
		return err
	}
	defer cleanupBinary()
	workspace, cleanupWorkspace, err := freshWorkspace(roots.Application)
	if err != nil {
		return err
	}
	defer cleanupWorkspace()
	model := newReleaseCodingModel()
	defer model.Close()
	run, err := runBuiltAgentWithOptions(
		binary, roots.Profiles, roots.Core, "agents/executor/profile.yaml", workspace,
		releaseAgentRunOptions(model.URL),
	)
	if err != nil {
		return err
	}
	if err := requireSuccessfulExecutor(workspace, run); err != nil {
		return err
	}
	fmt.Println("integration:executorLive PASS - canonical executor used the deterministic Ollama-compatible boundary, changed the greet workspace, and passed go test ./...")
	return nil
}

// PlannerDelegation runs the full canonical planner, including its selected
// local bd tracker and execute_task boundary. The tracker database lives only
// in the disposable workspace; the child process is the same real binary as
// the planner process.
func (Integration) PlannerDelegation() error {
	roots, err := resolveIntegrationRoots()
	if err != nil {
		return err
	}
	if err := validateReleaseDeadlines(); err != nil {
		return err
	}
	if reason := liveSkipReason(roots, "bd"); reason != "" {
		fmt.Printf("SKIP plannerDelegation: %s\n", reason)
		return nil
	}
	roots, cleanupProfiles, err := packageIntegrationRoots(roots)
	if err != nil {
		return err
	}
	defer cleanupProfiles()
	binary, cleanupBinary, err := buildAgent(roots.Core)
	if err != nil {
		return err
	}
	defer cleanupBinary()
	model := newReleaseCodingModel()
	defer model.Close()
	_, cleanupWorkspace, err := producePlannerCandidate(roots, binary, model.URL)
	if err != nil {
		return err
	}
	cleanupWorkspace()
	plannerCalls, _ := model.invocationCounts()
	if plannerCalls != 1 {
		return fmt.Errorf("planner model invocations = %d, want exactly 1", plannerCalls)
	}
	fmt.Println("integration:plannerDelegation PASS - canonical planner used one deterministic model response, materialized a local task, and delegated to the real canonical executor")
	return nil
}

func producePlannerCandidate(roots integrationRoots, binary, modelURL string) (string, func(), error) {
	workspace, cleanupWorkspace, err := freshWorkspace(roots.Application)
	if err != nil {
		return "", nil, err
	}
	if err := initializePlannerWorkspace(workspace); err != nil {
		cleanupWorkspace()
		return "", nil, err
	}
	run, err := runBuiltAgentWithOptions(
		binary, roots.Profiles, roots.Core, "agents/planner/profile.yaml", workspace,
		releaseAgentRunOptions(modelURL),
		"--child-agent-binary", binary, "--verbose-trace",
	)
	if err != nil {
		cleanupWorkspace()
		return "", nil, err
	}
	if run.ExitCode != 0 {
		cleanupWorkspace()
		return "", nil, fmt.Errorf("planner exited %d:\n%s\ntrace diagnostics:\n%s", run.ExitCode, run.Output, run.Trace)
	}
	if run.FinalState != "Completed" {
		cleanupWorkspace()
		return "", nil, fmt.Errorf("planner final state = %q, want Completed:\n%s", run.FinalState, run.Output)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".git", "agent-planner", "issue-body.yaml")); err != nil {
		cleanupWorkspace()
		return "", nil, fmt.Errorf("planner did not materialize its task: %w", err)
	}
	if err := requireGreetingAndTests(workspace); err != nil {
		cleanupWorkspace()
		return "", nil, fmt.Errorf("delegated executor result: %w", err)
	}
	return workspace, cleanupWorkspace, nil
}

func initializePlannerWorkspace(workspace string) error {
	commands := [][]string{
		{"git", "init", "-q"},
		{"bd", "init", "--quiet", "--non-interactive", "--skip-agents", "--skip-hooks", "--sandbox", "--prefix", "coding-loop"},
	}
	for _, argv := range commands {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = workspace
		cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w\n%s", strings.Join(argv, " "), err, output)
		}
	}
	return nil
}

func requireGreetingAndTests(workspace string) error {
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = workspace
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("workspace does not satisfy the greeting contract: go test ./...: %w\n%s", err, output)
	}
	return nil
}

// CriticGate independently gives the canonical changed-workspace critic two
// deterministic fixtures. It never invokes the planner; CodingLoop owns the
// exact Stage B workspace handoff used by the composite Stage C proof.
func (Integration) CriticGate() error {
	roots, err := resolveIntegrationRoots()
	if err != nil {
		return err
	}
	if reason := baseIntegrationSkipReason(roots, "sh"); reason != "" {
		fmt.Printf("SKIP criticGate: %s\n", reason)
		return nil
	}
	roots, cleanupProfiles, err := packageIntegrationRoots(roots)
	if err != nil {
		return err
	}
	defer cleanupProfiles()
	binary, cleanupBinary, err := buildAgent(roots.Core)
	if err != nil {
		return err
	}
	defer cleanupBinary()

	accepted, cleanupAccepted, err := freshCandidateFixture(roots.Application, "accepted")
	if err != nil {
		return err
	}
	defer cleanupAccepted()
	if err := runCriticGate(binary, roots, accepted, "deterministic conforming fixture"); err != nil {
		return err
	}
	return nil
}

func runCriticGate(binary string, roots integrationRoots, accepted, acceptedSource string) error {
	rejected, cleanupRejected, err := freshCandidateFixture(roots.Application, "rejected")
	if err != nil {
		return err
	}
	defer cleanupRejected()

	acceptedVerdict, err := runCanonicalCriticVerdict(binary, roots, accepted)
	if err != nil {
		return fmt.Errorf("accepted candidate from %s: %w", acceptedSource, err)
	}
	rejectedVerdict, err := runCanonicalCriticVerdict(binary, roots, rejected)
	if err != nil {
		return fmt.Errorf("rejected candidate fixture: %w", err)
	}
	acceptedOutcome, err := applicationOutcome(acceptedVerdict)
	if err != nil {
		return err
	}
	rejectedOutcome, err := applicationOutcome(rejectedVerdict)
	if err != nil {
		return err
	}
	if acceptedOutcome != "Succeeded" || rejectedOutcome != "Failed" {
		return fmt.Errorf("application outcomes accepted=%s rejected=%s, want Succeeded/Failed",
			acceptedOutcome, rejectedOutcome)
	}
	fmt.Printf("integration:criticGate PASS - canonical critic accepted the %s candidate -> Succeeded and rejected the non-conforming candidate -> Failed\n",
		acceptedSource)
	return nil
}

func freshCandidateFixture(appRoot, name string) (string, func(), error) {
	runDir, err := os.MkdirTemp("", "coding-loop-critic-candidate-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(runDir) }
	source := filepath.Join(appRoot, "testdata", "integration", "coding-loop", "candidates", name)
	workspace := filepath.Join(runDir, name)
	if err := copyTree(source, workspace); err != nil {
		cleanup()
		return "", nil, err
	}
	return workspace, cleanup, nil
}

type canonicalCriticVerdict struct {
	SchemaVersion string `json:"schema_version"`
	Mode          string `json:"mode"`
	Verdict       string `json:"verdict"`
	Accepted      bool   `json:"accepted"`
	Oracle        struct {
		Command string `json:"command"`
		Status  string `json:"status"`
	} `json:"oracle"`
}

func runCanonicalCriticVerdict(binary string, roots integrationRoots, workspace string) (canonicalCriticVerdict, error) {
	_ = os.Remove(filepath.Join(workspace, "critic-verdict.json"))
	run, err := runBuiltAgent(binary, roots.Profiles, roots.Core,
		"agents/critic/profile-workspace.yaml", workspace)
	if err != nil {
		return canonicalCriticVerdict{}, err
	}
	data, err := os.ReadFile(filepath.Join(workspace, "critic-verdict.json"))
	if err != nil {
		return canonicalCriticVerdict{}, fmt.Errorf("canonical critic emitted no verdict: %w\n%s", err, run.Output)
	}
	var verdict canonicalCriticVerdict
	if err := json.Unmarshal(data, &verdict); err != nil {
		return canonicalCriticVerdict{}, fmt.Errorf("parse canonical critic verdict: %w\n%s", err, data)
	}
	if verdict.SchemaVersion != "1" || verdict.Mode != "changed-workspace" ||
		verdict.Oracle.Command != "go test ./..." {
		return canonicalCriticVerdict{}, fmt.Errorf("invalid canonical critic verdict contract: %s", data)
	}
	switch verdict.Verdict {
	case "accepted":
		if !verdict.Accepted || verdict.Oracle.Status != "passed" ||
			run.ExitCode != 0 || run.FinalState != "Succeeded" {
			return canonicalCriticVerdict{}, fmt.Errorf("inconsistent accepting critic verdict/run: %s; exit=%d state=%s",
				data, run.ExitCode, run.FinalState)
		}
	case "rejected":
		if verdict.Accepted || verdict.Oracle.Status != "failed" ||
			run.ExitCode != 2 || run.FinalState != "Rejected" {
			return canonicalCriticVerdict{}, fmt.Errorf("inconsistent rejecting critic verdict/run: %s; exit=%d state=%s",
				data, run.ExitCode, run.FinalState)
		}
	default:
		return canonicalCriticVerdict{}, fmt.Errorf("unknown canonical critic verdict: %s", data)
	}
	return verdict, nil
}

func applicationOutcome(verdict canonicalCriticVerdict) (string, error) {
	switch verdict.Verdict {
	case "accepted":
		if !verdict.Accepted {
			return "", fmt.Errorf("accepted verdict has accepted=false")
		}
		return "Succeeded", nil
	case "rejected":
		if verdict.Accepted {
			return "", fmt.Errorf("rejected verdict has accepted=true")
		}
		return "Failed", nil
	default:
		return "", fmt.Errorf("cannot map unknown critic verdict %q", verdict.Verdict)
	}
}

type codingLoopStageFunctions struct {
	executor func() error
	planner  func() (string, func(), error)
	critic   func(string) error
}

func runCodingLoopStages(stages codingLoopStageFunctions) error {
	if err := stages.executor(); err != nil {
		return fmt.Errorf("stage A executorLive: %w", err)
	}
	workspace, cleanupWorkspace, err := stages.planner()
	if err != nil {
		return fmt.Errorf("stage B plannerDelegation: %w", err)
	}
	defer cleanupWorkspace()
	if err := stages.critic(workspace); err != nil {
		return fmt.Errorf("stage C criticGate: %w", err)
	}
	return nil
}

// CodingLoop runs all three stages while retaining the exact Stage B workspace
// until Stage C has reviewed it.
func (i Integration) CodingLoop() error {
	roots, err := resolveIntegrationRoots()
	if err != nil {
		return err
	}
	if err := validateReleaseDeadlines(); err != nil {
		return err
	}
	if reason := liveSkipReason(roots, "bd", "sh"); reason != "" {
		fmt.Printf("SKIP codingLoop: %s\n", reason)
		return nil
	}
	roots, cleanupProfiles, err := packageIntegrationRoots(roots)
	if err != nil {
		return err
	}
	defer cleanupProfiles()
	binary, cleanupBinary, err := buildAgent(roots.Core)
	if err != nil {
		return err
	}
	defer cleanupBinary()
	model := newReleaseCodingModel()
	defer model.Close()

	err = runCodingLoopStages(codingLoopStageFunctions{
		executor: i.ExecutorLive,
		planner: func() (string, func(), error) {
			workspace, cleanup, err := producePlannerCandidate(roots, binary, model.URL)
			if err != nil {
				return "", nil, err
			}
			plannerCalls, _ := model.invocationCounts()
			if plannerCalls != 1 {
				cleanup()
				return "", nil, fmt.Errorf("planner model invocations = %d, want exactly 1", plannerCalls)
			}
			return workspace, cleanup, nil
		},
		critic: func(workspace string) error {
			return runCriticGate(binary, roots, workspace, "exact Stage B workspace")
		},
	})
	if err != nil {
		return err
	}
	plannerCalls, _ := model.invocationCounts()
	if plannerCalls != 1 {
		return fmt.Errorf("composite planner model invocations = %d, want exactly 1", plannerCalls)
	}
	fmt.Println("integration:codingLoop PASS - Stage C reviewed the exact retained Stage B workspace after one planner model invocation")
	return nil
}

func validateReleaseDeadlines() error {
	if releaseModelCallTimeout >= childAgentRunTimeout {
		return fmt.Errorf("release deadline hierarchy: model call %s must be less than child run %s",
			releaseModelCallTimeout, childAgentRunTimeout)
	}
	if childAgentRunTimeout+processGroupCleanupGrace >= outerEmergencyTimeout {
		return fmt.Errorf("release deadline hierarchy: child run %s plus cleanup grace %s must be less than outer emergency %s",
			childAgentRunTimeout, processGroupCleanupGrace, outerEmergencyTimeout)
	}
	return nil
}

func releaseAgentRunOptions(modelURL string) agentRunOptions {
	return agentRunOptions{
		Timeout:      outerEmergencyTimeout,
		CleanupGrace: processGroupCleanupGrace,
		Env:          []string{"OLLAMA_URL=" + modelURL},
	}
}

type releaseCodingModel struct {
	*httptest.Server
	mu            sync.Mutex
	plannerCalls  int
	executorCalls int
	callTimeout   time.Duration
	responseDelay time.Duration
}

func newReleaseCodingModel() *releaseCodingModel {
	return newReleaseCodingModelWithTiming(releaseModelCallTimeout, 0)
}

func newReleaseCodingModelWithTiming(callTimeout, responseDelay time.Duration) *releaseCodingModel {
	model := &releaseCodingModel{callTimeout: callTimeout, responseDelay: responseDelay}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
		writeServingJSON(w, map[string]any{"models": []map[string]string{{"name": canonicalModel}}})
	})
	mux.HandleFunc("/api/chat", model.chat)
	model.Server = httptest.NewServer(mux)
	return model
}

func (model *releaseCodingModel) chat(w http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), model.callTimeout)
	defer cancel()

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
			strings.Contains(message.Content, "software planning assistant") ||
			strings.Contains(message.Content, "# Implementation Planning") {
			planner = true
			break
		}
	}

	model.mu.Lock()
	if planner {
		model.plannerCalls++
	} else {
		model.executorCalls++
	}
	executorCall := model.executorCalls
	model.mu.Unlock()

	if model.responseDelay > 0 {
		timer := time.NewTimer(model.responseDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusGatewayTimeout)
			_, _ = w.Write([]byte(`{"error":"deterministic model call deadline exceeded"}`))
			return
		}
	}

	content := `title: Implement greeting
summary: Implement the SRD greeting and validate the workspace.
files:
  - path: greet.go
    action: modify
    note: Return the required greeting.
requirements:
  - id: R1
    text: Return the required greeting.
design_decisions:
  - id: D1
    text: Make the smallest source-only change.
acceptance_criteria:
  - id: AC1
    text: go test ./... passes.
`
	if !planner {
		if executorCall == 1 {
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

func (model *releaseCodingModel) invocationCounts() (planner, executor int) {
	model.mu.Lock()
	defer model.mu.Unlock()
	return model.plannerCalls, model.executorCalls
}
