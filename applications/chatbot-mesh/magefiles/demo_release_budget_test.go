// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"path/filepath"
	"testing"
)

// mage demo:up used to install the chart with its UIs resident in the release,
// the one kind path that skipped externalization. The release Secret projected to
// 1176792 bytes against a 1048576 limit, so the install died creating it after
// the cluster and images were already built, and the failure named a Secret size
// rather than the chart (GH-1475).
//
// These pin both halves of the fix: the demo release fits its budget once the UI
// assets travel out of release, and it demonstrably does not fit with them left
// inside, so the externalization cannot be quietly dropped again.

// stageDemoBudgetChart stages the chart the demo installs and packages the
// out-of-release archive the projection needs.
func stageDemoBudgetChart(t *testing.T) (staged, archive string) {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	staged, cleanupChart, err := stageSmokeChart(applicationChartDir(root), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanupChart)
	return staged, packageDemoBudgetArchive(t, staged)
}

func packageDemoBudgetArchive(t *testing.T, staged string) string {
	t.Helper()
	archive, cleanup, err := packageApplierChart(staged)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	return archive
}

const demoBudgetImage = "declarative-agents/agent-core:budget"

// TestDemoReleaseFitsTheSecretBudget is the regression guard: the release the
// demo installs must fit, measured the same way the install gate measures it.
func TestDemoReleaseFitsTheSecretBudget(t *testing.T) {
	staged, _ := stageDemoBudgetChart(t)
	assets, cleanupAssets, err := externalizeUIAssets(staged, chatbotDemoRelease)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanupAssets)
	if len(assets) == 0 {
		t.Fatal("no UI assets externalized for the demo release")
	}
	// Package after externalization: the archive must be the thinned chart.
	archive := packageDemoBudgetArchive(t, staged)

	measured, err := measureHelmReleaseBudget(
		chatbotDemoRelease, staged, archive, demoValueArgs(staged, demoBudgetImage, assets))
	if err != nil {
		t.Fatalf("demo release does not fit its budget: %v", err)
	}
	if measured.ProjectedSecretBytes > measured.BudgetBytes {
		t.Errorf("demo release projects %d bytes over budget: %s",
			measured.ProjectedSecretBytes-measured.BudgetBytes, measured.String())
	}
	if measured.ProjectedSecretBytes >= kubernetesSecretLimit {
		t.Errorf("demo release projects past the Kubernetes Secret limit: %s", measured.String())
	}
}

// TestDemoReleaseNeedsExternalUIAssets shows the guard has teeth. Skipping
// externalization is the GH-1475 defect, and it must measure as an overage rather
// than passing quietly.
func TestDemoReleaseNeedsExternalUIAssets(t *testing.T) {
	staged, archive := stageDemoBudgetChart(t)

	measured, err := measureHelmReleaseBudget(
		chatbotDemoRelease, staged, archive, demoValueArgs(staged, demoBudgetImage, nil))
	if err == nil {
		t.Fatalf("release-resident UIs unexpectedly fit the demo budget: %s", measured.String())
	}
	if measured.ProjectedSecretBytes <= measured.BudgetBytes {
		t.Fatalf("baseline overage was not measured: %s: %v", measured.String(), err)
	}
}
