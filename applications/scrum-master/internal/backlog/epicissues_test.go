// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

func epicDoc(status string) string {
	return "---\nid: ep001-a\ntitle: The epic\nstatus: " + status + "\n---\n\n<!-- field: context | form: \"tree\" -->\n## Context\n\n- owner: technical program manager\n- horizon: 2026-W43..2026-W51\n"
}

func epicCheckout(t *testing.T) (string, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "system")
	for _, d := range []string{"control", "docs/specs/epics"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "control", "trackers.md"), []byte("---\nid: trackers\ntitle: Tracking systems\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "docs", "specs", "epics", "ep001-a.md")
	if err := os.WriteFile(file, []byte(epicDoc("agreed")), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "t"},
		{"remote", "add", "origin", "https://gitlab.example.com/g/system.git"}, {"add", "-A"}, {"commit", "-q", "-m", "start"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	return dir, file
}

func writePeople(t *testing.T, dir, rows string) {
	t.Helper()
	doc := "---\nid: people\ntitle: People\n---\n\n<!-- field: people -->\n## People\n\n| username | epics |\n| --- | --- |\n" + rows
	if err := os.WriteFile(filepath.Join(dir, "control", "people.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileEpicIssues(t *testing.T) {
	dir, file := epicCheckout(t)
	writePeople(t, dir, "| alice | ep001-a |\n| bob | ep001-a |\n")
	f := fake.New()

	writes := f.Writes
	r, err := ReconcileEpicIssues(f, dir, false, true)
	if err != nil || len(r.Epics) != 1 || r.Epics[0].Action != "filed" || f.Writes != writes {
		t.Fatalf("dry run = %+v, %v, writes %d", r, err, f.Writes-writes)
	}
	if r, err = ReconcileEpicIssues(f, dir, false, false); err != nil || r.Epics[0].Action != "filed" || r.Epics[0].Issue == "" {
		t.Fatalf("first run = %+v, %v", r, err)
	}
	i, err := f.GetIssue("g/system", 1)
	if err != nil || !i.HasLabel("type::tracking") || !i.HasLabel("plane::management") || !slices.Equal(i.Assignees, []string{"alice", "bob"}) ||
		!strings.Contains(i.Description, "alice, bob") || !strings.Contains(i.Description, "docs/specs/epics/ep001-a.md") {
		t.Fatalf("the issue = %+v, %v", i, err)
	}
	b, _ := os.ReadFile(file)
	if !strings.Contains(string(b), "- issue: g/system#1\n") {
		t.Fatalf("the document:\n%s", b)
	}
	d, err := hierarchy.Parse(file, b)
	if err != nil || d.Issue != "g/system#1" || d.Owner == "" {
		t.Fatalf("parsed = %+v, %v", d, err)
	}

	writes = f.Writes
	if r, err = ReconcileEpicIssues(f, dir, false, false); err != nil || r.Epics[0].Action != "unchanged" || f.Writes != writes {
		t.Fatalf("a second run = %+v, %v, writes %d", r, err, f.Writes-writes)
	}

	writePeople(t, dir, "| carol | ep001-a |\n")
	if r, err = ReconcileEpicIssues(f, dir, false, false); err != nil || r.Epics[0].Action != "updated" {
		t.Fatalf("after people changed = %+v, %v", r, err)
	}
	if i, _ = f.GetIssue("g/system", 1); !slices.Equal(i.Assignees, []string{"carol"}) || !strings.Contains(i.Description, "carol") || strings.Contains(i.Description, "alice") {
		t.Fatalf("the issue after people changed = %+v", i)
	}

	retired := strings.Replace(string(b), "status: agreed", "status: retired", 1)
	if err := os.WriteFile(file, []byte(retired), 0o644); err != nil {
		t.Fatal(err)
	}
	if r, err = ReconcileEpicIssues(f, dir, false, false); err != nil || r.Epics[0].Action != "closed" {
		t.Fatalf("retired = %+v, %v", r, err)
	}
	if i, _ = f.GetIssue("g/system", 1); i.State != "closed" {
		t.Fatalf("a retired epic's issue = %+v", i)
	}
	if r, err = ReconcileEpicIssues(f, dir, false, false); err != nil || r.Epics[0].Action != "unchanged" {
		t.Fatalf("after closing = %+v, %v", r, err)
	}
}

func TestAnEpicNotAgreedGetsNoIssue(t *testing.T) {
	dir, file := epicCheckout(t)
	if err := os.WriteFile(file, []byte(epicDoc("to_be_agreed")), 0o644); err != nil {
		t.Fatal(err)
	}
	f := fake.New()
	r, err := ReconcileEpicIssues(f, dir, false, false)
	if err != nil || r.Epics[0].Action != "none" || len(f.Issues["g/system"]) != 0 {
		t.Fatalf("reconcile = %+v, %v", r, err)
	}
}

// TestReconcileFindsAnIssueAnEarlierRunFiled: a run that stopped before writing the
// document files no second issue.
func TestReconcileFindsAnIssueAnEarlierRunFiled(t *testing.T) {
	dir, file := epicCheckout(t)
	f := fake.New()
	if _, err := f.CreateIssue("g/system", tracker.NewIssue{Title: "Epic: The epic", Description: "x\n" + epicMarker("ep001-a"), Labels: []string{"type::tracking"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcileEpicIssues(f, dir, false, false); err != nil {
		t.Fatal(err)
	}
	if n := len(f.Issues["g/system"]); n != 1 {
		t.Fatalf("%d issues", n)
	}
	if b, _ := os.ReadFile(file); !strings.Contains(string(b), "- issue: g/system#1\n") {
		t.Fatalf("the document:\n%s", b)
	}
}

func TestReconcileCommits(t *testing.T) {
	dir, _ := epicCheckout(t)
	r, err := ReconcileEpicIssues(fake.New(), dir, true, false)
	if err != nil || r.Commit == "" {
		t.Fatalf("reconcile = %+v, %v", r, err)
	}
	if out, _ := exec.Command("git", "-C", dir, "status", "--short").Output(); len(out) != 0 {
		t.Fatalf("left uncommitted: %s", out)
	}
}

func TestReconcileRefusesAMalformedIssueItem(t *testing.T) {
	dir, file := epicCheckout(t)
	if err := os.WriteFile(file, []byte(epicDoc("agreed")+"- issue: not-a-ref\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReconcileEpicIssues(fake.New(), dir, false, false); err == nil || !strings.Contains(err.Error(), "not-a-ref") {
		t.Fatalf("reconcile = %v", err)
	}
}

// TestReconcileOnBeads: beads holds one assignee, so the mirror names its people in
// its body and has none, and it closes through the state table.
func TestReconcileOnBeads(t *testing.T) {
	dir, file := epicCheckout(t)
	writePeople(t, dir, "| alice | ep001-a |\n| bob | ep001-a |\n")
	root := t.TempDir()
	clone := filepath.Join(root, "alice-tracker")
	beadstest.Clone(t, beadstest.TrackerRepository(t, root), clone, "alice")
	tr := beads.NewTracker(clone, "alice", beadstest.CodeRepository(t, root, "po-1", "alice"))

	r, err := ReconcileEpicIssues(tr, dir, false, false)
	if err != nil || r.Epics[0].Action != "filed" {
		t.Fatalf("first run = %+v, %v", r, err)
	}
	i, err := tr.GetIssue("g/system", 1)
	if err != nil || len(i.Assignees) != 0 || !strings.Contains(i.Description, "alice, bob") || !i.HasLabel("type::tracking") {
		t.Fatalf("the issue = %+v, %v", i, err)
	}
	b, _ := os.ReadFile(file)
	if strings.Contains(string(b), "issue_url") {
		t.Fatalf("a beads reference is no URL:\n%s", b)
	}
	if r, err = ReconcileEpicIssues(tr, dir, false, false); err != nil || r.Epics[0].Action != "unchanged" {
		t.Fatalf("a second run = %+v, %v", r, err)
	}
	if err := os.WriteFile(file, []byte(strings.Replace(string(b), "status: agreed", "status: retired", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if r, err = ReconcileEpicIssues(tr, dir, false, false); err != nil || r.Epics[0].Action != "closed" {
		t.Fatalf("retired = %+v, %v", r, err)
	}
	if i, _ = tr.GetIssue("g/system", 1); i.State != "closed" {
		t.Fatalf("a retired epic's issue = %+v", i)
	}
}

// TestReconcileIgnoresAForgesTrimming: GitLab drops a body's last newline, which is
// no change of ours.
func TestReconcileIgnoresAForgesTrimming(t *testing.T) {
	dir, _ := epicCheckout(t)
	f := fake.New()
	if _, err := ReconcileEpicIssues(f, dir, false, false); err != nil {
		t.Fatal(err)
	}
	f.Issues["g/system"][0].Description = strings.TrimRight(f.Issues["g/system"][0].Description, "\n")
	writes := f.Writes
	r, err := ReconcileEpicIssues(f, dir, false, false)
	if err != nil || r.Epics[0].Action != "unchanged" || f.Writes != writes {
		t.Fatalf("second run = %+v, %v, writes %d", r, err, f.Writes-writes)
	}
}
