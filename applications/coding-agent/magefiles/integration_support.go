// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/catalog/catalogroot"
)

const (
	canonicalModel           = "qwen3.6:35b-mlx"
	releaseModelCallTimeout  = 30 * time.Second
	childAgentRunTimeout     = 10 * time.Minute
	outerEmergencyTimeout    = 12 * time.Minute
	processGroupCleanupGrace = 3 * time.Second
	traceDiagnosticLimit     = 32 * 1024
)

type integrationRoots struct {
	Application string
	Core        string
	Profiles    string
}

type agentRun struct {
	Output     string
	ExitCode   int
	Trace      string
	FinalState string
	LastState  string
	LastTool   string
	Phase      string
}

type agentRunOptions struct {
	Timeout      time.Duration
	CleanupGrace time.Duration
	Env          []string
}

func resolveIntegrationRoots() (integrationRoots, error) {
	app, err := os.Getwd()
	if err != nil {
		return integrationRoots{}, err
	}
	app, err = filepath.Abs(filepath.Clean(app))
	if err != nil {
		return integrationRoots{}, fmt.Errorf("coding-agent integration: resolve application root: %w", err)
	}
	catalog, err := resolveCatalogRoot("coding-agent integration", app)
	if err != nil {
		return integrationRoots{}, err
	}
	repository := filepath.Clean(filepath.Join(app, "..", ".."))
	core, err := absoluteOwnerPath(loadDemoConfigOrEmpty(app).CoreRoot, app, filepath.Join(repository, "agent-core"))
	if err != nil {
		return integrationRoots{}, fmt.Errorf("coding-agent integration: resolve core_root: %w", err)
	}
	return integrationRoots{
		Application: app,
		Core:        core,
		Profiles:    catalog,
	}, nil
}

func resolveCatalogRoot(owner, startupCWD string) (string, error) {
	// DiscoveryCandidates walks the directory's ancestors, so it needs an
	// absolute path; a relative one yields candidates that double-join with
	// the working directory inside Resolve.
	absCWD, err := filepath.Abs(filepath.Clean(startupCWD))
	if err != nil {
		return "", fmt.Errorf("%s: resolve startup working directory %q: %w", owner, startupCWD, err)
	}
	resolution, err := catalogroot.Resolve(
		owner,
		absCWD,
		loadDemoConfigOrEmpty(absCWD).CatalogRoot,
		catalogroot.DiscoveryCandidates(absCWD)...,
	)
	if err != nil {
		return "", err
	}
	return resolution.Path, nil
}

func absoluteOwnerPath(value, startupCWD, fallback string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	} else if !filepath.IsAbs(value) {
		value = filepath.Join(startupCWD, value)
	}
	return filepath.Abs(filepath.Clean(value))
}

// packageIntegrationRoots snapshots the canonical profile closure into a
// disposable package. Live stages execute only from this returned root, so
// later checkout mutations cannot alter an in-flight proof.
func packageIntegrationRoots(roots integrationRoots) (integrationRoots, func(), error) {
	manifest, err := readApplicationProfileManifestWithCatalog(
		filepath.Join(roots.Application, filepath.FromSlash(profileManifestPath)),
		roots.Profiles,
	)
	if err != nil {
		return integrationRoots{}, nil, err
	}
	source, err := inspectPackageSource(roots.Profiles, manifest.Catalog.CompatibleRelease)
	if err != nil {
		return integrationRoots{}, nil, err
	}
	runDir, err := os.MkdirTemp("", "coding-loop-profile-closure-*")
	if err != nil {
		return integrationRoots{}, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(runDir) }
	packageRoot := filepath.Join(runDir, "profiles")
	if _, err := assembleProfileClosure(manifest, roots.Profiles, packageRoot, source); err != nil {
		cleanup()
		return integrationRoots{}, nil, fmt.Errorf("package integration profile closure: %w", err)
	}
	roots.Profiles = packageRoot
	return roots, cleanup, nil
}

// liveSkipReason keeps the application targets optional while release evidence
// supplies its own deterministic Ollama-compatible model boundary.
func liveSkipReason(roots integrationRoots, extraBinaries ...string) string {
	return baseIntegrationSkipReason(roots, extraBinaries...)
}

func baseIntegrationSkipReason(roots integrationRoots, extraBinaries ...string) string {
	for _, requirement := range []struct {
		path  string
		label string
	}{
		{filepath.Join(roots.Core, "go.mod"), "agent-core checkout"},
		{filepath.Join(roots.Profiles, "agents", "executor", "profile.yaml"), "canonical executor profile"},
		{filepath.Join(roots.Profiles, "agents", "planner", "profile.yaml"), "canonical planner profile"},
		{filepath.Join(roots.Profiles, "agents", "critic", "profile.yaml"), "canonical critic profile"},
		{filepath.Join(roots.Profiles, "agents", "critic", "profile-workspace.yaml"), "canonical changed-workspace critic profile"},
	} {
		if info, err := os.Stat(requirement.path); err != nil || info.IsDir() {
			return fmt.Sprintf("%s not found at %s", requirement.label, requirement.path)
		}
	}
	for _, binary := range append([]string{"go", "git"}, extraBinaries...) {
		if _, err := exec.LookPath(binary); err != nil {
			return fmt.Sprintf("%s not found on PATH", binary)
		}
	}
	return ""
}

func buildAgent(coreRoot string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "coding-agent-binary-*")
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

