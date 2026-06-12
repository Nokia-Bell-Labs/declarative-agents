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
