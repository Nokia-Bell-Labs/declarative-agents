// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package reusestats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCollectPinsFixtureMetrics(t *testing.T) {
	first := mustCollect(t, ".", "testdata/fixture", "testdata/fixture", "testdata/absent")
	second := mustCollect(t, ".", "testdata/fixture")
	if len(first.TopBlocks) != 1 {
		t.Fatalf("TopBlocks = %#v, want one", first.TopBlocks)
	}
	want := Result{
		TotalLines:       45,
		DuplicatedLines:  3,
		DuplicationRatio: 3.0 / 45.0,
		CeremonyLines:    6,
		BehaviorLines:    8,
		CeremonyRatio:    6.0 / 8.0,
		DistinctToolDefs: 2,
		ToolRefs:         4,
		TopBlocks: []DuplicateBlock{{
			Hash:  "4ad1a5aa11144e222d1a4f9aa0fde2c4d85ec09ddf95ddfcc167db89824c49f7",
			Lines: 3, Count: 2,
			Files: []string{"testdata/fixture/declarations.yaml"},
		}},
	}
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("Collect() = %#v, want %#v", first, want)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated Collect() differs:\n%#v\n%#v", first, second)
	}

	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstJSON, secondJSON) {
		t.Fatalf("repeated JSON differs:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestCollectCanonicalMappingsIgnoreCommentsButPreserveKeyOrder(t *testing.T) {
	root := t.TempDir()
	writeReuseFixture(t, root, "first.yaml", `first:
  limits:
    timeout: 1s # ignored
    retries: 2
`)
	writeReuseFixture(t, root, "second.yaml", `second:
  # ignored
  limits:
    timeout: 1s
    retries: 2
`)
	writeReuseFixture(t, root, "reordered.yaml", `third:
  limits:
    retries: 2
    timeout: 1s
`)

	result := mustCollect(t, root, ".")
	if result.DuplicatedLines != 3 || len(result.TopBlocks) != 1 {
		t.Fatalf("duplicate result = %#v", result)
	}
	block := result.TopBlocks[0]
	if block.Count != 2 || !reflect.DeepEqual(block.Files, []string{"first.yaml", "second.yaml"}) {
		t.Fatalf("duplicate block = %#v", block)
	}
}

func TestCollectParsesUnquotedEnvironmentReferencesDeterministically(t *testing.T) {
	root := t.TempDir()
	writeReuseFixture(t, root, "rest.yaml", `rest:
  limits:
    network:
      ports: [${SERVICE_PORT:-8080}]
  address: ${SERVICE_HOST:-127.0.0.1}:${SERVICE_PORT:-8080}
`)

	first := mustCollect(t, root, ".")
	second := mustCollect(t, root, ".")
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("environment-reference collection differs:\n%#v\n%#v", first, second)
	}
}

func writeReuseFixture(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustCollect(t *testing.T, base string, roots ...string) Result {
	t.Helper()
	result, err := Collect(base, roots...)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// TestCollectCountsUnitEdges pins the reuse acceptance rule of GH-2079: a
// unit earns its place with two importers or an instantiation. The fixture
// has one unit two files import, one unit one file imports twice (one
// importer, not two), and one fragment instantiated once, which is also an
// importer edge of that fragment.
func TestCollectCountsUnitEdges(t *testing.T) {
	result := mustCollect(t, ".", "testdata/units")

	if result.ImportedUnits != 3 {
		t.Fatalf("ImportedUnits = %d, want 3 (shared, only-first, the fragment)", result.ImportedUnits)
	}
	if result.SharedUnits != 1 {
		t.Fatalf("SharedUnits = %d, want 1", result.SharedUnits)
	}
	if result.SingleImporterUnits != 2 {
		t.Fatalf("SingleImporterUnits = %d, want 2", result.SingleImporterUnits)
	}
	if result.Instantiations != 1 {
		t.Fatalf("Instantiations = %d, want 1", result.Instantiations)
	}
}

func TestCollectWithoutImportsReportsNoUnits(t *testing.T) {
	result := mustCollect(t, ".", "testdata/fixture")
	if result.ImportedUnits != 0 || result.SharedUnits != 0 ||
		result.SingleImporterUnits != 0 || result.Instantiations != 0 {
		t.Fatalf("fixture without imports reported units: %#v", result)
	}
}

// TestCollectKeysLibraryRootedImportsByPath is srd056 R1.1: two files in
// different directories importing one agent-core unit by its install path
// share that unit, instead of each joining the path onto its own directory.
func TestCollectKeysLibraryRootedImportsByPath(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"agents/one", "agents/two"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
		writeReuseFixture(t, root, directory+"/declarations.yaml",
			"unit: "+filepath.Base(directory)+"\nimports:\n- /opt/agent-core/tools/units/types-core.yaml\ntools: []\n")
	}

	result := mustCollect(t, root, "agents")

	if result.ImportedUnits != 1 || result.SharedUnits != 1 {
		t.Fatalf("ImportedUnits, SharedUnits = %d, %d; want 1, 1", result.ImportedUnits, result.SharedUnits)
	}
}

// TestCollectCountsTemplateMachineActions is GH-2132: a machine template's
// actions sit under its machine body and count once, at the template.
func TestCollectCountsTemplateMachineActions(t *testing.T) {
	root := t.TempDir()
	writeReuseFixture(t, root, "template.yaml", "unit: serve\nparams:\n  - {name: word, type: string}\nmachine:\n  transitions:\n"+
		"    - {state: Idle, signal: Seed, next: Serving, action: launch}\n    - {state: Serving, signal: Tick, next: Serving, action: $tool}\n")
	writeReuseFixture(t, root, "instance.yaml", "unit: one\ninstantiate:\n  - fragment: template.yaml\n    args: {word: go}\n")

	result := mustCollect(t, root, ".")

	if result.ToolRefs != 1 {
		t.Fatalf("ToolRefs = %d, want 1 from the template body ($tool excluded)", result.ToolRefs)
	}
}
