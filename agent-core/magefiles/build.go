// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/magefile/mage/sh"
)

const binDir = "bin"

// Build compiles all cmd/ binaries into bin/.
// Packages with a ui/ subdirectory get their frontend built first and
// are compiled with -tags production to embed the assets.
func Build() error {
	pkgs, err := discoverCmdPackages()
	if err != nil {
		return err
	}
	if len(pkgs) == 0 {
		fmt.Println("no cmd/ packages found, skipping build")
		return nil
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", binDir, err)
	}
	for _, pkg := range pkgs {
		name := filepath.Base(pkg)
		out := filepath.Join(binDir, name)
		uiDir := filepath.Join("cmd", name, "ui")
		args := []string{"build", "-o", out}

		if hasUI(uiDir) {
			fmt.Printf("installing frontend deps for %s\n", name)
			if err := runInDir(uiDir, "npm", "install"); err != nil {
				return fmt.Errorf("%s npm install: %w", name, err)
			}
			fmt.Printf("building frontend for %s\n", name)
			if err := runInDir(uiDir, "npm", "run", "build"); err != nil {
				return fmt.Errorf("%s frontend build: %w", name, err)
			}
			args = append(args, "-tags", "production")
		}

		args = append(args, pkg)
		fmt.Printf("building %s → %s\n", pkg, out)
		if err := sh.Run("go", args...); err != nil {
			return fmt.Errorf("build %s: %w", pkg, err)
		}
	}
	return nil
}

func hasUI(uiDir string) bool {
	_, err := os.Stat(filepath.Join(uiDir, "package.json"))
	return err == nil
}

func runInDir(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Lint runs golangci-lint on the project.
func Lint() error {
	return sh.Run("golangci-lint", "run", "./...")
}

// Install runs go install for all cmd/ packages.
func Install() error {
	pkgs, err := discoverCmdPackages()
	if err != nil {
		return err
	}
	for _, pkg := range pkgs {
		fmt.Printf("installing %s\n", pkg)
		if err := sh.Run("go", "install", pkg); err != nil {
			return fmt.Errorf("install %s: %w", pkg, err)
		}
	}
	return nil
}

// Clean removes the bin/ directory.
func Clean() error {
	fmt.Printf("removing %s/\n", binDir)
	return os.RemoveAll(binDir)
}

// discoverCmdPackages finds all cmd/*/main.go packages.
func discoverCmdPackages() ([]string, error) {
	entries, err := os.ReadDir("cmd")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read cmd/: %w", err)
	}
	var pkgs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		main := filepath.Join("cmd", e.Name(), "main.go")
		if _, err := os.Stat(main); err == nil {
			pkgs = append(pkgs, "./cmd/"+e.Name())
		}
	}
	return pkgs, nil
}
