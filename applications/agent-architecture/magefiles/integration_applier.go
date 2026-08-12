// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	applierApplyURL         = "http://127.0.0.1:18330/api/v1/apply"
	applierRolloutURL       = "http://127.0.0.1:18330/api/v1/rollout"
	applierControlHealthURL = "http://127.0.0.1:18331/api/lifecycle/health"
	applierControlExitURL   = "http://127.0.0.1:18331/api/lifecycle/exit"
	applierReadyTimeout     = 30 * time.Second
	applierRequestTimeout   = 130 * time.Second
)

// Applier proves the applier's validate -> apply -> verify -> rollback flow and its
// HTTP contracts against the shipped profile (srd002-applier R2, R3, R4). It drives
// the applier that ships in agents/applier against recording fake helm and kubectl.
//
// The applier's exec words declare `binary: helm` and `binary: kubectl` with no
// path, so putting recording fakes ahead of the real tools on PATH is enough to
// drive every leg. Nothing in the profile needs a test-only branch: this runs the
// declaration that ships, and a word dropped from a machine or an argv contract
// changed in exec-declarations.yaml fails here.
//
// What this does not prove: that real helm and kubectl behave as the declarations
// assume. The fakes take their exit codes from the scenario, so this is evidence
// about the machines, the arguments they construct, and the responses they map --
// not about a cluster. The chart schema is proven separately against real helm
// (applier_schema_test.go), and the live tier is integration:applierLive.
func (Integration) Applier() error {
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		fmt.Printf("SKIP applier: %v\n", err)
		return nil
	}
	if reason := applierSkipReason(resolved); reason != "" {
		fmt.Printf("SKIP applier: %s\n", reason)
		return nil
	}
	return runApplierIntegration(resolved)
}

// applierSkipReason reports why the fake-CLI tracer cannot run, or "" when every
// dependency is present. It needs the Go toolchain, git, and sh (the recording
// fakes are shell scripts on the applier's PATH), plus the agent-core checkout.
func applierSkipReason(resolved roots) string {
	for _, binary := range []string{"go", "git", "sh"} {
		if _, err := exec.LookPath(binary); err != nil {
			return binary + " not found on PATH"
		}
	}
	if info, err := os.Stat(filepath.Join(resolved.Core, "go.mod")); err != nil || info.IsDir() {
		return "agent-core checkout not found at " + resolved.Core
	}
	if info, err := os.Stat(filepath.Join(resolved.Application, "agents", "applier", "profile.yaml")); err != nil || info.IsDir() {
		return "applier profile not found"
	}
	return ""
}

