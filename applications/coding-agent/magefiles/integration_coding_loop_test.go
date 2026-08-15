// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestReleaseCodingModelIsDeterministicAndCountsInvocations(t *testing.T) {
	model := newReleaseCodingModel()
	defer model.Close()

	planner := postReleaseModel(t, model.URL, "You are a software planning assistant.")
	if !strings.Contains(planner, "design_decisions:") ||
		!strings.Contains(planner, "acceptance_criteria:") {
		t.Fatalf("planner response does not satisfy canonical schema:\n%s", planner)
	}
	firstExecutor := postReleaseModel(t, model.URL, "You are a Go developer.")
	secondExecutor := postReleaseModel(t, model.URL, "Continue the Go implementation.")
	if !strings.Contains(firstExecutor, `\"tool\":\"edit\"`) ||
		!strings.Contains(secondExecutor, `\"tool\":\"done\"`) {
		t.Fatalf("executor responses were not deterministic edit/done sequence:\n%s\n%s",
			firstExecutor, secondExecutor)
	}
	plannerCalls, executorCalls := model.invocationCounts()
	if plannerCalls != 1 || executorCalls != 2 {
		t.Fatalf("model invocations = planner %d executor %d, want 1/2",
			plannerCalls, executorCalls)
	}
}

func TestReleaseCodingModelBoundsSlowCall(t *testing.T) {
	model := newReleaseCodingModelWithTiming(40*time.Millisecond, time.Second)
	defer model.Close()

	start := time.Now()
	body := postReleaseModelStatus(t, model.URL, "software planning assistant", http.StatusGatewayTimeout)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("slow model terminated after %s, want under 500ms", elapsed)
	}
	if !strings.Contains(body, "model call deadline exceeded") {
		t.Fatalf("slow-model response = %s", body)
	}
	plannerCalls, _ := model.invocationCounts()
	if plannerCalls != 1 {
		t.Fatalf("slow planner invocations = %d, want 1", plannerCalls)
	}
}

func TestReleaseDeadlineHierarchy(t *testing.T) {
	if err := validateReleaseDeadlines(); err != nil {
		t.Fatal(err)
	}
	if !(releaseModelCallTimeout < childAgentRunTimeout &&
		childAgentRunTimeout+processGroupCleanupGrace < outerEmergencyTimeout) {
		t.Fatalf("invalid hierarchy: model=%s child=%s cleanup=%s outer=%s",
			releaseModelCallTimeout, childAgentRunTimeout,
			processGroupCleanupGrace, outerEmergencyTimeout)
	}
}

func postReleaseModel(t *testing.T, baseURL, prompt string) string {
	t.Helper()
	return postReleaseModelStatus(t, baseURL, prompt, http.StatusOK)
}

func postReleaseModelStatus(t *testing.T, baseURL, prompt string, wantStatus int) string {
	t.Helper()
	payload := `{"messages":[{"role":"system","content":` +
		strconv.Quote(prompt) + `}]}`
	response, err := http.Post(baseURL+"/api/chat", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("model status = %d, want %d: %s", response.StatusCode, wantStatus, data)
	}
	return string(data)
}

func TestTraceFinalStateReadsMachineTerminal(t *testing.T) {
	trace := `{"Attributes":[{"Key":"run.final_state","Value":{"Type":"STRING","Value":"Completed"}}]}`
	if got := traceFinalState(trace); got != "Completed" {
		t.Fatalf("traceFinalState = %q, want Completed", got)
	}
}

