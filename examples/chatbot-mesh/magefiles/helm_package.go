// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/magefile/mage/mg"
)

const helmPackageDirEnv = "HELM_PACKAGE_DIR"

// Helm groups operator-facing chart packaging targets.
type Helm mg.Namespace

// Package stages the mesh profiles and UI into an installable Helm archive.
//
// The source chart intentionally does not duplicate the canonical programs under
// agents/ and ux/. Operators must install this packaged artifact, not the
// unstaged helm/ directory.
func (Helm) Package() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	destination := envOrDefault(helmPackageDirEnv, filepath.Join(root, "helm", "dist"))
	return packageHelmChart(filepath.Join(root, "helm"), root, destination)
}

func packageHelmChart(chartDir, profilesRoot, destination string) error {
	if _, err := exec.LookPath("helm"); err != nil {
		return fmt.Errorf("package chatbot-mesh chart: helm not found on PATH")
	}
	staged, cleanup, err := stageSmokeChart(chartDir, profilesRoot)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("create Helm package destination %s: %w", destination, err)
	}
	cmd := exec.Command("helm", "package", staged, "--destination", destination)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("package staged chatbot-mesh chart: %w", err)
	}
	fmt.Printf("helm:package: install the staged chart from %s\n", destination)
	return nil
}
