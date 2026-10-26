// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads"
)

// TestScaleSeedIsATrackerFile writes the seed the way the store keeps it,
// in either layout: every ID and IID once, the reserved issues ready to pop,
// and the JSONL file sorted by ID.
func TestScaleSeedIsATrackerFile(t *testing.T) {
	for _, layout := range []string{beads.LayoutFiles, "jsonl"} {
		dir := t.TempDir()
		size, err := writeScaleIssues(dir, layout, 500, 40)
		if err != nil || size == 0 {
			t.Fatalf("%s seed: %d bytes, %v", layout, size, err)
		}
		issues, err := readScaleIssues(dir, layout)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		iids := map[int]bool{}
		for _, i := range issues {
			if iids[i.IID] {
				t.Fatalf("%s: IID %d twice", layout, i.IID)
			}
			iids[i.IID] = true
			ids = append(ids, i.ID)
			if i.IID <= 40 && i.State != beads.Ready {
				t.Errorf("%s: reserved issue %d is %s, not ready", layout, i.IID, i.State)
			}
			if len(i.Comments) != 5 {
				t.Errorf("%s: issue %d has %d comments", layout, i.IID, len(i.Comments))
			}
		}
		if len(ids) != 500 || len(slices.Compact(slices.Sorted(slices.Values(ids)))) != 500 {
			t.Fatalf("%s: %d issues", layout, len(ids))
		}
		if layout == "jsonl" && !slices.IsSorted(ids) {
			t.Errorf("the JSONL seed is not sorted by ID")
		}
		if layout == beads.LayoutFiles {
			if b, err := os.ReadFile(filepath.Join(dir, beads.ConfigFile)); err != nil || !strings.Contains(string(b), "layout: files") {
				t.Errorf("the files seed's config = %q, %v", b, err)
			}
			for _, i := range issues {
				if b, err := os.ReadFile(filepath.Join(dir, beads.IndexEntry(i.Repository, i.IID))); err != nil || strings.TrimSpace(string(b)) != i.ID {
					t.Fatalf("the index entry for IID %d = %q, %v; want %s", i.IID, b, err, i.ID)
				}
			}
		}
	}
}

func TestScaleSizesReadsTheEnvironment(t *testing.T) {
	t.Setenv("BEADS_SCALE_TEST", "")
	if got, err := scaleSizes("BEADS_SCALE_TEST", "1,2"); err != nil || !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("default: %v, %v", got, err)
	}
	t.Setenv("BEADS_SCALE_TEST", "100, 300")
	if got, err := scaleSizes("BEADS_SCALE_TEST", "1"); err != nil || !slices.Equal(got, []int{100, 300}) {
		t.Fatalf("set: %v, %v", got, err)
	}
	t.Setenv("BEADS_SCALE_TEST", "x")
	if _, err := scaleSizes("BEADS_SCALE_TEST", "1"); err == nil {
		t.Fatal("a non-number was accepted")
	}
}