func TestCodingLoopStagesGenerateOnceAndPreserveExactWorkspace(t *testing.T) {
	const stageBWorkspace = "/tmp/exact-stage-b-workspace"
	plannerCalls := 0
	cleanups := 0
	var criticWorkspace string
	err := runCodingLoopStages(codingLoopStageFunctions{
		executor: func() error { return nil },
		planner: func() (string, func(), error) {
			plannerCalls++
			return stageBWorkspace, func() { cleanups++ }, nil
		},
		critic: func(workspace string) error {
			criticWorkspace = workspace
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plannerCalls != 1 {
		t.Fatalf("planner calls = %d, want 1", plannerCalls)
	}
	if criticWorkspace != stageBWorkspace {
		t.Fatalf("critic workspace = %q, want exact Stage B workspace %q",
			criticWorkspace, stageBWorkspace)
	}
	if cleanups != 1 {
		t.Fatalf("workspace cleanups = %d, want 1 after Stage C", cleanups)
	}
}

func TestRunBuiltAgentTimeoutRetainsPhaseDiagnosticsAndKillsGroup(t *testing.T) {
	const (
		diagnosticsTimeout = 10 * time.Second
		terminationBound   = 15 * time.Second
	)
	script := filepath.Join(t.TempDir(), "blocking-agent")
	writeTestFile(t, script, `#!/bin/sh
trace=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --otel-log-file) trace="$2"; shift 2 ;;
    *) shift ;;
  esac
done
printf '{"Attributes":[{"Key":"to_state","Value":{"Type":"STRING","Value":"%s"}},{"Key":"gen_ai.tool.name","Value":{"Type":"STRING","Value":"%s"}}]}\n' "$TRACE_STATE" "$TRACE_TOOL" > "$trace"
echo "retained timeout output for $MODE"
if [ "$MODE" = "child" ]; then
  sh -c 'trap "" TERM; echo "$$" > "$PID_FILE"; while :; do sleep 1; done' &
  wait
else
  trap "" TERM
  while :; do sleep 1; done
fi
`)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name    string
		mode    string
		profile string
		state   string
		tool    string
		phase   string
	}{
		{
			name: "slow planner model", mode: "model",
			profile: "agents/planner/profile.yaml",
			state:   "PlanInvoking", tool: "invoke_llm", phase: "planner model",
		},
		{
			name: "stuck child executor", mode: "child",
			profile: "agents/planner/profile.yaml",
			state:   "InvokingExecutor", tool: "self_invoke", phase: "child executor",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			pidFile := filepath.Join(t.TempDir(), "child.pid")
			options := agentRunOptions{
				Timeout:      diagnosticsTimeout,
				CleanupGrace: 100 * time.Millisecond,
				Env: []string{
					"MODE=" + test.mode,
					"TRACE_STATE=" + test.state,
					"TRACE_TOOL=" + test.tool,
					"PID_FILE=" + pidFile,
				},
			}
			workspace := filepath.Join(t.TempDir(), "workspace")
			if err := os.MkdirAll(workspace, 0o755); err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			run, err := runBuiltAgentWithOptions(
				script, t.TempDir(), t.TempDir(), test.profile, workspace, options,
			)
			if err == nil {
				t.Fatal("runBuiltAgentWithOptions succeeded, want timeout")
			}
			if elapsed := time.Since(start); elapsed > terminationBound {
				t.Fatalf("bounded termination took %s", elapsed)
			}
			for _, want := range []string{
				"outer emergency deadline",
				test.phase,
				`last state="` + test.state + `"`,
				`last tool="` + test.tool + `"`,
				"retained timeout output",
				"trace tail:",
			} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("timeout error missing %q:\n%v", want, err)
				}
			}
			if run.Phase != test.phase || run.LastState != test.state || run.LastTool != test.tool {
				t.Errorf("run diagnostics = phase %q state %q tool %q",
					run.Phase, run.LastState, run.LastTool)
			}
			if test.mode == "child" {
				assertProcessFromFileStopped(t, pidFile)
			}
		})
	}
}