func runApplierIntegration(resolved roots) error {
	binary, cleanupBinary, err := buildApplierBinary(resolved.Core)
	if err != nil {
		return err
	}
	defer cleanupBinary()

	fakes, err := newApplierFakes()
	if err != nil {
		return err
	}
	defer fakes.cleanup()

	traceDir, err := os.MkdirTemp("", "agent-architecture-applier-traces-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(traceDir) }()

	profileStage, err := os.MkdirTemp("", "agent-architecture-applier-profiles-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(profileStage) }()
	if err := prepareChartProfiles(resolved.Application, resolved.Catalog, profileStage); err != nil {
		return err
	}
	profileRoot := filepath.Join(profileStage, "profiles", "applier")

	agent, err := startApplierAgent(binary, resolved, profileRoot, fakes,
		filepath.Join(traceDir, "applier.ndjson"))
	if err != nil {
		return err
	}
	stopped := false
	defer func() {
		if !stopped {
			_ = stopApplierAgent(agent)
		}
	}()

	if err := waitHTTP200(applierControlHealthURL, applierReadyTimeout); err != nil {
		return fmt.Errorf("applier control health never came up: %w%s", err, agent.diagnostics())
	}

	for _, scenario := range applierScenarios() {
		if err := runApplierScenario(fakes, scenario); err != nil {
			return fmt.Errorf("%s: %w%s", scenario.name, err, agent.diagnostics())
		}
		fmt.Printf("applier: %s\n", scenario.name)
	}

	stopped = true
	if err := stopApplierAgent(agent); err != nil {
		return err
	}
	fmt.Println("integration:applier PASS - the shipped applier walked every apply and rollout leg, " +
		"constructed the declared helm and kubectl arguments, verified the collector Deployment, " +
		"and mapped each terminal to its contract response")
	return nil
}

// buildApplierBinary builds the agent-core runtime the applier profile runs on.
func buildApplierBinary(coreRoot string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "agent-architecture-applier-binary-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	binary := filepath.Join(dir, "agent")
	cmd := exec.Command("go", "build", "-tags", "production", "-o", binary, "./cmd/agent")
	cmd.Dir = coreRoot
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	fmt.Printf("building real agent binary from %s\n", coreRoot)
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("build agent: %w", err)
	}
	return binary, cleanup, nil
}

// runApplierScenario primes the fakes for one outcome, drives the endpoint, and
// checks the response and the calls the run actually made.
func runApplierScenario(fakes *applierFakes, scenario applierScenario) error {
	if err := fakes.plan(scenario.exits, scenario.stdout); err != nil {
		return err
	}
	status, body, err := applierRequest(scenario)
	if err != nil {
		return err
	}
	if status != scenario.wantStatus {
		return fmt.Errorf("status = %d, want %d: %s", status, scenario.wantStatus, body)
	}
	for _, want := range scenario.wantBody {
		if !strings.Contains(body, want) {
			return fmt.Errorf("response body missing %q: %s", want, body)
		}
	}
	calls, err := fakes.calls()
	if err != nil {
		return err
	}
	return assertApplierCalls(calls, scenario)
}

func applierRequest(scenario applierScenario) (int, string, error) {
	if scenario.applyBody != "" {
		return applierHTTP(http.MethodPost, applierApplyURL, scenario.applyBody)
	}
	return applierHTTP(http.MethodGet, applierRolloutURL, "")
}

func applierHTTP(method, url, body string) (int, string, error) {
	return applierHTTPWithTimeout(method, url, body, applierRequestTimeout)
}

// applierHTTPWithTimeout is applierHTTP with a caller-chosen deadline. The live
// tier's apply legs run a real helm upgrade, a 120s kubectl rollout verify, and a
// helm rollback, so they need a bound well past the fake-tracer's default.
func applierHTTPWithTimeout(method, url, body string, timeout time.Duration) (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return 0, "", err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, "", err
	}
	return resp.StatusCode, strings.TrimSpace(string(data)), nil
}

// assertApplierCalls checks what the run invoked, which is where an argv contract
// lives. A response alone cannot tell a values-file apply from a per-field --set
// one, nor prove that a rejected patch stopped before the apply.
func assertApplierCalls(calls []string, scenario applierScenario) error {
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
	// The authority boundary: the applier edits values and triggers rollouts only.
	// No invocation may carry an endpoint or credential for a running agent
	// (srd002-applier R2.3, R4.2).
	if problem := applierAuthorityProblem(calls); problem != "" {
		return fmt.Errorf("authority boundary: %s; calls were:\n%s", problem, joined)
	}
	return nil
}

// applierAuthorityProblem reports an invocation that carries transport authority, or
// "" when none does.
func applierAuthorityProblem(calls []string) string {
	for _, call := range calls {
		for _, marker := range []string{"http://", "https://", "--token", "Bearer ", "--kubeconfig"} {
			if strings.Contains(call, marker) {
				return fmt.Sprintf("invocation carries %q", marker)
			}
		}
		// A per-field --set is the construction srd002-applier R2.2 forbids: the
		// patch travels as a values file so the chart schema validates it whole.
		if strings.Contains(call, "--set") {
			return "invocation constructs per-field --set arguments"
		}
	}
	return ""
}

// applierAgent is a detached applier server driven over HTTP by the tracer.
type applierAgent struct {
	cmd    *exec.Cmd
	output *bytes.Buffer
	done   chan error
}

func (a *applierAgent) diagnostics() string {
	if a == nil || a.output == nil {
		return ""
	}
	if output := strings.TrimSpace(a.output.String()); output != "" {
		return "\napplier output:\n" + output
	}
	return ""
}

