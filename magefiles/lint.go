// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
)

var lintModuleDirs = []string{
	"magefiles",
	"agent-core",
	"agent-core/magefiles",
	"applications/catalog",
	"applications/chatbot-mesh",
	"applications/coding-agent",
	"applications/agent-architecture",
	"applications/prose-editor",
	"design-patterns/magefiles",
}

type lintRunner func(string) error

// Lint runs the pinned golangci-lint v2 policy in every non-fixture Go module,
// including the standalone Mage modules.
func Lint() error {
	return lintSubModules(lintModuleDirs, runGolangciLint)
}

func lintSubModules(modules []string, run lintRunner) error {
	for _, module := range modules {
		fmt.Printf("=== %s lint ===\n", module)
		if err := run(module); err != nil {
			return fmt.Errorf("lint in %s: %w", module, err)
		}
	}
	return nil
}

func runGolangciLint(dir string) error {
	cmd := exec.Command("golangci-lint", "run", "./...")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
