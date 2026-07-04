// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
)

// Integration contains use-case integration tests invoked as mage integration:ucXXX.
type Integration mg.Namespace

// All runs every integration test and prints a summary.
func (i Integration) All() error {
	tests := []struct {
		name string
		fn   func() error
	}{
		{"uc001", i.Uc001},
		{"uc002", i.Uc002},
		{"uc003", i.Uc003},
		{"uc004", i.Uc004},
		{"uc005", i.Uc005},
		{"uc008", i.Uc008},
	}

	var passed, failed, skipped int
	results := make([]string, 0, len(tests))

	for _, t := range tests {
		fmt.Printf("\n=== %s ===\n", t.name)
		err := t.fn()
		switch {
		case err != nil:
			failed++
			results = append(results, fmt.Sprintf("  FAIL  %s  %v", t.name, err))
		case wasSkipped(t.name):
			skipped++
			results = append(results, fmt.Sprintf("  SKIP  %s", t.name))
		default:
			passed++
			results = append(results, fmt.Sprintf("  PASS  %s", t.name))
		}
	}

	fmt.Printf("\n%s\n", strings.Repeat("─", 40))
	for _, r := range results {
		fmt.Println(r)
	}
	fmt.Printf("%s\n", strings.Repeat("─", 40))
	fmt.Printf("Total: %d passed, %d failed, %d skipped\n", passed, failed, skipped)

	if failed > 0 {
		return fmt.Errorf("%d integration test(s) failed", failed)
	}
	return nil
}

var skippedUCs = map[string]bool{}

// Uc001 runs rel01.0-uc001: Generator agent solves a Go coding task with Qwen 3.6.
func (Integration) Uc001() error {
	if err := requireOllama(); err != nil {
		return skipUC("uc001", err.Error())
	}
	if err := requireModel(qwen35b); err != nil {
		return skipUC("uc001", err.Error())
	}

	binary, err := buildIfNeeded()
	if err != nil {
		return err
	}

	rootDir, err := os.Getwd()
	if err != nil {
		return err
	}
	profileRoot, err := resolveAgentProfilesRoot(rootDir)
	if err != nil {
		return err
	}
	profilesRepoRoot, err := resolveAgentProfilesRepoRoot(rootDir)
	if err != nil {
		return err
	}

	workDir, cleanup, err := tempWorkspace(filepath.Join(profilesRepoRoot, generatorSample))
	if err != nil {
		return fmt.Errorf("uc001: prepare workspace: %w", err)
	}
	defer cleanup()

	fmt.Printf("uc001: workspace at %s\n", workDir)

	args := uc001AgentArgs(profileRoot, rootDir, workDir)

	if err := runAgent(binary, args); err != nil {
		return fmt.Errorf("uc001: agent failed: %w", err)
	}

	fmt.Println("uc001: PASS — generator reached Succeeded with Qwen 3.6")
	return nil
}

func uc001AgentArgs(profileRoot, coreRoot, workDir string) []string {
	return []string{
		"--profile", agentProfilePath(profileRoot, "generator"),
		"--directory", workDir,
		"--core-root", coreRoot,
	}
}

