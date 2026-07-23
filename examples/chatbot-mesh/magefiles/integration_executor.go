// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	execApplyURL   = "http://127.0.0.1:18090/provisioning/api/apply"
	execRolloutURL = "http://127.0.0.1:18090/provisioning/api/rollout"
	execStateURL   = "http://127.0.0.1:18090/provisioning/api/state"
	execControlURL = "http://127.0.0.1:18091/api/lifecycle/health"
	execReadyWait  = 30 * time.Second
)

// Executor proves the executor's validate -> apply -> verify -> rollback flow and
// its HTTP contracts against the shipped profile (srd006 R2, R3, R4; rel06.0
// uc001). It is the tracer test-rel06.0-executor names.
//
// The executor's exec words declare `binary: helm` and `binary: kubectl` with no
// path, so putting recording fakes ahead of the real tools on PATH is enough to
// drive every leg. Nothing in the profile needs a test-only branch, which is the
// point: this runs the declaration that ships, and a word dropped from the
// machine or an argv contract changed in exec-declarations.yaml fails here.
//
// What this does not prove: that real helm and kubectl behave as the
// declarations assume. The fakes take their exit codes from the scenario, so
// this is evidence about the machine, the arguments it constructs, and the
// responses it maps -- not about a cluster. The chart schema is proven
// separately against real helm, and the live tier is GH-735.
func (Integration) Executor() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault(agentCoreRootEnv, siblingPath(profilesRoot, "agent-core"))
	if err := requireProfilePaths(profilesRoot,
		"agents/executor/profile.yaml", "agents/executor/apply-machine.yaml",
		"agents/executor/rollout-machine.yaml", "agents/executor/state-machine.yaml",
		"agents/executor/exec-declarations.yaml",
	); err != nil {
		return err
	}
	if !agentCoreAvailable(coreRoot) {
		fmt.Printf("SKIP executor: agent-core checkout not found at %s (set %s)\n", coreRoot, agentCoreRootEnv)
		return nil
	}
	return runExecutorIntegration(profilesRoot, coreRoot)
}

func runExecutorIntegration(profilesRoot, coreRoot string) error {
	binary, err := buildAgent(coreRoot)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(binary) }()

	fakes, err := newExecutorFakes()
	if err != nil {
		return err
	}
	defer fakes.cleanup()

	trace, traceCleanup, err := chromaTraceFile("executor")
	if err != nil {
		return err
	}
	defer traceCleanup()

	stop, err := startDetachedAgentWithEnv(agentLaunch{
		Binary: binary, ProfilesRoot: profilesRoot, CoreRoot: coreRoot,
		Profile: "agents/executor/profile.yaml", TracePath: trace,
		// The workspace the values file lands in, and the same value the
		// deployment sets from workMountPath (GH-737). Without it write_overrides
		// resolves /work against a workspace that does not contain it and every
		// apply dies on its first word.
		Workdir: fakes.workDir,
		Env: []string{
			"PATH=" + fakes.binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
			"EXECUTOR_WORK_DIR=" + fakes.workDir,
		},
		GracefulWait: 15 * time.Second,
	})
	if err != nil {
		return err
	}
	stopped := false
	defer func() {
		if !stopped {
			_ = stop(true)
		}
	}()
	if err := waitHTTPStatus(execControlURL, http.StatusOK, execReadyWait); err != nil {
		return fmt.Errorf("executor control health never came up: %w", err)
	}

	for _, scenario := range executorScenarios() {
		if err := runExecutorScenario(fakes, scenario); err != nil {
			return fmt.Errorf("%s: %w", scenario.name, err)
		}
		fmt.Printf("executor: %s\n", scenario.name)
	}

	stopped = true
	if err := stop(true); err != nil {
		return err
	}
	fmt.Println("integration:executor PASS - the shipped executor walked every apply and rollout leg, " +
		"constructed the declared helm and kubectl arguments, and mapped each terminal to its contract response")
	return nil
}

// runExecutorScenario primes the fakes for one outcome, drives the endpoint, and
// checks the response and the calls the run actually made.
func runExecutorScenario(fakes *executorFakes, scenario executorScenario) error {
	if err := fakes.plan(scenario.exits, scenario.stdout); err != nil {
		return err
	}
	body, status, err := executorRequest(scenario)
	if err != nil {
		return err
	}
	if status != scenario.wantStatus {
		return fmt.Errorf("status = %d, want %d: %s", status, scenario.wantStatus, body)
	}
	for _, want := range scenario.wantBody {
		if !strings.Contains(string(body), want) {
			return fmt.Errorf("response body missing %q: %s", want, body)
		}
	}
	calls, err := fakes.calls()
	if err != nil {
		return err
	}
	return assertExecutorCalls(calls, scenario)
}

func executorRequest(scenario executorScenario) ([]byte, int, error) {
	if scenario.applyBody != "" {
		return requestInference(http.MethodPost, execApplyURL, scenario.applyBody, "executor apply")
	}
	if scenario.stateRead {
		return requestInference(http.MethodGet, execStateURL, "", "executor state read")
	}
	return requestInference(http.MethodGet, execRolloutURL, "", "executor rollout read")
}

