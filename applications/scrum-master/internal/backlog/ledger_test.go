// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// A story's issues record round-trips (srd045 R4.4): writing appends the
// field to a story without one, reading gives the entries back in ref
// order, writing the same entries twice writes the same bytes, a changed
// entry rewrites the field in place keeping what follows, and writing no
// entries removes the field.
func TestLedgerRoundTrip(t *testing.T) {
	doc := []byte("---\nid: st001-x\ntitle: X\n---\n\n<!-- field: problem -->\n## Problem\n\nP.\n")
	entries := []LedgerEntry{
		{Ref: "g/code-1#3", URL: "https://gitlab.example.com/g/code-1/-/issues/3", Title: "Wire it", Who: []string{"bo"}, Opened: "2026-11-01", State: "open"},
		{Ref: "g/po-1#7", URL: "https://gitlab.example.com/g/po-1/-/issues/7", Title: "Define it", Who: []string{"ana", "me"}, Opened: "2026-10-28", Closed: "2026-10-30", State: "closed"},
	}
	one := WriteLedger(doc, entries)
	if !bytes.Contains(one, []byte(`<!-- field: issues | form: "tree" -->`)) || !bytes.Contains(one, []byte("## Issues")) {
		t.Fatalf("the field was not appended:\n%s", one)
	}
	got, err := LedgerOf(one)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Ref != "g/code-1#3" || got[1].Ref != "g/po-1#7" {
		t.Fatalf("read back %+v", got)
	}
	if got[1].Closed != "2026-10-30" || got[1].State != "closed" || strings.Join(got[1].Who, "|") != "ana|me" || got[0].URL != entries[0].URL || got[0].Title != "Wire it" {
		t.Errorf("entries lost detail: %+v", got)
	}
	if two := WriteLedger(one, got); !bytes.Equal(one, two) {
		t.Errorf("writing the same entries changed the bytes:\n%s\n----\n%s", one, two)
	}
	// The field rewrites in place, keeping the section after it.
	mid := append(append([]byte{}, one...), []byte("\n<!-- field: non_goals -->\n## Non-goals\n\n- Nothing.\n")...)
	entries[0].Closed, entries[0].State = "2026-11-03", "closed"
	three := WriteLedger(mid, entries)
	if !bytes.Contains(three, []byte(`closed: "2026-11-03"`)) || !bytes.Contains(three, []byte("## Non-goals")) {
		t.Errorf("the rewrite lost the update or what follows:\n%s", three)
	}
	if i, j := bytes.Index(three, []byte("## Issues")), bytes.Index(three, []byte("## Non-goals")); i < 0 || j < i {
		t.Errorf("the field moved: issues at %d, non-goals at %d", i, j)
	}
	// No entries removes the field.
	if none := WriteLedger(three, nil); bytes.Contains(none, []byte("field: issues")) || !bytes.Contains(none, []byte("## Non-goals")) {
		t.Errorf("an empty record should remove the field and keep the rest:\n%s", none)
	}
}

// An entry reads who from the issue's assignees, else its author; keeps
// days, not instants; and a tracker's "opened" state reads as open while
// a closed date makes it closed (srd045 R4.4).
func TestEntryForNormalizes(t *testing.T) {
	e := EntryFor("g/po-1", tracker.Issue{IID: 7, Title: "T", State: "opened", Author: "ana", CreatedAt: "2026-11-01T10:11:12Z"})
	if e.Ref != "g/po-1#7" || e.State != "open" || e.Opened != "2026-11-01" || strings.Join(e.Who, ",") != "ana" {
		t.Errorf("open entry = %+v", e)
	}
	e = EntryFor("g/po-1", tracker.Issue{IID: 8, State: "opened", Assignees: []string{"bo"}, Author: "ana", CreatedAt: "2026-11-01T10:11:12Z", ClosedAt: "2026-11-03T09:00:00Z"})
	if e.State != "closed" || e.Closed != "2026-11-03" || strings.Join(e.Who, ",") != "bo" {
		t.Errorf("closed entry = %+v", e)
	}
}