// Uc002 runs rel01.0-uc002: Evaluator benchmarks generator across models.
func (Integration) Uc002() error {
	if err := requireOllama(); err != nil {
		return skipUC("uc002", err.Error())
	}
	if err := requireModel(qwen35b); err != nil {
		return skipUC("uc002", err.Error())
	}
	if err := requireModel(qwen27b); err != nil {
		return skipUC("uc002", err.Error())
	}

	binary, err := buildIfNeeded()
	if err != nil {
		return err
	}

	rootDir, err := os.Getwd()
	if err != nil {
		return err
	}
	profileRoot, err := resolveAgentProfilesRoot(rootDir)
	if err != nil {
		return err
	}
	profilesRepoRoot, err := resolveAgentProfilesRepoRoot(rootDir)
	if err != nil {
		return err
	}

	outputDir, err := os.MkdirTemp("", "eval-results-*")
	if err != nil {
		return fmt.Errorf("uc002: create output dir: %w", err)
	}
	defer os.RemoveAll(outputDir)

	fmt.Printf("uc002: output at %s\n", outputDir)

	binAbs, err := filepath.Abs(binDir)
	if err != nil {
		return err
	}
	os.Setenv("PATH", binAbs+":"+os.Getenv("PATH"))

	args := uc002AgentArgs(profileRoot, rootDir, filepath.Join(profilesRepoRoot, evaluatorSuite), outputDir)

	if err := runAgentInDir(binary, args, profilesRepoRoot); err != nil {
		return fmt.Errorf("uc002: evaluator failed: %w", err)
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil || len(entries) == 0 {
		return fmt.Errorf("uc002: no results in output dir %s", outputDir)
	}

	fmt.Printf("uc002: PASS — evaluator completed with %d result entries\n", len(entries))
	return nil
}

func uc002AgentArgs(profileRoot, coreRoot, requestPath, outputDir string) []string {
	return []string{
		"--profile", agentProfilePath(profileRoot, "evaluator"),
		"--request", requestPath,
		"--output", outputDir,
		"--core-root", coreRoot,
	}
}

// Uc003 runs rel01.0-uc003: Bench serves web UI for evaluation result exploration.
func (Integration) Uc003() error {
	return skipUC("uc003", "bench visualization — requires eval-results directory and a free port")
}

const (
	qwen35b         = "qwen3.6:35b-mlx"
	qwen27b         = "qwen3.6:27b-mlx"
	generatorSample = "testdata/integration/uc001-generator-coding"
	evaluatorSuite  = "testdata/integration/uc002-evaluator-benchmark/suite.yaml"
)

func tempWorkspace(sampleDir string) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "integration-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	cmd := exec.Command("cp", "-a", sampleDir+"/.", tmpDir)
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("copy sample %s: %w", sampleDir, err)
	}

	gitInit := exec.Command("git", "init")
	gitInit.Dir = tmpDir
	if err := gitInit.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("git init: %w", err)
	}

	gitAdd := exec.Command("git", "add", "-A")
	gitAdd.Dir = tmpDir
	if err := gitAdd.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("git add: %w", err)
	}

	gitCommit := exec.Command("git", "commit", "-m", "initial")
	gitCommit.Dir = tmpDir
	gitCommit.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
	)
	if err := gitCommit.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("git commit: %w", err)
	}

	return tmpDir, cleanup, nil
}

func skipUC(id, reason string) error {
	skippedUCs[id] = true
	fmt.Printf("SKIP %s: %s\n", id, reason)
	return nil
}

func wasSkipped(id string) bool {
	return skippedUCs[id]
}

func buildIfNeeded() (string, error) {
	binary := filepath.Join(binDir, "agent")
	info, err := os.Stat(binary)
	if err != nil || time.Since(info.ModTime()) > 24*time.Hour {
		fmt.Println("building agent binary...")
		if err := Build(); err != nil {
			return "", fmt.Errorf("build agent: %w", err)
		}
	}
	abs, err := filepath.Abs(binary)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func requireOllama() error {
	resp, err := http.Get("http://localhost:11434/api/version")
	if err != nil {
		return fmt.Errorf("ollama not reachable at localhost:11434: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}
	return nil
}

func requireModel(model string) error {
	resp, err := http.Get("http://localhost:11434/api/tags")
	if err != nil {
		return fmt.Errorf("ollama not reachable: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode ollama models: %w", err)
	}

	var names []string
	for _, m := range result.Models {
		if m.Name == model {
			return nil
		}
		names = append(names, m.Name)
	}
	return fmt.Errorf("model %q not found in ollama; available: %s", model, strings.Join(names, ", "))
}

func runAgent(binary string, args []string) error {
	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("running: %s %s\n", binary, strings.Join(args, " "))
	return cmd.Run()
}

func runAgentInDir(binary string, args []string, dir string) error {
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("running in %s: %s %s\n", dir, binary, strings.Join(args, " "))
	return cmd.Run()
}
