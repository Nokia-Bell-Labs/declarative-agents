// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/appmanifest"
	"gopkg.in/yaml.v3"
)

func TestPreparedProfilesFollowManifest(t *testing.T) {
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}
	chart := t.TempDir()
	if err := prepareChartProfiles(resolved.Application, resolved.Catalog, chart); err != nil {
		t.Fatal(err)
	}
	if err := validatePreparedProfiles(filepath.Join(chart, "profiles")); err != nil {
		t.Fatal(err)
	}

	var prepared preparedManifest
	if err := readStrictYAML(
		filepath.Join(chart, "profiles", preparedManifestFilename), &prepared); err != nil {
		t.Fatal(err)
	}
	if len(prepared.Roles) != 3 {
		t.Fatalf("prepared roles = %d, want three declared deployment entries", len(prepared.Roles))
	}
	if len(prepared.Closure.Roots) != 9 {
		t.Fatalf("closure provenance roots = %d, want four profiles, four UI roots, and catalog docs",
			len(prepared.Closure.Roots))
	}
	if !containsValue(prepared.ExternalAssetRoots, "ui-documentation-curator-docs") ||
		!containsValue(prepared.ExternalAssetRoots, "asset-catalog-docs") ||
		containsValue(prepared.ExternalAssetRoots, "ui-collector") {
		t.Fatalf("external asset roots = %v, want curator UI/docs external and collector UI packaged",
			prepared.ExternalAssetRoots)
	}
	for _, role := range prepared.Roles {
		if role.Role == "applier" && (role.Ownership != "local" ||
			role.Source != "application/agents/applier/profile.yaml") {
			t.Fatalf("applier provenance = %#v, want application-owned root", role)
		}
		if role.Role == "applier" {
			for _, required := range []string{
				"applications/catalog/applier/apply-machine.yaml",
				"applications/catalog/applier/declarations.yaml",
				"applications/agent-architecture/applier/profile.yaml",
				"applications/agent-architecture/applier/exec-declarations.yaml",
			} {
				if !containsValue(role.Files, required) {
					t.Errorf("applier package misses canonical/local closure member %s", required)
				}
			}
		}
	}
}

func TestManifestMutationControlsPreparedProfiles(t *testing.T) {
	applicationRoot, catalogRoot := mutableCompositionFixture(t)
	manifest := loadMutableManifest(t, applicationRoot, catalogRoot)
	for index := range manifest.Roots {
		if manifest.Roots[index].ID == "collector" {
			manifest.Roots[index].Source = "agents/lifecycle-exit/profile.yaml"
			manifest.Roots[index].RuntimePath = "agents/collector-replacement/profile.yaml"
		}
	}
	for index := range manifest.Deployment.Entries {
		if manifest.Deployment.Entries[index].ID == "collector" {
			manifest.Deployment.Entries[index].ProfilePath = "agents/collector-replacement/profile.yaml"
		}
	}
	writeMutableManifest(t, applicationRoot, manifest)

	chart := t.TempDir()
	if err := prepareChartProfiles(applicationRoot, catalogRoot, chart); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(
		chart, "profiles", "collector", "agents", "collector-replacement", "profile.yaml")
	if _, err := os.Stat(replacement); err != nil {
		t.Fatalf("manifest-selected replacement profile was not staged: %v", err)
	}
	if _, err := os.Stat(filepath.Join(
		chart, "profiles", "collector", "agents", "collector", "profile.yaml")); !os.IsNotExist(err) {
		t.Fatalf("old collector profile remained staged after manifest mutation: %v", err)
	}
}

func TestUndeclaredDeploymentRootFailsPreparation(t *testing.T) {
	applicationRoot, catalogRoot := mutableCompositionFixture(t)
	manifest := loadMutableManifest(t, applicationRoot, catalogRoot)
	manifest.Deployment.Entries[0].Root = "undeclared"
	writeMutableManifest(t, applicationRoot, manifest)

	err := prepareChartProfiles(applicationRoot, catalogRoot, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "undeclared root") {
		t.Fatalf("prepare error = %v, want undeclared root rejection", err)
	}
}

func TestManifestMutationControlsCatalogDocsArchive(t *testing.T) {
	applicationRoot, catalogRoot := mutableCompositionFixture(t)
	manifest := loadMutableManifest(t, applicationRoot, catalogRoot)
	manifest.Package.Assets[0].Source = "docs/specs/software-requirements"
	manifest.Package.Assets[0].RuntimePath = "selected-docs"
	manifest.Package.Assets[0].PackagePath = "selected-docs"
	writeMutableManifest(t, applicationRoot, manifest)

	archive, err := buildCuratorAssetArchive(applicationRoot, catalogRoot)
	if err != nil {
		t.Fatal(err)
	}
	names := curatorArchiveNames(t, archive)
	if !hasPathPrefix(names, "selected-docs/") {
		t.Fatalf("mutated catalog docs package path is absent: %v", names)
	}
	if hasPathPrefix(names, "docs/") {
		t.Fatalf("undeclared catalog docs root was added by package code: %v", names)
	}
}

func TestUndeclaredCatalogDocsAreNotArchived(t *testing.T) {
	applicationRoot, catalogRoot := mutableCompositionFixture(t)
	manifest := loadMutableManifest(t, applicationRoot, catalogRoot)
	manifest.Package.Assets = nil
	writeMutableManifest(t, applicationRoot, manifest)

	archive, err := buildCuratorAssetArchive(applicationRoot, catalogRoot)
	if err != nil {
		t.Fatal(err)
	}
	if names := curatorArchiveNames(t, archive); hasPathPrefix(names, "docs/") {
		t.Fatalf("undeclared catalog docs entered archive: %v", names)
	}
}

func mutableCompositionFixture(t *testing.T) (string, string) {
	t.Helper()
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}
	applicationRoot := filepath.Join(t.TempDir(), "agent-architecture")
	copyTree(t, filepath.Join(resolved.Application, "agents"), filepath.Join(applicationRoot, "agents"))
	return applicationRoot, resolved.Catalog
}

func loadMutableManifest(t *testing.T, applicationRoot, catalogRoot string) appmanifest.Manifest {
	t.Helper()
	manifest, err := appmanifest.Load(
		filepath.Join(applicationRoot, "agents", "application.yaml"),
		appmanifest.Options{ApplicationRoot: applicationRoot, CatalogRoot: catalogRoot})
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func writeMutableManifest(t *testing.T, applicationRoot string, manifest appmanifest.Manifest) {
	t.Helper()
	data, err := yaml.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(applicationRoot, "agents", "application.yaml"), string(data))
}

func curatorArchiveNames(t *testing.T, archive []byte) []string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gz.Close() }()
	reader := tar.NewReader(gz)
	var names []string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
	}
	sort.Strings(names)
	return names
}

func hasPathPrefix(paths []string, prefix string) bool {
	for _, path := range paths {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