// startApplierAgent launches the shipped applier profile as a detached server with
// the recording fakes ahead of the real tools on its PATH, and its workspace set to
// the fakes' work directory so write_overrides lands inside it.
func startApplierAgent(binary string, resolved roots, profilesRoot string, fakes *applierFakes, tracePath string) (*applierAgent, error) {
	profile := filepath.Join(profilesRoot, "applications", "agent-architecture", "applier", "profile.yaml")
	var output bytes.Buffer
	cmd := exec.Command(binary,
		"--profile", profile,
		"--directory", fakes.workDir,
		"--core-root", resolved.Core,
		"--otel-log-file", tracePath,
		"--otel-service-name", "agent-architecture-applier",
	)
	cmd.Dir = fakes.workDir
	cmd.Env = append(os.Environ(),
		// The fakes are shell scripts, so the ordinary tools stay on PATH; the
		// fakes' bin dir is prepended so helm and kubectl resolve to them.
		childPathWithPrefix(os.Environ(), fakes.binDir),
		// The workspace the values file lands in, matching what the chart sets from
		// applier.workDir (APPLIER_WORK_DIR). Without it write_overrides resolves
		// /work against a workspace that does not contain it and every apply dies on
		// its first word.
		"APPLIER_WORK_DIR="+fakes.workDir,
	)
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start applier server: %w", err)
	}
	agent := &applierAgent{cmd: cmd, output: &output, done: make(chan error, 1)}
	go func() { agent.done <- cmd.Wait() }()
	return agent, nil
}

func stopApplierAgent(agent *applierAgent) error {
	if agent == nil || agent.cmd == nil || agent.cmd.Process == nil {
		return nil
	}
	_, _, _ = applierHTTP(http.MethodPost, applierControlExitURL, `{"reason":"integration cleanup"}`)
	select {
	case err := <-agent.done:
		if err != nil {
			return fmt.Errorf("applier exit: %v%s", err, agent.diagnostics())
		}
		return nil
	case <-time.After(10 * time.Second):
		_ = agent.cmd.Process.Kill()
		<-agent.done
		return fmt.Errorf("applier graceful exit timed out%s", agent.diagnostics())
	}
}

// applierFakes is a PATH directory holding recording helm and kubectl stand-ins, the
// workspace the values file lands in, and the plan the fakes read their exit codes
// from.
type applierFakes struct {
	root    string
	binDir  string
	planDir string
	workDir string
	logPath string
}

func newApplierFakes() (*applierFakes, error) {
	root, err := os.MkdirTemp("", "applier-tracer-*")
	if err != nil {
		return nil, err
	}
	fakes := &applierFakes{
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

func (f *applierFakes) cleanup() { _ = os.RemoveAll(f.root) }

// plan writes the exit code each verb should take and any stdout it should emit, and
// clears the call log so a scenario sees only its own invocations.
func (f *applierFakes) plan(exits map[string]int, stdout map[string]string) error {
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

func (f *applierFakes) calls() ([]string, error) {
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

// helmVerbCase and kubectlVerbCase classify an invocation into the leg it serves, so
// one scenario can fail the verify read while the apply succeeds. The two kubectl
// rollout reads differ only by their timeout, which is what the poll and the verify
// declare (exec-declarations.yaml). The applier has no state endpoint, so there is
// no `helm get values` leg.
const helmVerbCase = `case "$*" in
  *--dry-run*) verb=dry-run ;;
  rollback*) verb=rollback ;;
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
// classifies itself, and takes its exit code and stdout from the planned files. An
// unplanned verb exits zero, so a scenario states only what it varies.
func fakeScript(name, logPath, planDir, classify string) string {
	return fmt.Sprintf(`#!/bin/sh
echo "%s $*" >> %q
%s
if [ -f %q/stdout.$verb ]; then cat %q/stdout.$verb; fi
if [ -f %q/exit.$verb ]; then exit "$(cat %q/exit.$verb)"; fi
exit 0
`, name, logPath, classify, planDir, planDir, planDir, planDir)
}
