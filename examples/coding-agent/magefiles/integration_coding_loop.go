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
	const attempts = 1
	var failures []string
	for attempt := 1; attempt <= attempts; attempt++ {
		_, cleanupWorkspace, err := producePlannerCandidate(roots, binary)
		if err != nil {
			failures = append(failures, fmt.Sprintf("attempt %d: %v", attempt, err))
			if attempt < attempts {
				fmt.Printf("plannerDelegation: attempt %d/%d did not complete; retrying from a fresh workspace\n", attempt, attempts)
			}
			continue
		}
		cleanupWorkspace()
		fmt.Println("integration:plannerDelegation PASS - canonical planner materialized a local task and delegated to the real canonical executor")
		return nil
	}
	return fmt.Errorf("planner did not complete in %d bounded attempts:\n%s", attempts, strings.Join(failures, "\n"))
}

func producePlannerCandidate(roots integrationRoots, binary string) (string, func(), error) {
	workspace, cleanupWorkspace, err := freshWorkspace(roots.Application)
	if err != nil {
		return "", nil, err
	}
	if err := initializePlannerWorkspace(workspace); err != nil {
		cleanupWorkspace()
		return "", nil, err
	}
	run, err := runBuiltAgent(binary, roots.Profiles, roots.Core, "agents/planner/profile.yaml", workspace,
		"--child-agent-binary", binary, "--verbose-trace")
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

// CriticGate gives the canonical changed-workspace critic two existing
// candidates and maps only its machine-readable verdicts to application
// outcomes. When the live planner is available, the accepted candidate is the
// actual workspace it produced; otherwise the target records the clean Stage B
// skip and uses the deterministic conforming candidate fixture.
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

	accepted, cleanupAccepted, acceptedSource, err := acceptedCriticCandidate(roots, binary)
	if err != nil {
		return err
	}
	defer cleanupAccepted()
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

func acceptedCriticCandidate(roots integrationRoots, binary string) (string, func(), string, error) {
	if reason := liveSkipReason(roots, "bd"); reason == "" {
		workspace, cleanup, err := producePlannerCandidate(roots, binary)
		return workspace, cleanup, "Stage B planner", err
	} else {
		fmt.Printf("SKIP criticGate Stage B candidate: %s; using deterministic conforming fixture\n", reason)
	}
	workspace, cleanup, err := freshCandidateFixture(roots.Application, "accepted")
	return workspace, cleanup, "deterministic conforming fixture", err
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
