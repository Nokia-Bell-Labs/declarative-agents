// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const srdUntraced = `---
id: srd010-delivery
title: "Subsystem: delivery"
---

<!-- req: R1 -->
## R1 - Fleet

<!-- req: R1.1 -->
### R1.1

The fleet runs promoted agents.

<!-- req: R1.2 -->
### R1.2

The fleet reports its health.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

A promoted agent runs.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
`

// TestCheckReadsSRDs counts the SRDs as documents and refuses a
// requirement item no acceptance criterion traces (#149).
func TestCheckReadsSRDs(t *testing.T) {
	stubSpec(t, nil, errors.New("no pinned release"))
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, SRDDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, SRDDir, "srd010-delivery.md"), []byte(srdUntraced), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Documents != 1 || len(r.Findings) != 1 || r.Findings[0].Field != "R1.2" ||
		r.Findings[0].File != filepath.Join(SRDDir, "srd010-delivery.md") {
		t.Fatalf("report = %+v, want one SRD and R1.2 untraced", r)
	}
	if r.SpecSkipped != "no pinned release" {
		t.Fatalf("spec check skipped = %q", r.SpecSkipped)
	}
}

// TestCheckRunsTheSpecToolsShapeCheck reports the parent spec tool's shape
// findings, such as a VISION field holding the wrong form, and leaves its
// formatting findings to scrum-master doctor (#149).
func TestCheckRunsTheSpecToolsShapeCheck(t *testing.T) {
	stubSpec(t, []string{
		"docs/VISION.md: field-shape-mismatch: what_this_does holds a list, want prose",
		"docs/ARCHITECTURE.md: not-canonical: run mage fmt",
	}, errors.New("exit status 1"))
	r, err := Check(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Findings) != 1 || r.Findings[0].File != "docs/VISION.md" || r.Findings[0].Field != "field-shape-mismatch" ||
		!strings.Contains(r.Findings[0].Message, "what_this_does") || r.SpecSkipped != "" {
		t.Fatalf("report = %+v, want the VISION shape finding alone", r)
	}
}

func stubSpec(t *testing.T, lines []string, err error) {
	prev := specCheck
	specCheck = func(string, string, string) ([]string, error) { return lines, err }
	t.Cleanup(func() { specCheck = prev })
}

// TestBacklogCheckNamesUntrackedDocuments is srd045 R8.2 for a document git
// does not track: the shape check cannot read it, so the gate names it
// until it is staged.
func TestBacklogCheckNamesUntrackedDocuments(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	epic := filepath.Join(dir, "docs", "specs", "epics", "ep001-scoring.md")
	if err := os.MkdirAll(filepath.Dir(epic), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(epic, []byte("not an epic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if f := untracked(dir); len(f) != 1 || f[0].File != epic || f[0].Field != "untracked" {
		t.Fatalf("untracked epic = %+v, want one finding naming it", f)
	}
	git("add", "-A")
	if f := untracked(dir); len(f) != 0 {
		t.Fatalf("a staged epic = %+v, want none", f)
	}
	if f := untracked(t.TempDir()); f != nil {
		t.Fatalf("a directory git does not manage = %+v, want none", f)
	}
}

// TestCheckReportsAnUnparsableDocument is srd045 R8.2 for a document the
// spec tool cannot parse: it is a finding naming the document and the
// cause, whether the tool reports it as a line or aborts naming it, and only
// a check that could not run at all is a skip (#188).
func TestCheckReportsAnUnparsableDocument(t *testing.T) {
	for name, stub := range map[string]struct {
		lines []string
		err   error
	}{
		"a parse line": {[]string{"docs/specs/epics/ep005-x.md: parse: field context: text line \"  2. ep003-model-gateway\" is not indented 5 spaces"}, errors.New("exit status 1")},
		"an abort":     {nil, errors.New("spectool: docs/specs/epics/ep005-x.md: field context: text line \"  2. ep003-model-gateway\" is not indented 5 spaces")},
	} {
		t.Run(name, func(t *testing.T) {
			stubSpec(t, stub.lines, stub.err)
			r, err := Check(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Findings) != 1 || r.Findings[0].File != "docs/specs/epics/ep005-x.md" || r.Findings[0].Field != "parse" ||
				!strings.Contains(r.Findings[0].Message, "not indented 5 spaces") || r.SpecSkipped != "" {
				t.Fatalf("report = %+v, want one parse finding and no skip", r)
			}
		})
	}
	stubSpec(t, nil, errors.New("docs/external-pins.yaml pins no release of the parent"))
	r, err := Check(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Findings) != 0 || r.SpecSkipped == "" {
		t.Fatalf("a check that could not run = %+v, want a skip and no finding", r)
	}
}

// TestRoadMapReleaseStatuses holds the gate to the release statuses design.md
// names: specified, implemented, and released pass, any other word is a
// finding naming the release (srd045 R8.2, #188).
func TestRoadMapReleaseStatuses(t *testing.T) {
	stubSpec(t, nil, nil)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	roadMap := "---\nid: road-map\ntitle: Road map\n---\n\n" +
		"<!-- release: 01.0 | status: released -->\n## 01.0 - Release 01.0\n\nCut.\n\n" +
		"<!-- release: 01.1 | status: done -->\n## 01.1 - Release 01.1\n\nPlanned.\n"
	if err := os.WriteFile(filepath.Join(dir, "docs", "road-map.md"), []byte(roadMap), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Findings) != 1 || r.Findings[0].Field != "release 01.1 status" || !strings.Contains(r.Findings[0].Message, `"done"`) {
		t.Fatalf("report = %+v, want one finding on release 01.1's status", r)
	}
}

// TestCheckCountsVisionAndArchitectureAndNamesUnfilled counts VISION and
// ARCHITECTURE, whose shape the gate checks, among its documents, and names
// a document that still holds init's placeholder text without failing it.
func TestCheckCountsVisionAndArchitectureAndNamesUnfilled(t *testing.T) {
	stubSpec(t, nil, errors.New("no pinned release"))
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"VISION.md":       "The Product Owner writes here what the program is.\n",
		"ARCHITECTURE.md": "The gateway routes every request.\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, "docs", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Documents != 2 || len(r.Findings) != 0 || !slices.Equal(r.Unfilled, []string{filepath.Join("docs", "VISION.md")}) {
		t.Fatalf("report = %+v, want two documents, no finding, VISION unfilled", r)
	}
}

// The format's refusal of an item with nothing under it says to leave the
// item out (GH-63).
func TestEmptyItemHint(t *testing.T) {
	if got := emptyItemHint("field context: an empty tree block"); !strings.Contains(got, "is left out") {
		t.Fatalf("hint %q", got)
	}
	if got := emptyItemHint("field goals: missing"); got != "field goals: missing" {
		t.Fatalf("another finding changed: %q", got)
	}
}
