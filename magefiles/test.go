// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/magefile/mage/mg"
)

// Test contains release test gates invoked as mage test:*.
type Test mg.Namespace

type unitTestRunner func(string) error

// Unit runs unit tests for sub-modules with a Go module.
func (Test) Unit() error {
	return testUnitSubModules(subModules, os.Stat, runGoUnitTests)
}

func testUnitSubModules(modules []string, stat statFunc, run unitTestRunner) error {
	for _, mod := range modules {
		goMod := filepath.Join(mod, "go.mod")
		if _, err := stat(goMod); err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("skipping %s (no go.mod)\n", mod)
				continue
			}
			return fmt.Errorf("stat %s: %w", goMod, err)
		}
		fmt.Printf("=== %s unit tests ===\n", mod)
		if err := run(mod); err != nil {
			return fmt.Errorf("unit tests in %s: %w", mod, err)
		}
	}
	return nil
}

func runGoUnitTests(dir string) error {
	cmd := exec.Command("go", "test", "-short", "./...")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
