// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplierReleaseFitsSecretBudgetWithExternalUIAssets(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	staged, cleanupChart, err := stageApplierLiveChart(chartDir, filepath.Dir(chartDir))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupChart()

	const runtimeImage = "declarative-agents/agent-core:budget"
	const applierImage = "declarative-agents/applier:budget"
	fullArchive, cleanupFull, err := packageApplierChart(staged)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupFull()
	full, err := measureApplierReleaseBudget(
		staged, fullArchive, runtimeImage, applierImage, nil)
	if err == nil {
		t.Fatalf("release-resident UIs unexpectedly fit the safe budget: %s", full.String())
	}
	if full.ProjectedSecretBytes <= full.BudgetBytes {
		t.Fatalf("baseline budget failure did not measure an overage: %s: %v", full.String(), err)
	}

	assets, cleanupAssets, err := externalizeApplierLiveUIs(staged)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupAssets()
	if len(assets) != 2 {
		t.Fatalf("external assets = %d, want collector and observer", len(assets))
	}
	for _, asset := range assets {
		assertApplierAssetArchiveMatchesInventory(t, asset)
		info, err := os.Stat(asset.Archive)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("external_asset: component=%s archive=%d files=%d checksum=%s",
			asset.Component, info.Size(), len(asset.Files), asset.Checksum)
	}
	thinArchive, cleanupThin, err := packageApplierChart(staged)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupThin()
	thin, err := measureApplierReleaseBudget(
		staged, thinArchive, runtimeImage, applierImage, assets)
	if err != nil {
		t.Fatalf("externalized release budget failed: %v", err)
	}
	if thin.ProjectedSecretBytes > applierReleaseBudget {
		t.Fatalf("projected release = %d, budget = %d", thin.ProjectedSecretBytes, applierReleaseBudget)
	}
	if thin.ChartArchiveBytes >= full.ChartArchiveBytes {
		t.Fatalf("thin chart archive = %d bytes, full = %d", thin.ChartArchiveBytes, full.ChartArchiveBytes)
	}
	t.Logf("before: %s", full.String())
	t.Logf("after:  %s", thin.String())
}

func TestApplierExternalUIArchivesAreDeterministic(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	stage := func() ([]applierLiveAsset, func()) {
		staged, cleanupChart, err := stageApplierLiveChart(chartDir, filepath.Dir(chartDir))
		if err != nil {
			t.Fatal(err)
		}
		assets, cleanupAssets, err := externalizeApplierLiveUIs(staged)
		if err != nil {
			cleanupChart()
			t.Fatal(err)
		}
		return assets, func() {
			cleanupAssets()
			cleanupChart()
		}
	}
	first, cleanupFirst := stage()
	defer cleanupFirst()
	second, cleanupSecond := stage()
	defer cleanupSecond()
	for index := range first {
		if first[index].Component != second[index].Component ||
			first[index].Checksum != second[index].Checksum ||
			first[index].ConfigMapName != second[index].ConfigMapName {
			t.Fatalf("asset %d changed across identical staging:\nfirst=%#v\nsecond=%#v",
				index, first[index], second[index])
		}
	}
}

func TestApplierExternalUIRenderReferencesButDoesNotStoreAssets(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	staged, cleanupChart, err := stageApplierLiveChart(chartDir, filepath.Dir(chartDir))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupChart()
	assets, cleanupAssets, err := externalizeApplierLiveUIs(staged)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupAssets()
	args := append([]string{"template", applierLiveRelease, staged},
		applierLiveValueArgs(
			staged,
			"declarative-agents/agent-core:render",
			"declarative-agents/applier:render",
			assets,
		)...)
	render, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("render external UI chart: %v\n%s", err, render)
	}
	text := string(render)
	for _, asset := range assets {
		inlineName := applierLiveRelease + "-chatbot-mesh-" + asset.Component + "-ui"
		for _, doc := range strings.Split(text, "\n---") {
			if strings.Contains(doc, "kind: ConfigMap") &&
				strings.Contains(doc, "name: "+inlineName) {
				t.Errorf("release still renders inline %s UI ConfigMap", asset.Component)
			}
		}
		for _, want := range []string{
			"name: " + asset.ConfigMapName,
			asset.Checksum,
			"name: stage-" + asset.Component + "-ui",
			"sha256sum -c -",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("%s external UI render missing %q", asset.Component, want)
			}
		}
	}
	if strings.Contains(text, "chart.tgz:") {
		t.Fatal("render stores chart archive bytes")
	}
}

func assertApplierAssetArchiveMatchesInventory(t *testing.T, asset applierLiveAsset) {
	t.Helper()
	file, err := os.Open(asset.Archive)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gz.Close() }()
	reader := tar.NewReader(gz)
	actual := map[string]string{}
	var checksumManifest string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.FileInfo().IsDir() {
			continue
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == ".inventory.sha256" {
			checksumManifest = string(data)
			continue
		}
		sum := sha256.Sum256(data)
		actual[header.Name] = "sha256:" + hex.EncodeToString(sum[:])
	}
	if len(actual) != len(asset.Files) {
		t.Fatalf("%s archive files = %d, inventory = %d", asset.Component, len(actual), len(asset.Files))
	}
	for _, inventory := range asset.Files {
		if got := actual[inventory.ArchivePath]; got != inventory.Checksum {
			t.Errorf("%s archive %s checksum = %s, inventory = %s",
				asset.Component, inventory.ArchivePath, got, inventory.Checksum)
		}
		if !strings.HasPrefix(inventory.MountedPath, "/") {
			t.Errorf("%s mounted path %q is not explicit", asset.Component, inventory.MountedPath)
		}
		line := strings.TrimPrefix(inventory.Checksum, "sha256:") + "  " + inventory.ArchivePath + "\n"
		if !strings.Contains(checksumManifest, line) {
			t.Errorf("%s archive checksum manifest missing %q", asset.Component, line)
		}
	}
}
