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
// Lint is not a release gate today, which is a temporary state rather than a
// policy: the decision is to gate it. It cannot be gated while the policy still
// reports findings, and running it for the first time surfaced twelve forbidigo
// violations that each need a judgment about refactoring versus a declared
// exception. GH-1481 resolves those; adding Lint to the release recipe is the
// last step of GH-1479 and happens once it reports clean.
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
