// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestApplicationManifestDeclaresExactProductionRoots(t *testing.T) {
	applicationRoot := filepath.Clean("..")
	catalogRoot, err := resolveCatalogRoot("application manifest roots test", applicationRoot)
	if err != nil {
		t.Fatal(err)
	}
	composition, err := resolveChatbotComposition(applicationRoot, catalogRoot)
	if err != nil {
		t.Fatal(err)
	}
	var local, catalog []string
	for _, root := range composition.manifest.Roots {
		switch root.Ownership {
		case "local":
			local = append(local, root.Source)
		case "catalog":
			catalog = append(catalog, root.Source)
			if root.CompatibleRelease != "v0.20260804.0" {
				t.Errorf("catalog root %s compatibility = %q", root.ID, root.CompatibleRelease)
			}
		default:
			t.Errorf("root %s has ownership %q", root.ID, root.Ownership)
		}
	}
	sort.Strings(local)
	sort.Strings(catalog)
	wantLocal := []string{
		"agents/applier/profile.yaml",
		"agents/chatbot/profile.yaml",
		"agents/corpus-ingest/profile.yaml",
		"agents/creator/profile.yaml",
		"agents/observer/profile.yaml",
		"agents/provisioning-workflow-orchestrator/profile.yaml",
		"agents/rag-server/profile.yaml",
	}
	wantCatalog := []string{
		"agents/collector/profile.yaml",
		"agents/knowledge-manager/corpus-ingest/profile.yaml",
	}
	if !reflect.DeepEqual(local, wantLocal) {
		t.Fatalf("local roots = %v, want %v", local, wantLocal)
	}
	if !reflect.DeepEqual(catalog, wantCatalog) {
		t.Fatalf("catalog roots = %v, want %v", catalog, wantCatalog)
	}

	// Discover primary local profiles independently so adding a production actor
	// without declaring it fails this test. Fixture profiles below tests/ do not
	// participate.
	entries, err := os.ReadDir(filepath.Join(applicationRoot, "agents"))
	if err != nil {
		t.Fatal(err)
	}
	var discovered []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		profile := filepath.Join(applicationRoot, "agents", entry.Name(), "profile.yaml")
		if info, err := os.Stat(profile); err == nil && info.Mode().IsRegular() {
			discovered = append(discovered, filepath.ToSlash(
				filepath.Join("agents", entry.Name(), "profile.yaml")))
		}
	}
	sort.Strings(discovered)
	if !reflect.DeepEqual(discovered, wantLocal) {
		t.Fatalf("discovered production profiles = %v, manifest local roots = %v",
			discovered, wantLocal)
	}
}

func TestManifestDerivedPackageClosure(t *testing.T) {
	applicationRoot := filepath.Clean("..")
	catalogRoot, err := resolveCatalogRoot("deterministic manifest closure test", applicationRoot)
	if err != nil {
		t.Fatal(err)
	}
	var previous []byte
	for iteration := 0; iteration < 2; iteration++ {
		chart := filepath.Join(t.TempDir(), "chatbot-mesh")
		if err := stageChatbotChartSource(filepath.Join(applicationRoot, "helm"), chart); err != nil {
			t.Fatal(err)
		}
		if err := stageChatbotComposition(chart, applicationRoot, catalogRoot); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(
			chart, filepath.FromSlash(chatbotClosureProvenance)))
		if err != nil {
			t.Fatal(err)
		}
		if iteration > 0 && !bytes.Equal(data, previous) {
			t.Fatal("manifest closure provenance changed across identical staging runs")
		}
		var provenance chatbotPackageProvenance
		if err := yaml.Unmarshal(data, &provenance); err != nil {
			t.Fatal(err)
		}
		if !provenance.Sources.Application.Available ||
			!provenance.Sources.Catalog.Available ||
			provenance.Sources.Application.Revision == "" ||
			provenance.Sources.Catalog.Revision == "" {
			t.Fatalf("checkout provenance = %#v", provenance.Sources)
		}
		for _, root := range provenance.Closure.Roots {
			if root.Ownership == "catalog" && !strings.HasPrefix(root.ID, "ui-") &&
				(!strings.HasPrefix(root.CompatibleRelease, "v0.") ||
					root.CompatibleRelease == provenance.Sources.Catalog.Revision) {
				t.Errorf("catalog root compatibility and source revision are not distinct: %#v, %#v",
					root, provenance.Sources.Catalog)
			}
		}
		previous = data
		for _, forbidden := range []string{
			"profiles/agents/chatbot/tests",
			"profiles/agents/chatbot/ui/app/src",
			"profiles/agents/chatbot/ui/app/node_modules",
			"profiles/agents/observer/ui/src",
			"profiles/agents/observer/ui/node_modules",
		} {
			if _, err := os.Stat(filepath.Join(chart, filepath.FromSlash(forbidden))); !os.IsNotExist(err) {
				t.Errorf("manifest closure carried development tree %s", forbidden)
			}
		}
	}
}

func TestManifestClosureRecordsExpectedUIRuntimeDestinations(t *testing.T) {
	applicationRoot := filepath.Clean("..")
	catalogRoot, err := resolveCatalogRoot("manifest UI destinations test", applicationRoot)
	if err != nil {
		t.Fatal(err)
	}
	chart := filepath.Join(t.TempDir(), "chatbot-mesh")
	if err := stageChatbotChartSource(filepath.Join(applicationRoot, "helm"), chart); err != nil {
		t.Fatal(err)
	}
	if err := stageChatbotComposition(chart, applicationRoot, catalogRoot); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"profiles/agents/chatbot/ui/ui.yaml",
		"profiles/agents/chatbot/ui/app/dist/index.html",
		"profiles/agents/observer/ui/dist/index.html",
		"collector-ui/ui/dist/index.html",
		"profiles/agents/collector/profile.yaml",
		"profiles/agents/knowledge-manager/corpus-ingest/profile.yaml",
	} {
		if _, err := os.Stat(filepath.Join(chart, filepath.FromSlash(required))); err != nil {
			t.Errorf("manifest closure missing %s: %v", required, err)
		}
	}
}

func TestArchiveValidationRejectsMissingManifestClosureFile(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "chatbot-mesh.tgz")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	err = validateChatbotChartArchive(archive, []string{
		"profiles/agents/chatbot/profile.yaml",
		chatbotClosureProvenance,
	})
	if err == nil || !strings.Contains(err.Error(), "missing required files") {
		t.Fatalf("missing manifest closure archive error = %v", err)
	}
}
