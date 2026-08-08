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
// including the standalone Mage modules. It preflights the binary so a version
// that cannot read the config schema fails with installation guidance rather than
// a schema error from inside the first module's run (GH-1479).
//
// Lint is a release gate, wired into the recipe in Tag. It could not be one
// before: the policy had never actually run, and its first run reported twelve
// forbidigo findings, which GH-1481 resolved by refactoring or annotating each
// site. The go-style constitution lists every annotated site (GH-1479).
func Lint() error {
	if err := checkGolangciLint(); err != nil {
		return err
	}
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
