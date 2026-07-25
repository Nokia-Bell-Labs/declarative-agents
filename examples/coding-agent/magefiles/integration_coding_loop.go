// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	if reason := liveSkipReason(roots); reason != "" {
		fmt.Printf("SKIP executorLive: %s\n", reason)
		return nil
	}
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
	run, err := runBuiltAgent(binary, roots.Profiles, roots.Core, "agents/executor/profile.yaml", workspace)
	if err != nil {
		return err
	}
	if err := requireSuccessfulExecutor(workspace, run); err != nil {
		return err
	}
	fmt.Println("integration:executorLive PASS - canonical executor changed the greet workspace and go test ./... passed")
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
	if reason := liveSkipReason(roots, "bd"); reason != "" {
		fmt.Printf("SKIP plannerDelegation: %s\n", reason)
		return nil
	}
	binary, cleanupBinary, err := buildAgent(roots.Core)
	if err != nil {
		return err
	}
	defer cleanupBinary()
	const attempts = 1
	var failures []string
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := runPlannerAttempt(roots, binary); err != nil {
			failures = append(failures, fmt.Sprintf("attempt %d: %v", attempt, err))
			if attempt < attempts {
				fmt.Printf("plannerDelegation: attempt %d/%d did not complete; retrying from a fresh workspace\n", attempt, attempts)
			}
			continue
		}
		fmt.Println("integration:plannerDelegation PASS - canonical planner materialized a local task and delegated to the real canonical executor")
		return nil
	}
	return fmt.Errorf("planner did not complete in %d bounded attempts:\n%s", attempts, strings.Join(failures, "\n"))
}

func runPlannerAttempt(roots integrationRoots, binary string) error {
	workspace, cleanupWorkspace, err := freshWorkspace(roots.Application)
	if err != nil {
		return err
	}
	defer cleanupWorkspace()
	if err := initializePlannerWorkspace(workspace); err != nil {
		return err
	}
	run, err := runBuiltAgent(binary, roots.Profiles, roots.Core, "agents/planner/profile.yaml", workspace,
		"--child-agent-binary", binary, "--verbose-trace")
	if err != nil {
		return err
	}
	if run.ExitCode != 0 {
		return fmt.Errorf("planner exited %d:\n%s\ntrace diagnostics:\n%s", run.ExitCode, run.Output, run.Trace)
	}
	if run.FinalState != "Completed" {
		return fmt.Errorf("planner final state = %q, want Completed:\n%s", run.FinalState, run.Output)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".git", "agent-planner", "issue-body.yaml")); err != nil {
		return fmt.Errorf("planner did not materialize its task: %w", err)
	}
	if err := requireGreetingAndTests(workspace); err != nil {
		return fmt.Errorf("delegated executor result: %w", err)
	}
	return nil
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
	data, err := os.ReadFile(filepath.Join(workspace, "greet.go"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), `return "Hello, " + name + "!"`) {
		return fmt.Errorf("greet.go does not contain the required implementation:\n%s", data)
	}
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = workspace
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go test ./...: %w\n%s", err, output)
	}
	return nil
}

// CriticGate executes the strongest boundary the current canonical critic
// contract supports: the real critic session invokes a real canonical executor,
// runs the configured oracle, records metrics, and reaches Done. The critic
// profile is a benchmark runner; it cannot consume an already-produced Stage B
// workspace or emit an accept/reject verdict. That contract gap is recorded in
// the application test suite and prevents this target from claiming the full
// Stage C gate.
func (Integration) CriticGate() error {
	roots, err := resolveIntegrationRoots()
	if err != nil {
		return err
	}
	if reason := liveSkipReason(roots); reason != "" {
		fmt.Printf("SKIP criticGate: %s\n", reason)
		return nil
	}
	binary, cleanupBinary, err := buildAgent(roots.Core)
	if err != nil {
		return err
	}
	defer cleanupBinary()
	runDir, err := os.MkdirTemp("", "coding-loop-critic-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(runDir)
	suite, err := stageCriticSuite(roots, runDir)
	if err != nil {
		return err
	}
	outputDir := filepath.Join(runDir, "results")
	workspace := filepath.Join(runDir, "critic-workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return err
	}
	run, err := runBuiltAgent(binary, roots.Profiles, roots.Core, "agents/critic/profile.yaml", workspace,
		"--request", suite, "--output", outputDir, "--child-agent-binary", binary)
	if err != nil {
		return err
	}
	if run.ExitCode != 0 {
		return fmt.Errorf("critic exited %d:\n%s", run.ExitCode, run.Output)
	}
	if run.FinalState != "Done" {
		return fmt.Errorf("critic final state = %q, want Done:\n%s", run.FinalState, run.Output)
	}
	if err := requirePassingCriticEvidence(outputDir); err != nil {
		return fmt.Errorf("%w\n%s", err, run.Output)
	}
	fmt.Println("integration:criticGate LIMITED PASS - canonical critic recorded a real executor point and passing oracle; current critic contracts expose no existing-candidate accept/reject gate")
	return nil
}

func stageCriticSuite(roots integrationRoots, runDir string) (string, error) {
	fixture := filepath.Join(roots.Application, "testdata", "integration", "coding-loop")
	staged := filepath.Join(runDir, "suite")
	if err := copyTree(fixture, staged); err != nil {
		return "", fmt.Errorf("stage critic fixture: %w", err)
	}
	suite := filepath.Join(staged, "critic-suite.yaml")
	data, err := os.ReadFile(suite)
	if err != nil {
		return "", err
	}
	data = []byte(strings.ReplaceAll(string(data), "@EXECUTOR_PROFILE@",
		filepath.Join(roots.Profiles, "agents", "executor", "profile.yaml")))
	if err := os.WriteFile(suite, data, 0o644); err != nil {
		return "", err
	}
	return suite, nil
}

func requirePassingCriticEvidence(outputDir string) error {
	var metaPaths []string
	if err := filepath.WalkDir(outputDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == "meta.json" {
			metaPaths = append(metaPaths, path)
		}
		return nil
	}); err != nil {
		return err
	}
	if len(metaPaths) != 1 {
		return fmt.Errorf("critic produced %d point metadata files, want 1", len(metaPaths))
	}
	data, err := os.ReadFile(metaPaths[0])
	if err != nil {
		return err
	}
	var meta struct {
		Harness     string `json:"harness"`
		TestsPassed bool   `json:"tests_passed"`
		TimedOut    bool   `json:"timed_out"`
		ExitCode    int    `json:"exit_code"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return fmt.Errorf("parse critic meta: %w", err)
	}
	if meta.Harness != "executor" || !meta.TestsPassed || meta.TimedOut || meta.ExitCode != 0 {
		return fmt.Errorf("critic point evidence is not an accepted real executor run: %s", data)
	}
	trace, err := os.ReadFile(filepath.Join(filepath.Dir(metaPaths[0]), "trace.ndjson"))
	if err != nil {
		return fmt.Errorf("read critic point trace: %w", err)
	}
	if !strings.Contains(string(trace), "gen_ai.usage.input_tokens") {
		return fmt.Errorf("critic point trace has no token evidence")
	}
	return nil
}

// CodingLoop runs the three independently addressable stages in order.
func (i Integration) CodingLoop() error {
	if err := i.ExecutorLive(); err != nil {
		return fmt.Errorf("stage A executorLive: %w", err)
	}
	if err := i.PlannerDelegation(); err != nil {
		return fmt.Errorf("stage B plannerDelegation: %w", err)
	}
	if err := i.CriticGate(); err != nil {
		return fmt.Errorf("stage C criticGate: %w", err)
	}
	return nil
}
