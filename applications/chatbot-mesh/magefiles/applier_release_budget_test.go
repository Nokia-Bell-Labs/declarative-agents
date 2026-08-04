// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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
	fullArgs := applierLiveValueArgs(staged, runtimeImage, applierImage, nil)
	full, err := measureHelmReleaseBudget(
		applierLiveRelease, staged, fullArchive, fullArgs)
	if err == nil {
		t.Fatalf("release-resident UIs unexpectedly fit the safe budget: %s", full.String())
	}
	if full.ProjectedSecretBytes <= full.BudgetBytes {
		t.Fatalf("baseline budget failure did not measure an overage: %s: %v", full.String(), err)
	}

	assets, cleanupAssets, err := externalizeUIAssets(staged, applierLiveRelease)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupAssets()
	if len(assets) != 2 {
		t.Fatalf("external assets = %d, want collector and observer", len(assets))
	}
	for _, asset := range assets {
		assertExternalAssetArchiveMatchesInventory(t, asset)
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
	thinArgs := applierLiveValueArgs(staged, runtimeImage, applierImage, assets)
	thin, err := measureHelmReleaseBudget(
		applierLiveRelease, staged, thinArchive, thinArgs)
	if err != nil {
		t.Fatalf("externalized release budget failed: %v", err)
	}
	if thin.ProjectedSecretBytes > helmReleaseBudget {
		t.Fatalf("projected release = %d, budget = %d", thin.ProjectedSecretBytes, helmReleaseBudget)
	}
	if thin.ChartArchiveBytes >= full.ChartArchiveBytes {
		t.Fatalf("thin chart archive = %d bytes, full = %d", thin.ChartArchiveBytes, full.ChartArchiveBytes)
	}
	t.Logf("before: %s", full.String())
	t.Logf("after:  %s", thin.String())
}

func TestExternalUIArchivesAreDeterministicForArbitraryRelease(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	const releaseName = "budget-fixture"
	stage := func() ([]externalUIAsset, func()) {
		staged, cleanupChart, err := stageApplierLiveChart(chartDir, filepath.Dir(chartDir))
		if err != nil {
			t.Fatal(err)
		}
		assets, cleanupAssets, err := externalizeUIAssets(staged, releaseName)
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
		wantName := releaseName + "-" + first[index].Component + "-ui-" + first[index].Checksum[:12]
		if first[index].ConfigMapName != wantName {
			t.Errorf("asset %d ConfigMap = %q, want %q",
				index, first[index].ConfigMapName, wantName)
		}
	}
}

func TestExternalUIRenderReferencesButDoesNotStoreAssets(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	staged, cleanupChart, err := stageApplierLiveChart(chartDir, filepath.Dir(chartDir))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupChart()
	const releaseName = "asset-render-fixture"
	assets, cleanupAssets, err := externalizeUIAssets(staged, releaseName)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupAssets()
	valueArgs := applierLiveValueArgs(
		staged,
		"declarative-agents/agent-core:render",
		"declarative-agents/applier:render",
		assets,
	)
	args := append([]string{"template", releaseName, staged}, valueArgs...)
	render, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("render external UI chart: %v\n%s", err, render)
	}
	text := string(render)
	for _, asset := range assets {
		inlineName := releaseName + "-chatbot-mesh-" + asset.Component + "-ui"
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
	archive, cleanupArchive, err := packageApplierChart(staged)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupArchive()
	measured, err := measureHelmReleaseBudget(releaseName, staged, archive, valueArgs)
	if err != nil {
		t.Fatalf("arbitrary release projection failed: %v", err)
	}
	if measured.ProjectedSecretBytes > helmReleaseBudget {
		t.Fatalf("arbitrary release projection = %d, budget = %d",
			measured.ProjectedSecretBytes, helmReleaseBudget)
	}
}

