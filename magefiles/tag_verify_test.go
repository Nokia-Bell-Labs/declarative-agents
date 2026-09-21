// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// scratchRepo builds a repository with one commit on main, so every case
// drives its own refs and this repository's are never touched.
func scratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "test@example.invalid"},
		{"config", "user.name", "Test"},
		{"commit", "--allow-empty", "-m", "first"},
	} {
		runGitInDir(t, dir, args...)
	}
	return dir
}

// scratchGitOutput returns one git command's output from a scratch
// repository; runGitInDir in tag_test.go covers the fire-and-forget calls.
func scratchGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func refSet(t *testing.T, dir string) string {
	t.Helper()
	return scratchGitOutput(t, dir, "for-each-ref")
}

const goodSummary = "Release from scratch main. Three PRs: the gate, the survey, and the check."

// R1.1: a missing tag and a lightweight tag are refused by name; an
// annotated one passes the rule.
func TestTagVerifyRefusesMissingAndLightweightTags(t *testing.T) {
	dir := scratchRepo(t)
	err := verifyTagAnnotation(dir, "v0.20260920.0")
	if err == nil || !strings.Contains(err.Error(), "does not exist") || !strings.Contains(err.Error(), "R1.1") {
		t.Fatalf("missing tag: %v", err)
	}
	runGitInDir(t, dir, "tag", "v0.20260920.0")
	err = verifyTagAnnotation(dir, "v0.20260920.0")
	if err == nil || !strings.Contains(err.Error(), "lightweight") || !strings.Contains(err.Error(), "R1.1") {
		t.Fatalf("lightweight tag: %v", err)
	}
}

// R2.1 and R2.2: the degenerate shapes, each with its rule named, and a real
// summary passing.
func TestTagVerifyRefusesDegenerateAnnotations(t *testing.T) {
	cases := map[string]struct {
		message string
		rule    string
	}{
		"subject is the tag name": {"v0.20260920.1", "R2.1"},
		"under the bar":           {"fixes", "R2.2"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			dir := scratchRepo(t)
			runGitInDir(t, dir, "tag", "-a", "v0.20260920.1", "-m", testCase.message)
			err := verifyTagAnnotation(dir, "v0.20260920.1")
			if err == nil || !strings.Contains(err.Error(), testCase.rule) {
				t.Fatalf("%s: %v", name, err)
			}
		})
	}
	dir := scratchRepo(t)
	runGitInDir(t, dir, "tag", "-a", "v0.20260920.1", "-m", goodSummary)
	if err := verifyTagAnnotation(dir, "v0.20260920.1"); err != nil {
		t.Fatalf("real summary refused: %v", err)
	}
}

// The bar is stated once: the constant, and every quote of it flows from
// there (srd007 R2.2).
func TestSubstanceBarIsStatedOnce(t *testing.T) {
	content, err := os.ReadFile("tag_verify.go")
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(content), "annotationSubstanceBar ="); count != 1 {
		t.Fatalf("the substance bar is defined %d times", count)
	}
}

// R3.1: a tag off main is refused, and an absent main is a named failure.
func TestTagVerifyRequiresReachabilityFromMain(t *testing.T) {
	dir := scratchRepo(t)
	runGitInDir(t, dir, "checkout", "-b", "side")
	runGitInDir(t, dir, "commit", "--allow-empty", "-m", "side work")
	runGitInDir(t, dir, "tag", "-a", "v0.20260920.2", "-m", goodSummary)
	err := verifyTagAnnotation(dir, "v0.20260920.2")
	if err == nil || !strings.Contains(err.Error(), "not reachable") || !strings.Contains(err.Error(), "R3.1") {
		t.Fatalf("side-branch tag: %v", err)
	}

	dir = scratchRepo(t)
	runGitInDir(t, dir, "tag", "-a", "v0.20260920.3", "-m", goodSummary)
	runGitInDir(t, dir, "checkout", "--detach")
	runGitInDir(t, dir, "branch", "-D", "main")
	err = verifyTagAnnotation(dir, "v0.20260920.3")
	if err == nil || !strings.Contains(err.Error(), "R3.1") {
		t.Fatalf("absent main: %v", err)
	}
}

// R4.1: a run mutates nothing, passing or refusing.
func TestTagVerifyMutatesNothing(t *testing.T) {
	dir := scratchRepo(t)
	runGitInDir(t, dir, "tag", "-a", "v0.20260920.4", "-m", goodSummary)
	runGitInDir(t, dir, "tag", "-a", "v0.20260920.5", "-m", "v0.20260920.5")
	before := refSet(t, dir)
	if err := verifyTagAnnotation(dir, "v0.20260920.4"); err != nil {
		t.Fatal(err)
	}
	if err := verifyTagAnnotation(dir, "v0.20260920.5"); err == nil {
		t.Fatal("degenerate annotation passed")
	}
	if after := refSet(t, dir); after != before {
		t.Fatalf("verification changed the ref set:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if status := scratchGitOutput(t, dir, "status", "--porcelain"); strings.TrimSpace(status) != "" {
		t.Fatalf("verification dirtied the tree: %s", status)
	}
}

// An empty tag argument is refused before any git runs.
func TestTagVerifyNeedsATagName(t *testing.T) {
	if err := verifyTagAnnotation(t.TempDir(), "  "); err == nil {
		t.Fatal("empty tag accepted")
	}
}

func TestSubjectEqualToTagNameInRealHistoryShape(t *testing.T) {
	// The shape the real degenerate releases carry: annotated, subject equal
	// to the tag, empty body. The refusal must name R2.1, not the substance
	// bar, so the operator learns which habit to fix.
	dir := scratchRepo(t)
	runGitInDir(t, dir, "tag", "-a", "v0.20260919.0", "-m", "v0.20260919.0")
	err := verifyTagAnnotation(dir, "v0.20260919.0")
	if err == nil || !strings.Contains(err.Error(), "R2.1") {
		t.Fatalf("real degenerate shape: %v", err)
	}
	if !strings.Contains(err.Error(), "tag name itself") {
		t.Fatalf("refusal does not say what is wrong: %v", err)
	}
}
