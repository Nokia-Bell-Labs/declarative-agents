// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	agentCoreRootEnv     = "AGENT_CORE_ROOT"
	agentProfilesRootEnv = "AGENT_PROFILES_ROOT"
	ollamaProbeURLEnv    = "CODING_AGENT_OLLAMA_URL"

	canonicalOllamaURL = "http://localhost:11434"
	canonicalModel     = "qwen3.6:35b-mlx"
	liveStageTimeout   = 15 * time.Minute
)

var prerequisiteHTTPClient = &http.Client{Timeout: 3 * time.Second}

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
}

func resolveIntegrationRoots() (integrationRoots, error) {
	app, err := os.Getwd()
	if err != nil {
		return integrationRoots{}, err
	}
	repository := filepath.Clean(filepath.Join(app, "..", ".."))
	return integrationRoots{
		Application: app,
		Core:        envOrDefault(agentCoreRootEnv, filepath.Join(repository, "agent-core")),
		Profiles:    envOrDefault(agentProfilesRootEnv, filepath.Join(repository, "agent-profiles")),
	}, nil
}

// packageIntegrationRoots snapshots the canonical profile closure into a
// disposable package. Live stages execute only from this returned root, so
// later checkout mutations cannot alter an in-flight proof.
func packageIntegrationRoots(roots integrationRoots) (integrationRoots, func(), error) {
	manifest, err := readApplicationProfileManifest(filepath.Join(roots.Application, filepath.FromSlash(profileManifestPath)))
	if err != nil {
		return integrationRoots{}, nil, err
	}
	source, err := inspectPackageSource(roots.Profiles, manifest.AgentProfiles.CompatibleRelease)
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

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

// liveSkipReason keeps the application targets optional. It checks the exact
// model selected by the canonical planner and executor profiles, rather than
// treating any reachable Ollama installation as sufficient.
func liveSkipReason(roots integrationRoots, extraBinaries ...string) string {
	if probe := strings.TrimSpace(os.Getenv(ollamaProbeURLEnv)); probe != "" && probe != canonicalOllamaURL {
		return fmt.Sprintf("%s=%s does not match canonical profile endpoint %s", ollamaProbeURLEnv, probe, canonicalOllamaURL)
	}
	if reason := baseIntegrationSkipReason(roots, extraBinaries...); reason != "" {
		return reason
	}
	return ollamaSkipReason(prerequisiteHTTPClient, canonicalOllamaURL, canonicalModel)
}

func baseIntegrationSkipReason(roots integrationRoots, extraBinaries ...string) string {
	for _, requirement := range []struct {
		path  string
		label string
	}{
		{filepath.Join(roots.Core, "go.mod"), "agent-core checkout"},
		{filepath.Join(roots.Profiles, "go.mod"), "agent-profiles checkout"},
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

func ollamaSkipReason(client *http.Client, baseURL, model string) string {
	resp, err := client.Get(strings.TrimRight(baseURL, "/") + "/api/tags")
	if err != nil {
		return fmt.Sprintf("Ollama not reachable at %s: %v", baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("Ollama tags endpoint returned %d", resp.StatusCode)
	}
	var payload struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Sprintf("decode Ollama tags: %v", err)
	}
	for _, installed := range payload.Models {
		if installed.Name == model || installed.Model == model {
			return ""
		}
	}
	return fmt.Sprintf("Ollama model %q not pulled", model)
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
	trace := filepath.Join(filepath.Dir(workspace), filepath.Base(profile)+".trace.ndjson")
	args := []string{
		"--profile", filepath.Join(profilesRoot, filepath.FromSlash(profile)),
		"--directory", workspace,
		"--core-root", coreRoot,
		"--otel-log-file", trace,
	}
	args = append(args, extraArgs...)
	ctx, cancel := context.WithTimeout(context.Background(), liveStageTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = profilesRoot
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	run := agentRun{Output: output.String()}
	if data, readErr := os.ReadFile(trace); readErr == nil {
		run.FinalState = traceFinalState(string(data))
		const traceDiagnosticLimit = 32 * 1024
		if len(data) > traceDiagnosticLimit {
			data = data[len(data)-traceDiagnosticLimit:]
		}
		run.Trace = string(data)
	}
	if ctx.Err() != nil {
		return run, fmt.Errorf("%s exceeded live stage timeout %s: %w", profile, liveStageTimeout, ctx.Err())
	}
	if err == nil {
		return run, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		run.ExitCode = exitErr.ExitCode()
		return run, nil
	}
	return run, fmt.Errorf("start agent: %w", err)
}

func traceFinalState(trace string) string {
	for _, state := range []string{"Succeeded", "Failed", "BudgetExceeded", "Completed", "Stalled", "Paused", "Done", "Rejected"} {
		if strings.Contains(trace, `"Key":"run.final_state","Value":{"Type":"STRING","Value":"`+state+`"}`) {
			return state
		}
	}
	return ""
}

func requireSuccessfulExecutor(workspace string, run agentRun) error {
	if run.ExitCode != 0 {
		return fmt.Errorf("executor exited %d:\n%s", run.ExitCode, run.Output)
	}
	if !strings.Contains(strings.ToLower(run.Output), "terminal state: succeeded") {
		return fmt.Errorf("executor did not report Succeeded:\n%s", run.Output)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "greet.go"))
	if err != nil {
		return fmt.Errorf("read executor result: %w", err)
	}
	if !strings.Contains(string(data), `return "Hello, " + name + "!"`) {
		return fmt.Errorf("executor did not implement the greeting:\n%s", data)
	}
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = workspace
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("workspace validation failed: %w\n%s", err, output)
	}
	return nil
}
