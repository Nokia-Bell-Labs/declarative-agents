// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// HelmPrepare regenerates every manifest-declared deployment package and stages
// the exact generated inventory into the source chart.
func HelmPrepare() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := Package(); err != nil {
		return err
	}
	source := demoProfilesOutput(root)
	if err := prepareHelmProfiles(source, filepath.Join(root, "helm")); err != nil {
		return err
	}
	fmt.Printf("prepared Helm profile artifacts from %s\n", source)
	return nil
}

func prepareHelmProfiles(packageRoot, chartRoot string) error {
	if _, err := os.Stat(filepath.Join(packageRoot, "deployment-manifest.yaml")); err != nil {
		return fmt.Errorf("prepared profile package has no deployment manifest: %w", err)
	}
	if err := validatePreparedPackage(packageRoot); err != nil {
		return fmt.Errorf("validate prepared profile package: %w", err)
	}
	parent := chartRoot
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".profiles-stage-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	if err := copyTree(packageRoot, stage); err != nil {
		return fmt.Errorf("stage Helm profiles: %w", err)
	}
	destination := filepath.Join(chartRoot, "profiles")
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := os.Rename(stage, destination); err != nil {
		return fmt.Errorf("publish Helm profiles: %w", err)
	}
	return nil
}