func TestExternalUIAssetValueArgsAreExact(t *testing.T) {
	assets := []externalUIAsset{
		{Component: "collector", ConfigMapName: "matrix-collector-ui-123", Checksum: strings.Repeat("1", 64)},
		{Component: "observer", ConfigMapName: "matrix-observer-ui-abc", Checksum: strings.Repeat("a", 64)},
	}
	got := externalUIAssetValueArgs(assets)
	want := []string{
		"--set", "collector.uiArchiveConfigMap=matrix-collector-ui-123",
		"--set-string", "collector.uiArchiveChecksum=" + strings.Repeat("1", 64),
		"--set", "observer.uiArchiveConfigMap=matrix-observer-ui-abc",
		"--set-string", "observer.uiArchiveChecksum=" + strings.Repeat("a", 64),
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("external asset values:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestExternalUIAssetsVerifyStagedProvenance(t *testing.T) {
	chartDir := findChartDir(t)
	staged, cleanupChart, err := stageApplierLiveChart(chartDir, filepath.Dir(chartDir))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupChart()
	if err := os.WriteFile(
		filepath.Join(staged, "collector-ui", "ui", "dist", "index.html"),
		[]byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	assets, _, err := externalizeUIAssets(staged, "provenance-fixture")
	if err == nil || !strings.Contains(err.Error(), "inventory checksum mismatch") {
		t.Fatalf("tampered provenance file returned assets=%#v, err=%v", assets, err)
	}
}

func TestHelmReleaseProjectionUsesExactValuesAndArbitraryName(t *testing.T) {
	valuesFile := filepath.Join(t.TempDir(), "target-values.yaml")
	values := []byte("collector:\n  enabled: true\n")
	if err := os.WriteFile(valuesFile, values, 0o644); err != nil {
		t.Fatal(err)
	}
	valueArgs := []string{
		"--values", valuesFile,
		"--set", "image.pullPolicy=Never",
		"--set-string", "image.tag=matrix-target",
	}
	payload, err := releaseBudgetValuesPayload(valueArgs)
	if err != nil {
		t.Fatal(err)
	}
	wantPayload := string(values) + "image.pullPolicy=Never\nimage.tag=matrix-target\n"
	if string(payload) != wantPayload {
		t.Fatalf("values payload = %q, want %q", payload, wantPayload)
	}

	const releaseName = "llm-tier-fixture"
	projection, err := helmReleaseProjection(
		releaseName, nil, payload, []byte("kind: Deployment\n"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Name   string `json:"name"`
		Config string `json:"config"`
	}
	if err := json.Unmarshal(projection, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Name != releaseName || decoded.Config != wantPayload {
		t.Fatalf("projection did not preserve exact target inputs: %#v", decoded)
	}
	if helmReleaseBudget != 896<<10 ||
		helmReleaseRequiredMargin != 128<<10 ||
		helmReleaseProjectionAllowance != 64<<10 {
		t.Fatalf("release budget changed: budget=%d margin=%d allowance=%d",
			helmReleaseBudget, helmReleaseRequiredMargin, helmReleaseProjectionAllowance)
	}
}

func TestInspectHelmReleaseSecretsForArbitraryRelease(t *testing.T) {
	const releaseName = "swap-tier-fixture"
	valid := helmSecretListJSON(t, "sh.helm.release.v1."+releaseName+".v1",
		[]byte(`{"name":"swap-tier-fixture","manifest":"kind: Service"}`))
	if count, largest, err := inspectHelmReleaseSecrets(releaseName, valid, nil); err != nil ||
		count != 1 || largest <= 0 || largest > helmReleaseBudget {
		t.Fatalf("Secret assertion count=%d largest=%d err=%v", count, largest, err)
	}
	archive := []byte("external archive fixture")
	encodedArchive := base64.StdEncoding.EncodeToString(archive)
	secretList := helmSecretListJSON(t, "sh.helm.release.v1."+releaseName+".v2",
		[]byte(`{"config":{"archive":"`+encodedArchive+`"}}`))
	_, _, err := inspectHelmReleaseSecrets(
		releaseName, secretList, [][]byte{[]byte(encodedArchive)})
	if err == nil || !strings.Contains(err.Error(), "stores an out-of-release archive") {
		t.Fatalf("external archive Secret assertion error = %v", err)
	}
}

func helmSecretListJSON(t *testing.T, name string, releaseJSON []byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	writer, err := gzip.NewWriterLevel(&compressed, gzip.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(releaseJSON); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	stored := []byte(base64.StdEncoding.EncodeToString(compressed.Bytes()))
	secretList, err := json.Marshal(map[string]any{
		"items": []any{map[string]any{
			"metadata": map[string]any{"name": name},
			"data": map[string]any{
				"release": base64.StdEncoding.EncodeToString(stored),
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return secretList
}

func assertExternalAssetArchiveMatchesInventory(t *testing.T, asset externalUIAsset) {
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