func assertProcessFromFileStopped(t *testing.T, pidFile string) {
	t.Helper()
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("stuck child did not record pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("stuck child process %d survived process-group cleanup", pid)
}

func TestFreshWorkspaceIsPortableAndIsolated(t *testing.T) {
	appRoot := filepath.Clean(filepath.Join(".."))
	workspace, cleanup, err := freshWorkspace(appRoot)
	if err != nil {
		t.Fatalf("freshWorkspace: %v", err)
	}
	defer cleanup()
	for _, rel := range []string{
		"go.mod",
		"greet.go",
		"greet_test.go",
		filepath.Join("doc", "specs", "software-requirements", "srd001-greet.yaml"),
		filepath.Join("docs", "SPECIFICATIONS.yaml"),
	} {
		if _, err := os.Stat(filepath.Join(workspace, rel)); err != nil {
			t.Errorf("fresh workspace missing %s: %v", rel, err)
		}
	}
	if err := os.WriteFile(filepath.Join(workspace, "greet.go"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(appRoot, "testdata", "integration", "coding-loop", "workspace", "greet.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(source) == "changed" {
		t.Fatal("fresh workspace mutation changed the fixture")
	}
}

func TestPackagedIntegrationRootsDoNotObserveCheckoutMutations(t *testing.T) {
	appRoot := filepath.Join(t.TempDir(), "test")
	profilesRoot := t.TempDir()
	writeTestFile(t, filepath.Join(appRoot, "agents", "application.yaml"), `schema_version: 1
application: test
ownership: agent-owning
module_status: implemented
capabilities:
  runnable_module: {status: implemented, evidence: [test]}
  packaged: {status: implemented, evidence: [test]}
roots:
  - {id: executor, ownership: catalog, source: agents/executor/profile.yaml, runtime_path: agents/executor/profile.yaml, compatible_release: v0.test}
  - {id: planner, ownership: catalog, source: agents/planner/profile.yaml, runtime_path: agents/planner/profile.yaml, compatible_release: v0.test}
  - {id: critic, ownership: catalog, source: agents/critic/profile.yaml, runtime_path: agents/critic/profile.yaml, compatible_release: v0.test}
  - {id: critic-workspace, ownership: catalog, source: agents/critic/profile-workspace.yaml, runtime_path: agents/critic/profile-workspace.yaml, compatible_release: v0.test}
  - {id: coding-planner-server, ownership: local, source: agents/planner/profile.yaml, runtime_path: applications/coding-agent/planner/profile.yaml}
  - {id: coding-executor-server, ownership: local, source: agents/executor/profile.yaml, runtime_path: applications/coding-agent/executor/profile.yaml}
  - {id: coding-critic-server, ownership: local, source: agents/critic/profile.yaml, runtime_path: applications/coding-agent/critic/profile.yaml}
  - {id: applier, ownership: local, source: agents/applier/profile.yaml, runtime_path: applications/coding-agent/applier/profile.yaml}
runtime:
  mount_path: /profiles
  image_contains_profiles: false
deployment:
  entries:
    - {id: planner, root: coding-planner-server}
    - {id: executor, root: coding-executor-server}
    - {id: critic, root: coding-critic-server}
    - {id: applier, root: applier}
`)
	for _, actor := range []string{"planner", "executor", "critic", "applier"} {
		writeTestFile(t, filepath.Join(appRoot, "agents", actor, "profile.yaml"), "name: "+actor+"\n")
	}
	sourceProfile := filepath.Join(profilesRoot, "agents", "executor", "profile.yaml")
	writeTestFile(t, sourceProfile, "name: packaged-executor\n")
	writeTestFile(t, filepath.Join(profilesRoot, "agents", "planner", "profile.yaml"), "name: packaged-planner\n")
	writeTestFile(t, filepath.Join(profilesRoot, "agents", "critic", "profile.yaml"), "name: packaged-critic\n")
	writeTestFile(t, filepath.Join(profilesRoot, "agents", "critic", "profile-workspace.yaml"), "name: packaged-critic-workspace\n")
	coreRoot := t.TempDir()
	writeTestFile(t, filepath.Join(coreRoot, "go.mod"), "module test-core\n\ngo 1.26\n")

	packaged, cleanup, err := packageIntegrationRoots(integrationRoots{
		Application: appRoot,
		Core:        coreRoot,
		Profiles:    profilesRoot,
	})
	if err != nil {
		t.Fatalf("packageIntegrationRoots: %v", err)
	}
	packageParent := filepath.Dir(packaged.Profiles)
	defer cleanup()
	if packaged.Profiles == profilesRoot {
		t.Fatal("integration profile root still points at the checkout")
	}
	if reason := baseIntegrationSkipReason(packaged); reason != "" {
		t.Fatalf("packaged profile root was misclassified as unavailable: %s", reason)
	}
	packagedProfile := filepath.Join(packaged.Profiles, "agents", "executor", "profile.yaml")
	before, err := os.ReadFile(packagedProfile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourceProfile, []byte("name: mutated-checkout\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(packagedProfile)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || strings.Contains(string(after), "mutated-checkout") {
		t.Fatalf("packaged profile observed checkout mutation:\n%s", after)
	}
	cleanup()
	if _, err := os.Stat(packageParent); !os.IsNotExist(err) {
		t.Fatalf("temporary closure still exists after cleanup: %v", err)
	}
}

func TestObservableGreetingValidationAcceptsEquivalentImplementation(t *testing.T) {
	workspace := t.TempDir()
	writeTestFile(t, filepath.Join(workspace, "go.mod"), "module greet\n\ngo 1.26\n")
	writeTestFile(t, filepath.Join(workspace, "greet.go"), "package greet\n\nimport \"fmt\"\n\nfunc Hello(name string) string { return fmt.Sprintf(\"Hello, %s!\", name) }\n")
	writeTestFile(t, filepath.Join(workspace, "greet_test.go"), "package greet\n\nimport \"testing\"\n\nfunc TestHello(t *testing.T) { if Hello(\"Go\") != \"Hello, Go!\" { t.Fail() } }\n")
	if err := requireGreetingAndTests(workspace); err != nil {
		t.Fatalf("requireGreetingAndTests rejected equivalent implementation: %v", err)
	}
	if err := requireSuccessfulExecutor(workspace, agentRun{Output: "terminal state: Succeeded\n"}); err != nil {
		t.Fatalf("requireSuccessfulExecutor rejected equivalent implementation: %v", err)
	}
}

func TestApplicationOutcomeMapsOnlyCanonicalVerdicts(t *testing.T) {
	accepted := canonicalCriticVerdict{Verdict: "accepted", Accepted: true}
	if got, err := applicationOutcome(accepted); err != nil || got != "Succeeded" {
		t.Fatalf("accepted outcome = %q, %v", got, err)
	}
	rejected := canonicalCriticVerdict{Verdict: "rejected", Accepted: false}
	if got, err := applicationOutcome(rejected); err != nil || got != "Failed" {
		t.Fatalf("rejected outcome = %q, %v", got, err)
	}
	if _, err := applicationOutcome(canonicalCriticVerdict{Verdict: "accepted"}); err == nil {
		t.Fatal("accepted verdict with accepted=false must not map to success")
	}
}

func TestCriticCandidateFixturesHaveOppositeOracleResults(t *testing.T) {
	appRoot := filepath.Clean(filepath.Join(".."))
	for _, tc := range []struct {
		name     string
		wantPass bool
	}{
		{name: "accepted", wantPass: true},
		{name: "rejected", wantPass: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("go", "test", "./...")
			cmd.Dir = filepath.Join(appRoot, "testdata", "integration", "coding-loop", "candidates", tc.name)
			err := cmd.Run()
			if (err == nil) != tc.wantPass {
				t.Fatalf("go test error = %v, wantPass=%t", err, tc.wantPass)
			}
		})
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
