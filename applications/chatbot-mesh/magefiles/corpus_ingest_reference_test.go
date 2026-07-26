// Copyright (c) 2026 Nokia. All rights reserved.

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
	found := false
	for _, program := range chartProfilePrograms() {
		if program.rel == "profiles/agents/knowledge-manager/corpus-ingest" {
			found = program.src == canonicalCorpusIngestProgram
		}
	}
	if !found {
		t.Fatal("chart packaging does not stage canonical corpus-ingest library assets")
	}
}
