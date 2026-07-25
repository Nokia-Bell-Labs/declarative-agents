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
	"ci",
	"schema-fixtures",
	"templates",
	"values.schema.json",
	"values.yaml",
}

var chartArchiveInventory = []string{
	"coding-agent/.helmignore",
	"coding-agent/Chart.yaml",
	"coding-agent/ci/small-values.yaml",
	"coding-agent/schema-fixtures/README.md",
	"coding-agent/schema-fixtures/invalid-image.yaml",
	"coding-agent/schema-fixtures/invalid-models.yaml",
	"coding-agent/schema-fixtures/invalid-mount.yaml",
	"coding-agent/schema-fixtures/invalid-port.yaml",
	"coding-agent/schema-fixtures/invalid-replicas.yaml",
	"coding-agent/schema-fixtures/invalid-resources.yaml",
	"coding-agent/schema-fixtures/invalid-storage.yaml",
	"coding-agent/schema-fixtures/invalid-telemetry.yaml",
	"coding-agent/schema-fixtures/invalid-url.yaml",
	"coding-agent/schema-fixtures/valid-collector-debug.yaml",
	"coding-agent/schema-fixtures/valid-existing-workspace.yaml",
	"coding-agent/schema-fixtures/valid-external-llm.yaml",
	"coding-agent/schema-fixtures/valid-incluster-ollama.yaml",
	"coding-agent/schema-fixtures/valid-no-telemetry.yaml",
	"coding-agent/templates/NOTES.txt",
	"coding-agent/templates/_helpers.tpl",
	"coding-agent/templates/agents.yaml",
	"coding-agent/templates/collector.yaml",
	"coding-agent/templates/jaeger.yaml",
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
	defer os.RemoveAll(stage)
	chart := filepath.Join(stage, "coding-agent")
	if err := stageChartSource(chartRoot, chart); err != nil {
		return "", fmt.Errorf("stage source chart: %w", err)
	}
	if err := prepareHelmProfiles(profilesRoot, chart); err != nil {
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
	if err := validatePreparedPackage(filepath.Join(chart, "profiles")); err != nil {
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
	file, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("open chart archive: %w", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open chart gzip: %w", err)
	}
	defer gz.Close()
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
