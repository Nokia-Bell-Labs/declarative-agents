// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package checkout

import (
	"errors"
	"os/exec"
	"slices"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// TestTolerateMissing: while tolerated, a project with no checkout reads as absent and
// is reported; once restored, it is the error it was.
func TestTolerateMissing(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"remote", "add", "origin", "https://gitlab.example.com/g/own.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	c := &Code{Dir: dir}
	if _, err := c.GetFile("g/none", "README.md", "main"); !errors.Is(err, ErrNoCheckout) {
		t.Fatalf("not tolerated = %v", err)
	}
	var heard []string
	restore := TolerateMissing(func(p string) { heard = append(heard, p) })
	defer restore()
	if _, err := c.GetFile("g/none", "README.md", "main"); !errors.Is(err, tracker.ErrNotFound) || errors.Is(err, ErrNoCheckout) {
		t.Fatalf("tolerated = %v", err)
	}
	if !slices.Contains(heard, "g/none") {
		t.Fatalf("heard = %v", heard)
	}
	if merges, err := c.Merges("g/none", "", "2026-11-01"); err != nil || len(merges) != 0 {
		t.Fatalf("merges of an absent project = %v, %v", merges, err)
	}
	restore()
	if _, err := c.GetFile("g/none", "README.md", "main"); !errors.Is(err, ErrNoCheckout) {
		t.Fatalf("restored = %v", err)
	}
}
