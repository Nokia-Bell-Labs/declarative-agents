// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"path/filepath"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/objectives"
)

// TestObjectiveDocumentsAreValid gates every initiative and objective
// document in the repository against srd034 R1 (srd034 R2.1). The parent's
// corpus audit does not read docs/specs/initiatives/ or docs/specs/objectives/
// (SPECIFICATIONS GAP8), so mage audit checks them here.
func TestObjectiveDocumentsAreValid(t *testing.T) {
	docs, err := objectives.LoadAll("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range docs {
		for _, f := range objectives.Validate(d) {
			t.Error(f)
		}
	}
}

// TestObjectiveGateRejectsMalformedFixtures proves the gate fails on each
// malformed fixture and passes the valid one.
func TestObjectiveGateRejectsMalformedFixtures(t *testing.T) {
	valid, err := objectives.LoadDir(filepath.Join("..", "testdata", "objectives", "valid"))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range valid {
		if f := objectives.Validate(d); len(f) != 0 {
			t.Errorf("valid fixture %s: %v", d.Path, f)
		}
	}
	malformed, err := objectives.LoadDir(filepath.Join("..", "testdata", "objectives", "malformed"))
	if err != nil {
		t.Fatal(err)
	}
	if len(malformed) == 0 {
		t.Fatal("no malformed fixtures")
	}
	for _, d := range malformed {
		if len(objectives.Validate(d)) == 0 {
			t.Errorf("malformed fixture %s passed", d.Path)
		}
	}
}
