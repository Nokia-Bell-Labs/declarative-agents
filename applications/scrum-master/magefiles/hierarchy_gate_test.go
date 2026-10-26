// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
)

// TestHierarchyDocumentsAreValid gates every theme, initiative, epic,
// feature, story, and objective document in the repository (srd045 R6.1).
// The parent's corpus audit does not read their directories (SPECIFICATIONS
// GAP8), so mage audit checks them here.
func TestHierarchyDocumentsAreValid(t *testing.T) {
	docs, err := hierarchy.Load("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range hierarchy.Validate(docs) {
		t.Error(f)
	}
}

// TestHierarchyGateRejectsMalformedFixtures proves the gate passes the valid
// hierarchy and fails each malformed repository.
func TestHierarchyGateRejectsMalformedFixtures(t *testing.T) {
	root := filepath.Join("..", "testdata", "hierarchy")
	valid, err := hierarchy.Load(filepath.Join(root, "valid"))
	if err != nil || len(valid) == 0 {
		t.Fatalf("valid fixtures: %d %v", len(valid), err)
	}
	if f := hierarchy.Validate(valid); len(f) != 0 {
		t.Errorf("valid fixtures: %v", f)
	}
	cases, err := os.ReadDir(filepath.Join(root, "malformed"))
	if err != nil || len(cases) == 0 {
		t.Fatalf("malformed fixtures: %v", err)
	}
	for _, c := range cases {
		docs, err := hierarchy.Load(filepath.Join(root, "malformed", c.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if len(hierarchy.Validate(docs)) == 0 {
			t.Errorf("malformed fixture %s passed", c.Name())
		}
	}
}