// assertExecutorCalls checks what the run invoked, which is where an argv
// contract lives. A response alone cannot tell a values-file apply from a
// per-field --set one, nor prove that a rejected patch stopped before the apply.
func assertExecutorCalls(calls []string, scenario executorScenario) error {
	joined := strings.Join(calls, "\n")
	for _, want := range scenario.wantCalls {
		if !strings.Contains(joined, want) {
			return fmt.Errorf("no recorded call contains %q; calls were:\n%s", want, joined)
		}
	}
	for _, absent := range scenario.absentCalls {
		if strings.Contains(joined, absent) {
			return fmt.Errorf("a recorded call contains %q, which this leg must not reach; calls were:\n%s",
				absent, joined)
		}
	}
	// The authority boundary: the executor edits values and triggers rollouts
	// only. No invocation may carry an endpoint or credential for a running agent
	// (srd006 R2.3, R4.2).
	if problem := executorAuthorityProblem(calls); problem != "" {
		return fmt.Errorf("authority boundary: %s; calls were:\n%s", problem, joined)
	}
	return nil
}

// executorAuthorityProblem reports an invocation that carries transport
// authority, or "" when none does.
func executorAuthorityProblem(calls []string) string {
	for _, call := range calls {
		for _, marker := range []string{"http://", "https://", "--token", "Bearer ", "--kubeconfig"} {
			if strings.Contains(call, marker) {
				return fmt.Sprintf("invocation carries %q", marker)
			}
		}
		// A per-field --set is the construction srd006 R2.2 forbids: the patch
		// travels as a values file so the chart schema validates it whole.
		if strings.Contains(call, "--set") {
			return "invocation constructs per-field --set arguments"
		}
	}
	return ""
}

// executorFakes is a PATH directory holding recording helm and kubectl stand-ins,
// the workspace the values file lands in, and the plan the fakes read their exit
// codes from.
type executorFakes struct {
	root    string
	binDir  string
	planDir string
	workDir string
	logPath string
}

func newExecutorFakes() (*executorFakes, error) {
	root, err := os.MkdirTemp("", "executor-tracer-*")
	if err != nil {
		return nil, err
	}
	fakes := &executorFakes{
		root:    root,
		binDir:  filepath.Join(root, "bin"),
		planDir: filepath.Join(root, "plan"),
		workDir: filepath.Join(root, "work"),
		logPath: filepath.Join(root, "calls.log"),
	}
	for _, dir := range []string{fakes.binDir, fakes.planDir, fakes.workDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fakes.cleanup()
			return nil, err
		}
	}
	for name, classify := range map[string]string{
		"helm":    helmVerbCase,
		"kubectl": kubectlVerbCase,
	} {
		script := fakeScript(name, fakes.logPath, fakes.planDir, classify)
		if err := os.WriteFile(filepath.Join(fakes.binDir, name), []byte(script), 0o755); err != nil {
			fakes.cleanup()
			return nil, err
		}
	}
	return fakes, nil
}

func (f *executorFakes) cleanup() { _ = os.RemoveAll(f.root) }

// plan writes the exit code each verb should take and any stdout it should emit,
// and clears the call log so a scenario sees only its own invocations.
func (f *executorFakes) plan(exits map[string]int, stdout map[string]string) error {
	if err := os.RemoveAll(f.planDir); err != nil {
		return err
	}
	if err := os.MkdirAll(f.planDir, 0o755); err != nil {
		return err
	}
	for verb, code := range exits {
		path := filepath.Join(f.planDir, "exit."+verb)
		if err := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", code)), 0o644); err != nil {
			return err
		}
	}
	for verb, out := range stdout {
		path := filepath.Join(f.planDir, "stdout."+verb)
		if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(f.logPath, nil, 0o644)
}

func (f *executorFakes) calls() ([]string, error) {
	data, err := os.ReadFile(f.logPath)
	if err != nil {
		return nil, fmt.Errorf("read recorded calls: %w", err)
	}
	var calls []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			calls = append(calls, line)
		}
	}
	return calls, nil
}

// helmVerbCase and kubectlVerbCase classify an invocation into the leg it serves,
// so one scenario can fail the verify read while the apply succeeds. The two
// kubectl rollout reads differ only by their timeout, which is what the poll and
// the verify declare (exec-declarations.yaml).
const helmVerbCase = `case "$*" in
  *--dry-run*) verb=dry-run ;;
  rollback*) verb=rollback ;;
  "get values"*) verb=get-values ;;
  upgrade*) verb=upgrade ;;
  *) verb=other ;;
esac`

const kubectlVerbCase = `case "$*" in
  *"--timeout 3s"*) verb=poll ;;
  "rollout status"*) verb=verify ;;
  get*) verb=counts ;;
  *) verb=other ;;
esac`

// fakeScript is a recording stand-in: it appends its argv to the shared log,
// classifies itself, and takes its exit code and stdout from the planned files.
// An unplanned verb exits zero, so a scenario states only what it varies.
func fakeScript(name, logPath, planDir, classify string) string {
	return fmt.Sprintf(`#!/bin/sh
echo "%s $*" >> %q
%s
if [ -f %q/stdout.$verb ]; then cat %q/stdout.$verb; fi
if [ -f %q/exit.$verb ]; then exit "$(cat %q/exit.$verb)"; fi
exit 0
`, name, logPath, classify, planDir, planDir, planDir, planDir)
}
