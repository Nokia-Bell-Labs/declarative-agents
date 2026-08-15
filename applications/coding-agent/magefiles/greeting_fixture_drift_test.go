// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The coding-loop workspace is the canonical greeting fixture. Two retired
// catalog fixtures carry byte-identical copies of the shared triad so their
// recorded integration evidence keeps compiling and passing. Without an
// executable contract the copies can silently drift from the canonical
// source. This test is that contract: it fails the moment a copy diverges,
// forcing an intentional regeneration rather than accidental drift.
//
// Candidate copies with deliberately opposite outcomes (accepted/rejected)
// are intentionally excluded — their behavior is tested elsewhere and must
// differ.

// canonicalGreetingWorkspace is the source of truth for the greeting fixture.
func canonicalGreetingWorkspace() string {
	return filepath.Join("..", "testdata", "integration", "coding-loop", "workspace")
}

// retiredGreetingCopies are the compatibility copies that must track the
// canonical workspace byte-for-byte. Paths are relative to this module root.
func retiredGreetingCopies() []string {
	return []string{
		filepath.Join("..", "..", "catalog", "testdata", "integration", "uc001-generator-coding"),
		filepath.Join("..", "..", "catalog", "testdata", "integration", "uc002-evaluator-benchmark", "samples", "greet", "workspace"),
	}
}

// greetingSharedTriad lists the files that define the fixture's behavior and
// therefore must not drift between the canonical workspace and its copies.
func greetingSharedTriad() []string {
	return []string{"go.mod", "greet.go", "greet_test.go"}
}

func TestGreetingFixtureCopiesTrackCanonical(t *testing.T) {
	canonical := canonicalGreetingWorkspace()

	want := make(map[string][]byte, len(greetingSharedTriad()))
	for _, name := range greetingSharedTriad() {
		data, err := os.ReadFile(filepath.Join(canonical, name))
		if err != nil {
			t.Fatalf("read canonical %s: %v", name, err)
		}
		want[name] = data
	}

	for _, copyDir := range retiredGreetingCopies() {
		for _, name := range greetingSharedTriad() {
			got, err := os.ReadFile(filepath.Join(copyDir, name))
			if err != nil {
				t.Errorf("read copy %s: %v", filepath.Join(copyDir, name), err)
				continue
			}
			if string(got) != string(want[name]) {
				t.Errorf("greeting fixture drift: %s differs from canonical %s;\n"+
					"regenerate the copy from the coding-loop workspace instead of editing it in place",
					filepath.Join(copyDir, name), filepath.Join(canonical, name))
			}
		}
	}
}
