// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

var subModules = []string{
	"agent-core",
	"agent-profiles",
	"design-patterns",
}

type buildRunner func(string) error

// All runs the default mage target in each sub-module (default target).
func All() error {
	for _, mod := range subModules {
		mageDir := filepath.Join(mod, "magefiles")
		if _, err := os.Stat(mageDir); os.IsNotExist(err) {
			fmt.Printf("skipping %s (no magefiles/)\n", mod)
			continue
		}
		fmt.Printf("=== %s ===\n", mod)
		cmd := exec.Command("mage")
		cmd.Dir = mod
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("mage in %s: %w", mod, err)
		}
	}
	return nil
}

// Build runs mage build in each sub-module.
func Build() error {
	return buildSubModules(subModules, os.Stat, runMageBuild)
}

func buildSubModules(modules []string, stat statFunc, run buildRunner) error {
	for _, mod := range modules {
		mageDir := filepath.Join(mod, "magefiles")
		if _, err := stat(mageDir); err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("skipping %s (no magefiles/)\n", mod)
				continue
			}
			return fmt.Errorf("stat %s: %w", mageDir, err)
		}
		fmt.Printf("=== %s build ===\n", mod)
		if err := run(mod); err != nil {
			return fmt.Errorf("build in %s: %w", mod, err)
		}
	}
	return nil
}

func runMageBuild(dir string) error {
	cmd := exec.Command("mage", "build")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
