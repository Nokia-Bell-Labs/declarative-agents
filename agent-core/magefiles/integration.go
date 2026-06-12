// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

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

// Uc001 runs rel01.0-uc001: Generator agent solves Go coding tasks using YAML config.
func (Integration) Uc001() error {
	return skipUC("uc001", "generator coding — requires a sample coding workspace and Ollama with a pulled model")
}

// Uc002 runs rel01.0-uc002: Evaluator runs generator across models and collects results.
func (Integration) Uc002() error {
	return skipUC("uc002", "evaluator benchmark — requires a suite YAML, sample workspaces, and Ollama")
}

// Uc003 runs rel01.0-uc003: Bench serves web UI for evaluation result exploration.
func (Integration) Uc003() error {
	return skipUC("uc003", "bench visualization — requires eval-results directory and a free port")
}

const (
	qwenModel        = "qwen3.6"
	qwen8b           = "qwen3.6:8b-mlx"
	qwen35b          = "qwen3.6:35b-mlx"
	generatorSample  = "testdata/integration/uc004-qwen-generator"
	evaluatorSuite   = "testdata/integration/uc005-qwen-evaluator/suite.yaml"
)

// Uc004 runs rel01.0-uc004: Generator agent solves a Go coding task with Qwen 3.6.
func (Integration) Uc004() error {
	if err := requireOllama(); err != nil {
		return skipUC("uc004", err.Error())
	}
	if err := requireModel(qwenModel); err != nil {
		return skipUC("uc004", err.Error())
	}

	binary, err := buildIfNeeded()
	if err != nil {
		return err
	}

	workDir, cleanup, err := tempWorkspace(generatorSample)
	if err != nil {
		return fmt.Errorf("uc004: prepare workspace: %w", err)
	}
	defer cleanup()

	fmt.Printf("uc004: workspace at %s\n", workDir)

	rootDir, err := os.Getwd()
	if err != nil {
		return err
	}

	args := []string{
		"--machine", filepath.Join(rootDir, "agents/generator/machine.yaml"),
		"--tools-declaration", filepath.Join(rootDir, "tools/builtin.yaml"),
		"--tools-declaration", filepath.Join(rootDir, "tools/exec.yaml"),
		"--tools-declaration", filepath.Join(rootDir, "agents/generator/llm/default.yaml"),
		"--tools", filepath.Join(rootDir, "agents/generator/tools.yaml"),
		"--directory", workDir,
	}

	if err := runAgent(binary, args); err != nil {
		return fmt.Errorf("uc004: agent failed: %w", err)
	}

	fmt.Println("uc004: PASS — generator reached Succeeded with Qwen 3.6")
	return nil
}

// Uc005 runs rel01.0-uc005: Evaluator benchmarks Qwen 3.6 across parameter sizes.
func (Integration) Uc005() error {
	if err := requireOllama(); err != nil {
		return skipUC("uc005", err.Error())
	}
	if err := requireModel(qwen8b); err != nil {
		return skipUC("uc005", err.Error())
	}
	if err := requireModel(qwen35b); err != nil {
		return skipUC("uc005", err.Error())
	}

	binary, err := buildIfNeeded()
	if err != nil {
		return err
	}

	rootDir, err := os.Getwd()
	if err != nil {
		return err
	}

	outputDir, err := os.MkdirTemp("", "eval-results-*")
	if err != nil {
		return fmt.Errorf("uc005: create output dir: %w", err)
	}
	defer os.RemoveAll(outputDir)

	fmt.Printf("uc005: output at %s\n", outputDir)

	args := []string{
		"--machine", filepath.Join(rootDir, "agents/evaluator/machine.yaml"),
		"--tools", filepath.Join(rootDir, "agents/evaluator/tools.yaml"),
		"--tools-declaration", filepath.Join(rootDir, "tools/builtin.yaml"),
		"--tools-declaration", filepath.Join(rootDir, "agents/evaluator/builtin.yaml"),
		"--input", filepath.Join(rootDir, evaluatorSuite),
		"--output", outputDir,
	}

	if err := runAgent(binary, args); err != nil {
		return fmt.Errorf("uc005: evaluator failed: %w", err)
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil || len(entries) == 0 {
		return fmt.Errorf("uc005: no results in output dir %s", outputDir)
	}

	fmt.Printf("uc005: PASS — evaluator completed with %d result entries\n", len(entries))
	return nil
}

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
	fmt.Printf("SKIP %s: %s (not yet implemented)\n", id, reason)
	return nil
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

	base := strings.Split(model, ":")[0]
	for _, m := range result.Models {
		if m.Name == model || strings.Split(m.Name, ":")[0] == base {
			return nil
		}
	}
	return fmt.Errorf("model %q not found in ollama (pulled models: %d)", model, len(result.Models))
}

func runAgent(binary string, args []string) error {
	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("running: %s %s\n", binary, strings.Join(args, " "))
	return cmd.Run()
}
