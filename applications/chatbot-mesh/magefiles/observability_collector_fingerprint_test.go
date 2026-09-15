// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// syntheticCollectorCheckout lays down the shape the fingerprint covers: a
// runtime tree, a collector directory, and the type unit its declarations
// import from beside that directory.
func syntheticCollectorCheckout(t *testing.T) (coreRoot, catalogRoot, unit string) {
	t.Helper()
	root := t.TempDir()
	coreRoot = filepath.Join(root, "agent-core")
	catalogRoot = filepath.Join(root, "catalog")
	write := func(path, body string) string {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write(filepath.Join(coreRoot, "main.go"), "package main\n")
	write(filepath.Join(catalogRoot, "agents", "collector", "declarations.yaml"),
		"unit: collector\nimports:\n- ../units/types-collector.yaml\ntools: []\n")
	unit = write(filepath.Join(catalogRoot, "agents", "units", "types-collector.yaml"),
		"unit: types-collector\ntypes:\n- name: SpanStatsResponse\n  schema:\n    type: object\n")
	return coreRoot, catalogRoot, unit
}

// TestCollectorFingerprintCoversTheImportedUnits is the GH-2040 regression. The
// fingerprint gates process reuse, so a type the collector loads must change it;
// walking the agent directory alone never reached the unit beside it.
func TestCollectorFingerprintCoversTheImportedUnits(t *testing.T) {
	t.Parallel()
	coreRoot, catalogRoot, unit := syntheticCollectorCheckout(t)

	before, err := collectorFingerprintOf(coreRoot, catalogRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unit,
		[]byte("unit: types-collector\ntypes:\n- name: SpanStatsResponse\n  schema:\n    type: string\n"),
		0o644); err != nil {
		t.Fatal(err)
	}

	after, err := collectorFingerprintOf(coreRoot, catalogRoot)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("editing an imported type left the fingerprint unchanged, so a running collector would be reused against declarations it was not launched with")
	}
}

// TestCollectorFingerprintNamesTheImportedUnit keeps the logical name out of the
// checkout path, so two checkouts of the same source agree on the digest.
func TestCollectorFingerprintNamesTheImportedUnit(t *testing.T) {
	t.Parallel()
	coreRoot, catalogRoot, _ := syntheticCollectorCheckout(t)

	files, err := collectorFingerprintSources(coreRoot, catalogRoot)
	if err != nil {
		t.Fatal(err)
	}
	want := "collector-imports/agents/units/types-collector.yaml"
	for _, file := range files {
		if file.logical == want {
			return
		}
	}
	t.Fatalf("logical names %v do not include %s", logicalNames(files), want)
}

func logicalNames(files []collectorFingerprintSource) []string {
	names := make([]string, 0, len(files))
	for _, file := range files {
		names = append(names, file.logical)
	}
	return names
}
