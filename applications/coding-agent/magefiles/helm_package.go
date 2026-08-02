// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/magefile/mage/mg"
)

const helmPackageDirEnv = "HELM_PACKAGE_DIR"

var chartSourceInventory = []string{
	".helmignore",
	"Chart.yaml",
	"PACKAGING.md",
	"README.md",
	"ci",
	"schema-fixtures",
	"templates",
	"values.schema.json",
	"values.yaml",
}

var chartArchiveInventory = []string{
	"coding-agent/.helmignore",
	"coding-agent/Chart.yaml",
	"coding-agent/PACKAGING.md",
	"coding-agent/README.md",
	"coding-agent/ci/kind-applier-values.yaml",
	"coding-agent/ci/kind-config.yaml",
	"coding-agent/ci/kind-demo-config.yaml",
	"coding-agent/ci/kind-values.yaml",
	"coding-agent/ci/kind-workspace.yaml",
	"coding-agent/ci/small-values.yaml",
	"coding-agent/schema-fixtures/README.md",
	"coding-agent/schema-fixtures/invalid-applier-port.yaml",
	"coding-agent/schema-fixtures/invalid-image.yaml",
	"coding-agent/schema-fixtures/invalid-models.yaml",
	"coding-agent/schema-fixtures/invalid-mount.yaml",
	"coding-agent/schema-fixtures/invalid-port.yaml",
	"coding-agent/schema-fixtures/invalid-replicas.yaml",
	"coding-agent/schema-fixtures/invalid-resources.yaml",
	"coding-agent/schema-fixtures/invalid-storage.yaml",
	"coding-agent/schema-fixtures/invalid-url.yaml",
	"coding-agent/schema-fixtures/valid-applier-enabled.yaml",
	"coding-agent/schema-fixtures/valid-collector-spool.yaml",
	"coding-agent/schema-fixtures/valid-existing-workspace.yaml",
	"coding-agent/schema-fixtures/valid-external-llm.yaml",
	"coding-agent/schema-fixtures/valid-incluster-ollama.yaml",
	"coding-agent/schema-fixtures/valid-no-telemetry.yaml",
	"coding-agent/templates/NOTES.txt",
	"coding-agent/templates/_helpers.tpl",
	"coding-agent/templates/agents.yaml",
	"coding-agent/templates/applier.yaml",
	"coding-agent/templates/collector.yaml",
	"coding-agent/templates/ollama.yaml",
	"coding-agent/templates/profiles-configmaps.yaml",
	"coding-agent/templates/workspace.yaml",
	"coding-agent/values.schema.json",
	"coding-agent/values.yaml",
}

// Helm groups packaged-chart release targets.
type Helm mg.Namespace

// Package prepares, validates, and packages a self-contained chart archive.
func (Helm) Package() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := Package(); err != nil {
		return err
	}
	profiles := envOrDefault(
		profileOutputEnv, filepath.Join(root, filepath.FromSlash(defaultProfileOutput)))
	destination := envOrDefault(
		helmPackageDirEnv, filepath.Join(root, "helm", "dist"))
	archive, err := packageHelmChart(
		filepath.Join(root, "helm"), profiles, destination)
	if err != nil {
		return err
	}
	fmt.Printf("packaged self-contained coding-agent chart %s\n", archive)
	return nil
}

