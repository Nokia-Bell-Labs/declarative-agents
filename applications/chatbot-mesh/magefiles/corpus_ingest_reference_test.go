// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCorpusIngestApplicationDirectoryContainsOnlyWrapperAndREST(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "agents", "corpus-ingest"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	want := []string{"corpus-rest.yaml", "profile.yaml"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("mesh corpus-ingest assets = %v, want application-only %v", names, want)
	}
	profile, err := os.ReadFile(filepath.Join("..", "agents", "corpus-ingest", "profile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, reference := range []string{
		"../knowledge-manager/corpus-ingest/machine.yaml",
		"../knowledge-manager/corpus-ingest/tools.yaml",
		"../knowledge-manager/corpus-ingest/declarations.yaml",
	} {
		if !strings.Contains(string(profile), reference) {
			t.Errorf("wrapper profile missing canonical runtime reference %s", reference)
		}
	}
}

func TestCorpusIngestRuntimeStagesCanonicalLibraryProgram(t *testing.T) {
	meshRoot := filepath.Clean("..")
	stage, cleanup, err := stageCorpusIngestRuntime(meshRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for _, path := range []string{
		"agents/corpus-ingest/profile.yaml",
		"agents/corpus-ingest/corpus-rest.yaml",
		"agents/knowledge-manager/corpus-ingest/machine.yaml",
		"agents/knowledge-manager/corpus-ingest/tools.yaml",
		"agents/knowledge-manager/corpus-ingest/declarations.yaml",
	} {
		if _, err := os.Stat(filepath.Join(stage, filepath.FromSlash(path))); err != nil {
			t.Errorf("staged runtime missing %s: %v", path, err)
		}
	}
}

func TestChartStagesCanonicalCorpusIngestReference(t *testing.T) {
	applicationRoot := filepath.Clean("..")
	catalogRoot, err := resolveCatalogRoot("corpus-ingest manifest test", applicationRoot)
	if err != nil {
		t.Fatal(err)
	}
	composition, err := resolveChatbotComposition(applicationRoot, catalogRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range composition.manifest.Roots {
		if root.ID == "catalog-corpus-ingest" {
			if root.Ownership != "catalog" ||
				root.Source != "agents/knowledge-manager/corpus-ingest/profile.yaml" {
				t.Fatalf("canonical corpus-ingest root = %#v", root)
			}
			return
		}
	}
	t.Fatal("agents/application.yaml has no catalog-owned corpus-ingest root")
}
