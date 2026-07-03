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