func packageHelmChart(chartRoot, profilesRoot, destination string) (string, error) {
	if _, err := exec.LookPath("helm"); err != nil {
		return "", fmt.Errorf("package coding-agent chart: helm not found on PATH")
	}
	stage, err := os.MkdirTemp("", "coding-agent-chart-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	chart := filepath.Join(stage, "coding-agent")
	if err := stageChartSource(chartRoot, chart); err != nil {
		return "", fmt.Errorf("stage source chart: %w", err)
	}
	if err := prepareHelmProfiles(profilesRoot, chart); err != nil {
		return "", err
	}
	// The collector profile ships in the archive, not the checkout: helm/profiles/
	// is gitignored, so every packaging path must stage it or the deployed
	// collector mounts an empty /profiles and crash-loops (GH-1162). The catalog
	// resolves from the working directory, not chartRoot, because callers may
	// package a chart copy staged outside the repository.
	catalogRoot, err := resolveCatalogRoot("package coding-agent chart", ".")
	if err != nil {
		return "", err
	}
	if err := stageCollectorProfile(catalogRoot, chart); err != nil {
		return "", err
	}
	// The applier profile ships in the archive the same way the collector does:
	// helm/profiles/ is gitignored, so every packaging path must stage it or the
	// deployed applier mounts an empty /profiles. It is application-owned, so it
	// resolves from the coding-agent application root rather than the catalog root.
	applierRoot, err := resolveApplicationRoot("package coding-agent chart")
	if err != nil {
		return "", err
	}
	if err := stageApplierProfile(applierRoot, chart); err != nil {
		return "", err
	}
	if err := validateStagedChart(chart); err != nil {
		return "", err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return "", err
	}
	if output, err := exec.Command(
		"helm", "package", chart, "--destination", destination).CombinedOutput(); err != nil {
		return "", fmt.Errorf("helm package: %w: %s", err, strings.TrimSpace(string(output)))
	}
	archive := filepath.Join(destination, "coding-agent-0.1.0.tgz")
	if err := validateChartArchive(archive, profilesRoot); err != nil {
		return "", err
	}
	if output, err := exec.Command("helm", "lint", archive).CombinedOutput(); err != nil {
		return "", fmt.Errorf("lint packaged chart: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if output, err := exec.Command("helm", "template", "packaged", archive).CombinedOutput(); err != nil {
		return "", fmt.Errorf("render packaged chart: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return archive, nil
}

func stageChartSource(source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	allowed := make(map[string]bool, len(chartSourceInventory)+2)
	for _, name := range chartSourceInventory {
		allowed[name] = true
	}
	// Generated package inputs and prior release outputs are deliberately
	// excluded. They are replaced from #875 output or regenerated below.
	allowed["profiles"] = true
	allowed["dist"] = true
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !allowed[name] {
			return fmt.Errorf("chart source contains unclassified top-level entry %s", name)
		}
		seen[name] = true
	}
	for _, name := range chartSourceInventory {
		if !seen[name] {
			return fmt.Errorf("chart source missing required entry %s", name)
		}
		sourcePath := filepath.Join(source, name)
		destinationPath := filepath.Join(destination, name)
		info, err := os.Lstat(sourcePath)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("chart source entry %s is a symlink", name)
		}
		if info.IsDir() {
			if err := copySourceTreeStrict(sourcePath, destinationPath); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("chart source entry %s is not regular", name)
		}
		if err := copyProfileAsset(sourcePath, destinationPath); err != nil {
			return err
		}
	}
	return nil
}

func validateStagedChart(chart string) error {
	if err := validatePreparedPackage(filepath.Join(chart, "profiles"), "collector", "applier"); err != nil {
		return err
	}
	commands := [][]string{
		{"lint", chart},
		{"template", "validation", chart},
		{"template", "validation", chart, "-f", filepath.Join(chart, "ci", "small-values.yaml")},
	}
	validFixtures, err := filepath.Glob(filepath.Join(chart, "schema-fixtures", "valid-*.yaml"))
	if err != nil {
		return err
	}
	sort.Strings(validFixtures)
	for _, fixture := range validFixtures {
		commands = append(commands,
			[]string{"template", "validation", chart, "-f", fixture})
	}
	for _, args := range commands {
		if output, err := exec.Command("helm", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("helm %s: %w: %s",
				strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func validateChartArchive(archive, profilesRoot string) error {
	expected, err := regularRelativeFiles(profilesRoot)
	if err != nil {
		return err
	}
	required := make(map[string]bool, len(chartArchiveInventory)+len(expected))
	for _, path := range chartArchiveInventory {
		required[path] = true
	}
	for _, path := range expected {
		required["coding-agent/profiles/"+path] = true
	}
	// The staged collector profile mirrors stageCollectorProfile: the
	// top-level regular files of the catalog collector family.
	catalogRoot, err := resolveCatalogRoot("validate coding-agent chart archive", ".")
	if err != nil {
		return err
	}
	collectorEntries, err := os.ReadDir(filepath.Join(catalogRoot, "agents", "collector"))
	if err != nil {
		return err
	}
	for _, entry := range collectorEntries {
		if !entry.Type().IsRegular() {
			continue
		}
		required["coding-agent/profiles/collector/agents/collector/"+entry.Name()] = true
	}
	// The collector's served trace UI ships as ui/dist (srd020 R7); the staged
	// tree carries every built asset, so each is a required archive entry.
	collectorUIRoot := filepath.Join(catalogRoot, "agents", "collector", "ui", "dist")
	if err := filepath.Walk(collectorUIRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(collectorUIRoot, path)
		if err != nil {
			return err
		}
		required["coding-agent/profiles/collector/agents/collector/ui/dist/"+filepath.ToSlash(rel)] = true
		return nil
	}); err != nil {
		return err
	}
	// The staged applier profile mirrors stageApplierProfile: the top-level regular
	// files of the application's agents/serving/applier family.
	applierRoot, err := resolveApplicationRoot("validate coding-agent chart archive")
	if err != nil {
		return err
	}
	applierEntries, err := os.ReadDir(filepath.Join(applierRoot, "agents", "serving", "applier"))
	if err != nil {
		return err
	}
	for _, entry := range applierEntries {
		if !entry.Type().IsRegular() {
			continue
		}
		required["coding-agent/profiles/applier/agents/applier/"+entry.Name()] = true
	}
	file, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("open chart archive: %w", err)
	}
	defer func() { _ = file.Close() }()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open chart gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read chart archive: %w", err)
		}
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			return fmt.Errorf("chart archive contains link %s", header.Name)
		}
		name := filepath.ToSlash(header.Name)
		if header.FileInfo().IsDir() {
			continue
		}
		if !required[name] {
			return fmt.Errorf("chart archive contains unexpected file %s", name)
		}
		delete(required, name)
	}
	if len(required) > 0 {
		var missing []string
		for path := range required {
			missing = append(missing, path)
		}
		sort.Strings(missing)
		return fmt.Errorf("chart archive missing required files: %s", strings.Join(missing, ", "))
	}
	return nil
}
