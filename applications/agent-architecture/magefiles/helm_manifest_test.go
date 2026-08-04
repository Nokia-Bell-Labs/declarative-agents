// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
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
	if len(prepared.Closure.Roots) != 8 {
		t.Fatalf("closure provenance roots = %d, want four profiles and four UI roots",
			len(prepared.Closure.Roots))
	}
	if !containsValue(prepared.ExternalUIRoots, "ui-documentation-curator-docs") ||
		containsValue(prepared.ExternalUIRoots, "ui-collector") {
		t.Fatalf("external UI roots = %v, want curator UI external and collector UI packaged",
			prepared.ExternalUIRoots)
	}
	for _, role := range prepared.Roles {
		if role.Role == "applier" && (role.Ownership != "local" ||
			role.Source != "application/agents/applier/profile.yaml") {
			t.Fatalf("applier provenance = %#v, want application-owned root", role)
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
