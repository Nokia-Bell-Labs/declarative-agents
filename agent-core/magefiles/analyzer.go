// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Analyzer groups the analyzer UI dev and production build targets.
type Analyzer mg.Namespace

// Install runs npm install in the analyzer UI directory.
func (Analyzer) Install() error {
	return runInDir("cmd/analyzer/ui", "npm", "install")
}

// Dev runs the analyzer in development mode.
// Starts the Vite dev server for frontend hot-reloading.
func (Analyzer) Dev() error {
	mg.Deps(Analyzer.Install)
	return runInDir("cmd/analyzer/ui", "npm", "run", "dev")
}

// Build builds the production analyzer binary with embedded frontend assets.
func (Analyzer) Build() error {
	mg.Deps(Analyzer.Install)

	fmt.Println("building frontend...")
	if err := runInDir("cmd/analyzer/ui", "npm", "run", "build"); err != nil {
		return fmt.Errorf("frontend build: %w", err)
	}

	fmt.Println("building analyzer binary...")
	return sh.Run("go", "build", "-tags", "production", "-o", "bin/analyzer", "./cmd/analyzer")
}

// Serve builds and runs the production analyzer binary.
func (Analyzer) Serve() error {
	mg.Deps(Analyzer.Build)
	return sh.Run("./bin/analyzer", "serve",
		"--data", "eval-results",
		"--configs", "configs",
		"--profiles-dir", "pkg/llm/profiles",
	)
}

// runInDir runs a command with its working directory set to dir.
func runInDir(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