func copyTree(source, destination string) error {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	return fs.WalkDir(os.DirFS(source), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}
		target := filepath.Join(destination, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(os.DirFS(source), path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

func freshWorkspace(appRoot string) (string, func(), error) {
	runDir, err := os.MkdirTemp("", "coding-loop-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(runDir) }
	source := filepath.Join(appRoot, "testdata", "integration", "coding-loop", "workspace")
	workspace := filepath.Join(runDir, "workspace")
	if err := copyTree(source, workspace); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("copy coding-loop workspace: %w", err)
	}
	return workspace, cleanup, nil
}

func runBuiltAgent(binary, profilesRoot, coreRoot, profile, workspace string, extraArgs ...string) (agentRun, error) {
	return runBuiltAgentWithOptions(
		binary, profilesRoot, coreRoot, profile, workspace, agentRunOptions{}, extraArgs...)
}

func runBuiltAgentWithOptions(
	binary, profilesRoot, coreRoot, profile, workspace string,
	options agentRunOptions,
	extraArgs ...string,
) (agentRun, error) {
	trace := filepath.Join(filepath.Dir(workspace), filepath.Base(profile)+".trace.ndjson")
	args := []string{
		"--profile", filepath.Join(profilesRoot, filepath.FromSlash(profile)),
		"--directory", workspace,
		"--core-root", coreRoot,
		"--otel-log-file", trace,
	}
	args = append(args, extraArgs...)
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = outerEmergencyTimeout
	}
	cleanupGrace := options.CleanupGrace
	if cleanupGrace <= 0 {
		cleanupGrace = processGroupCleanupGrace
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.Command(binary, args...)
	cmd.Dir = profilesRoot
	cmd.Env = append(os.Environ(), options.Env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = cleanupGrace
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		return agentRun{}, fmt.Errorf("start agent: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	timedOut := false
	select {
	case err = <-done:
	case <-ctx.Done():
		timedOut = true
		err = terminateProcessGroup(cmd.Process.Pid, done, cleanupGrace)
	}
	run := agentRun{Output: output.String()}
	if data, readErr := os.ReadFile(trace); readErr == nil {
		run.FinalState, run.LastState, run.LastTool = traceRunPosition(string(data))
		if len(data) > traceDiagnosticLimit {
			data = data[len(data)-traceDiagnosticLimit:]
		}
		run.Trace = string(data)
	}
	run.Phase = inferRunPhase(profile, run.LastState, run.LastTool, run.Output)
	if timedOut {
		run.ExitCode = -1
		return run, fmt.Errorf(
			"%s exceeded outer emergency deadline %s during %s (last state=%q, last tool=%q): %w\nstdout/stderr:\n%s\ntrace tail:\n%s",
			profile, timeout, run.Phase, run.LastState, run.LastTool, ctx.Err(), run.Output, run.Trace)
	}
	if err == nil {
		return run, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		run.ExitCode = exitErr.ExitCode()
		return run, nil
	}
	return run, fmt.Errorf("wait for agent: %w\nstdout/stderr:\n%s\ntrace tail:\n%s", err, run.Output, run.Trace)
}

func terminateProcessGroup(pid int, done <-chan error, grace time.Duration) error {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	timer := time.NewTimer(grace)
	defer timer.Stop()
	var waitErr error
	reaped := false
	select {
	case waitErr = <-done:
		reaped = true
	case <-timer.C:
	}
	// The group can retain a child after its leader exits, so always send the
	// final group kill after grace rather than treating leader exit as cleanup.
	if reaped {
		<-timer.C
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	if reaped {
		return waitErr
	}
	return <-done
}

func traceFinalState(trace string) string {
	final, _, _ := traceRunPosition(trace)
	return final
}

var traceStringAttribute = regexp.MustCompile(
	`"Key":"([^"]+)","Value":\{"Type":"STRING","Value":"([^"]*)"\}`,
)

func traceRunPosition(trace string) (finalState, lastState, lastTool string) {
	for _, match := range traceStringAttribute.FindAllStringSubmatch(trace, -1) {
		key, value := match[1], match[2]
		switch key {
		case "run.final_state":
			finalState = value
			lastState = value
		case "final_state":
			if finalState == "" {
				finalState = value
			}
			lastState = value
		case "to_state", "state":
			lastState = value
		case "gen_ai.tool.name", "tool.name":
			lastTool = value
		}
	}
	if finalState != "" {
		return finalState, lastState, lastTool
	}
	for _, state := range []string{"Succeeded", "Failed", "BudgetExceeded", "Completed", "Stalled", "Paused", "Done", "Rejected"} {
		if strings.Contains(trace, `"Key":"run.final_state","Value":{"Type":"STRING","Value":"`+state+`"}`) {
			return state, state, lastTool
		}
	}
	return "", lastState, lastTool
}

func inferRunPhase(profile, state, tool, output string) string {
	combined := strings.ToLower(state + " " + tool + " " + output)
	switch {
	case strings.Contains(combined, "invokingexecutor") ||
		strings.Contains(combined, "self_invoke") ||
		strings.Contains(combined, "invoke_executor"):
		return "child executor"
	case strings.Contains(profile, "planner") &&
		(strings.Contains(combined, "planinvoking") || strings.Contains(combined, "invoke_llm")):
		return "planner model"
	case strings.Contains(profile, "executor") &&
		(strings.Contains(combined, "composing") || strings.Contains(combined, "invoke_llm")):
		return "executor model"
	case strings.Contains(profile, "critic"):
		return "critic oracle"
	default:
		return "agent run"
	}
}

func requireSuccessfulExecutor(workspace string, run agentRun) error {
	if run.ExitCode != 0 {
		return fmt.Errorf("executor exited %d:\n%s", run.ExitCode, run.Output)
	}
	if !strings.Contains(strings.ToLower(run.Output), "terminal state: succeeded") {
		return fmt.Errorf("executor did not report Succeeded:\n%s", run.Output)
	}
	if err := requireGreetingAndTests(workspace); err != nil {
		return fmt.Errorf("executor workspace validation failed: %w", err)
	}
	return nil
}
