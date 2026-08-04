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

// Helm groups packaged-chart release targets.
type Helm mg.Namespace

// Package prepares, validates, and packages a self-contained chart archive that
// carries the staged curator and collector closures, so the archive lints and
// renders from a directory with no source checkout present (srd001 R2.4, R2.5).
func (Helm) Package() error {
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		return err
	}
	archive, err := packageHelmChart(filepath.Join(resolved.Application, "helm"), resolved.Catalog, resolved.HelmDist)
	if err != nil {
		return err
	}
	fmt.Printf("packaged self-contained agent-architecture chart %s\n", archive)
	return nil
}

func packageHelmChart(chartRoot, catalogRoot, destination string) (string, error) {
	if _, err := exec.LookPath("helm"); err != nil {
		return "", fmt.Errorf("package agent-architecture chart: helm not found on PATH")
	}
	stage, err := os.MkdirTemp("", "agent-architecture-chart-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	chart := filepath.Join(stage, "agent-architecture")
	if err := stageChartSource(chartRoot, chart); err != nil {
		return "", fmt.Errorf("stage source chart: %w", err)
	}
	// The profile closures ship in the archive, not the checkout: helm/profiles is
	// gitignored, so every packaging path stages them fresh from the pinned
	// catalog or the deployed workloads mount an empty /profiles.
	if err := prepareChartProfiles(filepath.Dir(chartRoot), catalogRoot, chart); err != nil {
		return "", err
	}
	// The curator UI/docs shards do NOT ride in the archive: the gzipped UI is
	// incompressible and carrying it in the Helm release (chart files plus the
	// rendered binaryData) blows past the 3 MiB release limit (GH-1402). The
	// deploy/test tooling provisions the shard ConfigMaps out-of-release and the
	// chart references them by name via curatorUI.shards.
	if err := validatePreparedProfiles(filepath.Join(chart, "profiles")); err != nil {
		return "", fmt.Errorf("validate staged profiles: %w", err)
	}
	if err := validateStagedChart(chart); err != nil {
		return "", err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return "", err
	}
	if output, err := exec.Command("helm", "package", chart, "--destination", destination).CombinedOutput(); err != nil {
		return "", fmt.Errorf("helm package: %w: %s", err, strings.TrimSpace(string(output)))
	}
	archive := filepath.Join(destination, "agent-architecture-0.1.0.tgz")
	if err := validateChartArchive(archive, chart); err != nil {
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
	// Generated profile inputs, curator UI asset shards, and prior release outputs
	// are excluded and never copied into the archive: profiles are regenerated
	// from the catalog below, while curator UI shards are provisioned
	// out-of-release by the deploy/test tooling (GH-1368, GH-1402). A leftover
	// helm/curator-assets tree from a local build is tolerated here but not
	// packaged.
	allowed["profiles"] = true
	allowed[curatorAssetsDir] = true
	allowed["dist"] = true
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".profiles-stage-") || strings.HasPrefix(name, ".curator-assets-stage-") {
			continue
		}
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
			if err := copyDirTree(sourcePath, destinationPath); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("chart source entry %s is not regular", name)
		}
		if err := copyRegularFile(sourcePath, destinationPath); err != nil {
			return err
		}
	}
	return nil
}

func validateStagedChart(chart string) error {
	commands := [][]string{
		{"lint", chart},
		{"template", "validation", chart},
	}
	validFixtures, err := filepath.Glob(filepath.Join(chart, "schema-fixtures", "valid-*.yaml"))
	if err != nil {
		return err
	}
	sort.Strings(validFixtures)
	for _, fixture := range validFixtures {
		commands = append(commands, []string{"template", "validation", chart, "-f", fixture})
	}
	for _, args := range commands {
		if output, err := exec.Command("helm", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("helm %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func validateChartArchive(archive, stagedChart string) error {
	staged, err := regularRelativeFiles(stagedChart)
	if err != nil {
		return err
	}
	required := make(map[string]bool, len(staged))
	for _, path := range staged {
		required["agent-architecture/"+path] = true
	}
	var prepared preparedManifest
	if err := readStrictYAML(filepath.Join(stagedChart, "profiles", preparedManifestFilename), &prepared); err != nil {
		return fmt.Errorf("read staged profile provenance: %w", err)
	}
	mustExist := []string{"agent-architecture/profiles/" + preparedManifestFilename}
	for _, role := range prepared.Roles {
		mustExist = append(mustExist,
			"agent-architecture/profiles/"+role.Path+"/"+role.Profile)
	}
	for _, must := range mustExist {
		if !required[must] {
			return fmt.Errorf("staged chart is missing required file %s; run mage helmPrepare", must)
		}
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
		missing := make([]string, 0, len(required))
		for path := range required {
			missing = append(missing, path)
		}
		sort.Strings(missing)
		return fmt.Errorf("chart archive missing required files: %s", strings.Join(missing, ", "))
	}
	return nil
}
