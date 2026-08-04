// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestHelmReadmeInstallsOnlyPackagedArtifacts proves the README teaches the
// packaging path, not a bare source-chart install: the agent programs live beside
// the chart under agents/ and in the catalog, so `helm install <chart>/helm` would
// ship a chart with no staged profiles.
func TestHelmReadmeInstallsOnlyPackagedArtifacts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(findChartDir(t), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := string(data)
	sourceInstall := regexp.MustCompile(`(?m)^\s*helm install\b[^\n]*\b(?:applications/agent-architecture/)?helm(?:\s|$)`)
	if command := sourceInstall.FindString(readme); command != "" {
		t.Fatalf("README installs the unstaged source chart: %s", command)
	}
	for _, required := range []string{
		"mage helm:package",
		"helm/dist/agent-architecture-*.tgz",
		"ci/kind-applier-values.yaml",
	} {
		if !strings.Contains(readme, required) {
			t.Errorf("README is missing %q", required)
		}
	}
}

// TestHelmPackagingDocumentsTheApplierStaging proves PACKAGING.md records the
// manifest-owned applier deployment and its render-time placeholder rewrite.
func TestHelmPackagingDocumentsTheApplierStaging(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(findChartDir(t), "PACKAGING.md"))
	if err != nil {
		t.Fatal(err)
	}
	packaging := string(data)
	for _, required := range []string{
		"manifest's `applier` deployment entry",
		"exec-declarations.yaml",
		"Release.Name",
	} {
		if !strings.Contains(packaging, required) {
			t.Errorf("PACKAGING.md no longer documents %q; the applier staging invariant is unrecorded", required)
		}
	}
}
