// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

const defaultRunTimeout = 90 * time.Second

// RunConfig describes a single agent CLI invocation for a family test.
type RunConfig struct {
	// Profile is the path to the profile YAML passed via --profile. Relative
	// paths resolve against the agent-profiles root.
	Profile string
	// Directory is the workspace passed via --directory (optional).
	Directory string
	// Request is the request file passed via --request (optional).
	Request string
	// Output is the output directory passed via --output (optional).
	Output string
	// Args are extra CLI arguments appended after the standard flags.
	Args []string
	// Env is appended to the child process environment.
	Env []string
	// WorkDir is the process working directory (default: agent-profiles root).
	WorkDir string
	// Timeout bounds the run (default: defaultRunTimeout).
	Timeout time.Duration
}

// RunResult is the observable outcome of an agent CLI invocation.
type RunResult struct {
	Spans    Spans
	ExitCode int
	Output   string
	LogFile  string
}

// RootRequired asserts a single root agent.run span is present and returns it.
func (r RunResult) RootRequired(t *testing.T) Span {
	t.Helper()
	root, ok := r.Spans.Root()
	if !ok {
		t.Fatalf("expected a single root span, got spans %v\noutput:\n%s", r.Spans.Names(), r.Output)
	}
	if root.Name != RootSpanName {
		t.Fatalf("root span = %q, want %q\nspans: %v", root.Name, RootSpanName, r.Spans.Names())
	}
	return root
}

// RequireNoErrorSpans fails if any span carries an error status.
func (r RunResult) RequireNoErrorSpans(t *testing.T) {
	t.Helper()
	if errored := r.Spans.Errored(); len(errored) > 0 {
		t.Fatalf("expected zero error-status spans, got %v\noutput:\n%s", errored.Names(), r.Output)
	}
}

// RequireExit fails if the CLI exit code differs from want.
func (r RunResult) RequireExit(t *testing.T, want int) {
	t.Helper()
	if r.ExitCode != want {
		t.Fatalf("CLI exit code = %d, want %d\noutput:\n%s", r.ExitCode, want, r.Output)
	}
}

// TerminalOutcome asserts a run.terminal event was recorded and returns its
// final_state and status attributes. This is the terminal outcome every family
// SRD requires the run to reach.
func (r RunResult) TerminalOutcome(t *testing.T) (finalState, status string) {
	t.Helper()
	event, _, ok := r.Spans.FindEvent(TerminalEventName)
	if !ok {
		t.Fatalf("no %q event found; span names: %v\noutput:\n%s", TerminalEventName, r.Spans.Names(), r.Output)
	}
	finalState, _ = event.StringAttr("final_state")
	status, _ = event.StringAttr("status")
	return finalState, status
}

// RequireTerminalState asserts the run reached one of the wanted terminal
// states and returns the observed final state.
func (r RunResult) RequireTerminalState(t *testing.T, want ...string) string {
	t.Helper()
	finalState, _ := r.TerminalOutcome(t)
	for _, w := range want {
		if finalState == w {
			return finalState
		}
	}
	t.Fatalf("terminal final_state = %q, want one of %v\noutput:\n%s", finalState, want, r.Output)
	return finalState
}

// RequireToolSpans asserts an execute_tool span is present for each tool name.
func (r RunResult) RequireToolSpans(t *testing.T, tools ...string) {
	t.Helper()
	for _, tool := range tools {
		if got := r.Spans.Named("execute_tool " + tool); len(got) == 0 {
			t.Errorf("missing execute_tool span for %q; span names: %v", tool, r.Spans.Names())
		}
	}
}

// RootSpanName is the name agent-core gives the root span for a run
// (cmd/agent/main.go: telemetry.NewRoot("agent", "agent.run", ...)).
const RootSpanName = "agent.run"

// RequireCoreRoot returns the sibling agent-core checkout path, derived from the
// package's own location, or skips the test when that checkout is absent so
// plain `go test ./...` stays hermetic for docs-only checkouts. The conformance
// package builds the agent binary from that checkout.
func RequireCoreRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(filepath.Dir(ProfilesRoot()), "agent-core")
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Skipf("agent-core checkout not found at %s; skipping conformance run", root)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("resolve %q: %v", root, err)
	}
	return abs
}

// ProfilesRoot returns the agent-profiles repository root (the parent of this
// package directory), independent of the test's working directory.
func ProfilesRoot() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(thisFile))
}

// ProfilePath joins rel onto the agent-profiles root.
func ProfilePath(rel string) string {
	return filepath.Join(ProfilesRoot(), rel)
}

var (
	binaryOnce sync.Once
	binaryPath string
	binaryErr  error
)

// agentBinary builds the agent binary from coreRoot once per test process and
// returns its path. It mirrors magefiles/integration_support.go
// buildIntegrationAgent; magefiles is package main, so the builder is
// duplicated here rather than imported.
func agentBinary(t *testing.T, coreRoot string) string {
	t.Helper()
	binaryOnce.Do(func() {
		out := filepath.Join(os.TempDir(), "agent-profiles-conformance-agent")
		cmd := exec.Command("go", "build", "-tags", "production", "-o", out, "./cmd/agent")
		cmd.Dir = coreRoot
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		if err := cmd.Run(); err != nil {
			binaryErr = err
			t.Logf("build agent from %s failed:\n%s", coreRoot, buf.String())
			return
		}
		binaryPath = out
	})
	if binaryErr != nil {
		t.Fatalf("build agent binary: %v", binaryErr)
	}
	return binaryPath
}

// Run invokes the agent CLI for one family and returns the parsed trace plus
// the CLI exit state. It skips the test when the sibling agent-core checkout is absent.
func Run(t *testing.T, cfg RunConfig) RunResult {
	t.Helper()
	coreRoot := RequireCoreRoot(t)
	binary := agentBinary(t, coreRoot)

	profile := cfg.Profile
	if profile != "" && !filepath.IsAbs(profile) {
		profile = ProfilePath(profile)
	}

	logFile := filepath.Join(t.TempDir(), "trace.otel.json")
	args := []string{"--profile", profile, "--core-root", coreRoot, "--otel-log-file", logFile}
	if cfg.Directory != "" {
		args = append(args, "--directory", cfg.Directory)
	}
	if cfg.Request != "" {
		args = append(args, "--request", cfg.Request)
	}
	if cfg.Output != "" {
		args = append(args, "--output", cfg.Output)
	}
	args = append(args, cfg.Args...)

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultRunTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = cfg.WorkDir
	if cmd.Dir == "" {
		cmd.Dir = ProfilesRoot()
	}
	cmd.Env = append(os.Environ(), cfg.Env...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("agent run timed out after %s\nargs: %v\noutput:\n%s", timeout, args, out.String())
	}

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("agent run failed to start: %v\nargs: %v\noutput:\n%s", runErr, args, out.String())
		}
	}

	spans, err := ParseSpansFile(logFile)
	if err != nil {
		t.Fatalf("parse trace: %v\nexit=%d output:\n%s", err, exitCode, out.String())
	}

	return RunResult{Spans: spans, ExitCode: exitCode, Output: out.String(), LogFile: logFile}
}
