// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// pushFixture is a repository holding a story, with a local bare repository as the
// origin its checkout pushes to, written as a URL naming project g/po-1.
func pushFixture(t *testing.T) (dir, bare, story string, f *fake.Fake) {
	t.Helper()
	root := t.TempDir()
	bare = filepath.Join(root, "origin.git")
	dir = filepath.Join(root, "po-1")
	run := func(d string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", d}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	if out, err := exec.Command("git", "init", "-q", "--bare", "-b", "main", bare).CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
	if err := os.MkdirAll(filepath.Join(dir, "docs", "specs", "stories"), 0o755); err != nil {
		t.Fatal(err)
	}
	run(dir, "init", "-q", "-b", "main")
	run(dir, "config", "user.email", "t@example.com")
	run(dir, "config", "user.name", "t")
	cfg := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[url \""+bare+"\"]\n\tinsteadOf = https://gitlab.example.com/g/po-1.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	run(dir, "remote", "add", "origin", "https://gitlab.example.com/g/po-1.git")
	story = filepath.Join(dir, "docs", "specs", "stories", "st001-a.md")
	if err := os.WriteFile(story, []byte("---\nid: st001-a\ntitle: A\nstatus: agreed\n---\n\n<!-- field: context | form: \"tree\" -->\n## Context\n\n- owner: po\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(dir, "add", "-A")
	run(dir, "commit", "-q", "-m", "start")
	run(dir, "push", "-q", "-u", "origin", "main")
	f = fake.New()
	f.Issues["g/po-1"] = []*tracker.Issue{{Project: "g/po-1", IID: 1, Title: "Closed by a merge", State: "closed", ClosedAt: "2026-11-02T10:00:00Z", Description: "Implements: st001-a\n"}}
	return dir, bare, story, f
}

func originHolds(t *testing.T, bare, want string) bool {
	t.Helper()
	out, err := exec.Command("git", "-C", bare, "show", "main:docs/specs/stories/st001-a.md").CombinedOutput()
	if err != nil {
		t.Fatalf("%s", out)
	}
	return strings.Contains(string(out), want)
}

// TestRecordAndPush: the record reaches origin, so a merge anyone runs keeps the
// backlog current; a second run changes nothing.
func TestRecordAndPush(t *testing.T) {
	dir, bare, _, f := pushFixture(t)
	r, err := RecordAndPush(f, dir, "", false)
	if err != nil || !r.Pushed || r.Commit == "" {
		t.Fatalf("record = %+v, %v", r, err)
	}
	if !originHolds(t, bare, "g/po-1#1") || !originHolds(t, bare, "state: closed") {
		t.Fatal("origin holds no record")
	}
	if r, err = RecordAndPush(f, dir, "", false); err != nil || r.Pushed || r.Commit != "" {
		t.Fatalf("second run = %+v, %v", r, err)
	}
}

// TestRecordAndPushLandsFromAWorktree: the merge profile runs in the issue's
// worktree, on its branch; the record goes on the main checkout's default branch.
func TestRecordAndPushLandsFromAWorktree(t *testing.T) {
	dir, bare, _, f := pushFixture(t)
	wt := filepath.Join(filepath.Dir(dir), "wt")
	if out, err := exec.Command("git", "-C", dir, "worktree", "add", "-q", "-b", "1-work", wt).CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
	r, err := RecordAndPush(f, wt, "", false)
	if err != nil || !r.Pushed {
		t.Fatalf("record = %+v, %v", r, err)
	}
	if !originHolds(t, bare, "g/po-1#1") {
		t.Fatal("origin holds no record")
	}
}

// TestRecordAndPushLeavesAPersonsWorkAlone: changes and another branch are theirs.
func TestRecordAndPushLeavesAPersonsWorkAlone(t *testing.T) {
	dir, bare, story, f := pushFixture(t)
	if err := os.WriteFile(filepath.Join(dir, "scratch.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := RecordAndPush(f, dir, "", false)
	if err != nil || len(r.Notes) == 0 || !strings.Contains(strings.Join(r.Notes, " "), "holds changes") {
		t.Fatalf("with changes = %+v, %v", r, err)
	}
	os.Remove(filepath.Join(dir, "scratch.txt"))
	if out, err := exec.Command("git", "-C", dir, "checkout", "-q", "-b", "other").CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
	r, err = RecordAndPush(f, dir, "", false)
	if err != nil || !strings.Contains(strings.Join(r.Notes, " "), "on other") {
		t.Fatalf("on another branch = %+v, %v", r, err)
	}
	if b, _ := os.ReadFile(story); strings.Contains(string(b), "g/po-1#1") {
		t.Fatal("wrote the record anyway")
	}
	if originHolds(t, bare, "g/po-1#1") {
		t.Fatal("pushed anyway")
	}
}

// TestRecordAndPushDropsARefusedPush: a host refusing the push, as a protected
// branch does, is a note, and the checkout stays level with origin.
func TestRecordAndPushDropsARefusedPush(t *testing.T) {
	dir, bare, _, f := pushFixture(t)
	hook := filepath.Join(bare, "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho protected >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := RecordAndPush(f, dir, "", false)
	if err != nil || r.Pushed || r.Commit != "" || !strings.Contains(strings.Join(r.Notes, " "), "not pushed") {
		t.Fatalf("record = %+v, %v", r, err)
	}
	head, _ := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	origin, _ := exec.Command("git", "-C", dir, "rev-parse", "origin/main").Output()
	if string(head) != string(origin) {
		t.Fatalf("the checkout is not level with origin: %s %s", head, origin)
	}
	if status, _ := exec.Command("git", "-C", dir, "status", "--porcelain").Output(); len(status) != 0 {
		t.Fatalf("left changes: %s", status)
	}
}

// TestRecordProgramLeavesOutWhatItCannotRead: a team with no checkout is a note.
func TestRecordProgramLeavesOutWhatItCannotRead(t *testing.T) {
	system, _, _ := recordProgram(t)
	f := fake.New()
	if err := os.RemoveAll(filepath.Join(filepath.Dir(system), "po-1")); err != nil {
		t.Fatal(err)
	}
	results, notes, err := RecordProgram(f, system, true)
	if err != nil || len(results) == 0 {
		t.Fatalf("results = %+v, notes %v, %v", results, notes, err)
	}
	if !strings.Contains(strings.Join(notes, " "), "no checkout") {
		t.Fatalf("notes = %v", notes)
	}
}
