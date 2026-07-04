// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type statFunc func(string) (os.FileInfo, error)
type auditRunner func(string) error

// Audit runs mage audit in each sub-module.
func Audit() error {
	return auditSubModules(subModules, os.Stat, runMageAudit)
}

func auditSubModules(modules []string, stat statFunc, run auditRunner) error {
	for _, mod := range modules {
		mageDir := filepath.Join(mod, "magefiles")
		if _, err := stat(mageDir); err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("skipping %s (no magefiles/)\n", mod)
				continue
			}
			return fmt.Errorf("stat %s: %w", mageDir, err)
		}
		fmt.Printf("=== %s audit ===\n", mod)
		if err := run(mod); err != nil {
			return fmt.Errorf("audit in %s: %w", mod, err)
		}
	}
	return nil
}

func runMageAudit(dir string) error {
	cmd := exec.Command("mage", "audit")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