// recordProgram lays a system checkout beside the scrum repository's and
// a code repository's: the system's trackers document and the subsystem
// SRD that names g/po-1 the scrum repository and g/code-1 its component's.
func recordProgram(t *testing.T) (system, scrum, code string) {
	t.Helper()
	root := t.TempDir()
	system, scrum, code = filepath.Join(root, "system"), filepath.Join(root, "po-1"), filepath.Join(root, "code-1")
	write := func(dir, p string, b []byte) {
		t.Helper()
		os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, p), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(system, "control/trackers.md", []byte("---\nid: trackers\ntitle: Tracking systems\n---\n\n<!-- field: version -->\n## Version\n\n1\n\n<!-- field: default -->\n## Default\n\n<!-- field: system -->\n### System\n\nbd\n"))
	write(system, "docs/specs/software-requirements/srd010-po-1.md", []byte(`---
id: srd010-po-1
title: "Subsystem: srd010-po-1"
---

<!-- field: context | form: "tree" -->
## Context

- owner: po-1
- scrum_repository: https://gitlab.example.com/g/po-1

<!-- req: R1 -->
## R1 - Work

<!-- req: R1.1 -->
### R1.1

Work arrives.

<!-- field: implementation_targets -->
## Implementation targets

| name | repository | path |
| --- | --- | --- |
| widget | https://gitlab.example.com/g/code-1 | / |
`))
	write(scrum, "docs/specs/stories/st001-score-trajectories.md", teamStory(t))
	write(code, "README.md", []byte("code\n"))
	for dir, origin := range map[string]string{system: "https://gitlab.example.com/g/system.git", scrum: "https://gitlab.example.com/g/po-1.git", code: "https://gitlab.example.com/g/code-1.git"} {
		for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"remote", "add", "origin", origin}, {"add", "-A"}, {"-c", "user.email=t@e", "-c", "user.name=t", "commit", "-q", "-m", "seed"}} {
			if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v: %s", args, err, out)
			}
		}
	}
	return system, scrum, code
}

// record sweeps the team's repositories for issues implementing its
// stories — a bare id in the scrum repository, a pinned URL in a code
// repository — writes each story's issues record, keeps an entry it
// cannot judge, and is idempotent; run from the code repository it finds
// the scrum repository through the registry; --commit records the change;
// a dry run writes nothing (srd045 R4.4).
func TestRecordWritesTheStoriesRecord(t *testing.T) {
	_, scrum, code := recordProgram(t)
	f := fake.New()
	f.Issues["g/po-1"] = []*tracker.Issue{{Project: "g/po-1", IID: 7, Title: "Define scoring", State: "closed", WebURL: "https://gitlab.example.com/g/po-1/-/issues/7",
		Description: "Implements: st001-score-trajectories", Assignees: []string{"ana"}, CreatedAt: "2026-10-28T08:00:00Z", ClosedAt: "2026-10-30T17:00:00Z"}}
	f.Issues["g/code-1"] = []*tracker.Issue{{Project: "g/code-1", IID: 3, Title: "Wire the scorer", State: "opened", WebURL: "https://gitlab.example.com/g/code-1/-/issues/3",
		Description: "Implements: https://gitlab.example.com/g/po-1/-/blob/main/docs/specs/stories/st001-score-trajectories.md", Author: "bo", CreatedAt: "2026-11-01T08:00:00Z"}}
	// An entry in a project outside the sweep stays as written.
	p := filepath.Join(scrum, "docs", "specs", "stories", "st001-score-trajectories.md")
	doc, _ := os.ReadFile(p)
	os.WriteFile(p, WriteLedger(doc, []LedgerEntry{{Ref: "other/repo#9", State: "open"}}), 0o644)

	r, err := Record(f, code, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Scrum != "g/po-1" || len(r.Stories) != 1 || r.Stories[0].Action != "recorded" {
		t.Fatalf("record = %+v", r)
	}
	got, _ := os.ReadFile(p)
	entries, err := LedgerOf(got)
	if err != nil {
		t.Fatal(err)
	}
	refs := make([]string, len(entries))
	for i, e := range entries {
		refs[i] = e.Ref
	}
	if strings.Join(refs, " ") != "g/code-1#3 g/po-1#7 other/repo#9" {
		t.Fatalf("refs = %v\n%s", refs, got)
	}
	again, err := Record(f, scrum, "", false, false)
	if err != nil || again.Stories[0].Action != "unchanged" {
		t.Fatalf("a second record should change nothing: %+v, %v", again.Stories, err)
	}
	// The close reaches the record, and --commit records it.
	f.Issues["g/code-1"][0].State, f.Issues["g/code-1"][0].ClosedAt = "closed", "2026-11-04T12:00:00Z"
	dry, err := Record(f, scrum, "", false, true)
	if err != nil || dry.Stories[0].Action != "recorded" {
		t.Fatalf("dry run = %+v, %v", dry.Stories, err)
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, got) {
		t.Fatal("the dry run wrote")
	}
	committed, err := Record(f, scrum, "", true, false)
	if err != nil || committed.Commit == "" {
		t.Fatalf("commit = %+v, %v", committed, err)
	}
	if out, _ := exec.Command("git", "-C", scrum, "log", "-1", "--format=%s").Output(); !strings.Contains(string(out), "The stories record their issues") {
		t.Errorf("commit subject = %q", out)
	}
	if b, _ := os.ReadFile(p); !bytes.Contains(b, []byte(`closed: "2026-11-04"`)) {
		t.Errorf("the close did not reach the record:\n%s", b)
	}
}
