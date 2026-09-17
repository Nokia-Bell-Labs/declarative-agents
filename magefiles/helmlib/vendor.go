// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package helmlib vendors the shared agent-services library chart into an
// application chart. Helm resolves a dependency only from the chart's own
// charts/ directory, and the application charts are packaged from a staging
// copy, so the library is copied in rather than fetched (GH-2045).
package helmlib

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Name is the library chart's directory and Chart.yaml name.
const Name = "agent-services"

// SourceDir returns the library chart's place in the repository.
func SourceDir(repoRoot string) string {
	return filepath.Join(repoRoot, "applications", "charts", Name)
}

// Vendor replaces chartRoot/charts/agent-services with a fresh copy of the
// library chart, so a render or package sees exactly the repository's version.
//
// A chart copied out of the repository carries its vendored copy and has no
// library source beside it; that copy is left as it stands.
func Vendor(repoRoot, chartRoot string) error {
	source := SourceDir(repoRoot)
	if _, err := os.Stat(filepath.Join(source, "Chart.yaml")); err != nil {
		if _, vendored := os.Stat(filepath.Join(chartRoot, "charts", Name, "Chart.yaml")); vendored == nil {
			return nil
		}
		return fmt.Errorf("library chart %s: %w", source, err)
	}
	destination := filepath.Join(chartRoot, "charts", Name)
	if err := os.RemoveAll(destination); err != nil {
		return fmt.Errorf("clear vendored library chart: %w", err)
	}
	return copyTree(source, destination)
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("library chart %s is not a regular file", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
