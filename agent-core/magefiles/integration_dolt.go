// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Dolt proves checkpoint persistence and command-state rehydration against a real
// Dolt SQL server. The test harness launches `dolt sql-server` from a prebuilt
// dolt binary for the duration of the run (no Docker, no manual setup), so this
// target only needs a dolt binary on PATH or a dolt_bin setting in demo.yaml.
func (Integration) Dolt() error {
	beginUC("dolt")
	root, err := doltIntegrationRoot()
	if err != nil {
		return fmt.Errorf("dolt: %w", err)
	}
	config, err := loadAgentCoreDemoConfig(root)
	if err != nil {
		return fmt.Errorf("dolt: load demo config: %w", err)
	}
	doltBin, err := resolveDoltBinary(config.DoltBin, exec.LookPath)
	if err != nil {
		if configured := strings.TrimSpace(config.DoltBin); configured != "" {
			return skipUC("dolt", fmt.Sprintf("configured dolt_bin %q is unavailable", configured))
		}
		return skipUC("dolt", "no dolt binary on PATH; install dolt (https://docs.dolthub.com/introduction/installation) or set dolt_bin in demo.yaml")
	}

	cmd := exec.Command(
		"go", "test", "./cmd/agent",
		"-run", "TestDoltCheckpoint|TestDoltCommandStateRehydratesThroughRealAdapter|TestRequestSignalSource_SuspendResume",
		"-count=1",
		"-args", "-dolt-bin", doltBin,
	)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dolt: live persistence proof failed: %w", err)
	}
	fmt.Println("dolt: PASS - checkpoints survive adapter and process boundaries")
	return nil
}

// DoltWord proves configured provision, query, and write boundary words against
// a real throwaway Dolt SQL server, including commit history and checkpoint
// database separation.
func (Integration) DoltWord() error {
	beginUC("doltWord")
	root, err := doltIntegrationRoot()
	if err != nil {
		return fmt.Errorf("doltWord: %w", err)
	}
	config, err := loadAgentCoreDemoConfig(root)
	if err != nil {
		return fmt.Errorf("doltWord: load demo config: %w", err)
	}
	doltBin, err := resolveDoltBinary(config.DoltBin, exec.LookPath)
	if err != nil {
		if configured := strings.TrimSpace(config.DoltBin); configured != "" {
			return skipUC("doltWord", fmt.Sprintf("configured dolt_bin %q is unavailable", configured))
		}
		return skipUC("doltWord", "no dolt binary on PATH; install dolt (https://docs.dolthub.com/introduction/installation) or set dolt_bin in demo.yaml")
	}

	cmd := exec.Command(
		"go", "test", "./cmd/agent",
		"-run", "^TestDoltWordIntegration$",
		"-count=1",
		"-args", "-dolt-bin", doltBin,
	)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("doltWord: configured word proof failed: %w", err)
	}
	fmt.Println("doltWord: PASS - configured words provision, bind, commit, query, and isolate")
	return nil
}

func doltIntegrationRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve module root: %w", err)
	}
	for _, candidate := range []string{current, filepath.Dir(current)} {
		if info, statErr := os.Stat(filepath.Join(candidate, "cmd", "agent")); statErr == nil && info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("resolve module root from %q", current)
}

func resolveDoltBinary(configured string, lookPath func(string) (string, error)) (string, error) {
	name := strings.TrimSpace(configured)
	if name == "" {
		name = "dolt"
	}
	return lookPath(name)
}
