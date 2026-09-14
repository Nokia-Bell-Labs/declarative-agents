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
		TotalLines:       39,
		DuplicatedLines:  3,
		DuplicationRatio: 3.0 / 39.0,
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
