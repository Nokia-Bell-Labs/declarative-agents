// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type cleanRunner func(string) error

// Clean runs mage clean in each sub-module.
func Clean() error {
	return cleanSubModules(subModules, os.Stat, runMageClean)
}

func cleanSubModules(modules []string, stat statFunc, run cleanRunner) error {
	for _, mod := range modules {
		mageDir := filepath.Join(mod, "magefiles")
		if _, err := stat(mageDir); err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("skipping %s (no magefiles/)\n", mod)
				continue
			}
			return fmt.Errorf("stat %s: %w", mageDir, err)
		}
		fmt.Printf("=== %s clean ===\n", mod)
		if err := run(mod); err != nil {
			return fmt.Errorf("clean in %s: %w", mod, err)
		}
	}
	return nil
}

func runMageClean(dir string) error {
	cmd := exec.Command("mage", "clean")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
