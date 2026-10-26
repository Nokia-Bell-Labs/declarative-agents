// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

func newBeadsTracker(t *testing.T, user string) (*beads.Tracker, string) {
	t.Helper()
	root := t.TempDir()
	remote := beadstest.TrackerRepository(t, root)
	clone := filepath.Join(root, user+"-tracker")
	beadstest.Clone(t, remote, clone, user)
	checkout := beadstest.CodeRepository(t, root, "po-1", user)
	return beads.NewTracker(clone, user, checkout), checkout
}

// beadsState reads an issue's srd041 state as the tracker repository holds it.
func beadsState(t *testing.T, tr *beads.Tracker, project string, iid int) beads.Issue {
	t.Helper()
	issues, _, err := tr.Store.Issues()
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range issues {
		if i.Repository == project && i.IID == iid {
			return i
		}
	}
	t.Fatalf("no %s#%d", project, iid)
	return beads.Issue{}
}

// TestFileAndLintOnBeads files issues on the beads tracker (srd013 R3.1):
// admission is the srd041 lint rows there, so an admitted issue is ready, a
// failing one is in needs-triage with its reasons posted once, and fixing
// it and linting again admits it. The sprint milestone is found by date.
func TestFileAndLintOnBeads(t *testing.T) {
	tr, _ := newBeadsTracker(t, "alice")
	if _, err := tr.EnsureMilestone("g", tracker.Milestone{Title: "01.1", StartDate: "2026-11-02", DueDate: "2026-11-13"}); err != nil {
		t.Fatal(err)
	}
	sprint, reason, err := SprintMilestone(tr, "g/po-1", "", time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC), false)
	if err != nil || sprint != "01.1" {
		t.Fatalf("sprint = %q (%s), %v, want 01.1", sprint, reason, err)
	}

	good, err := File(tr, "g/po-1", tracker.NewIssue{Title: "Score one agent", Description: goodBody, Milestone: sprint,
		Labels: Labels("data", "", "build", "po-1")}, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !good.Lint.Admitted {
		t.Fatalf("good issue = %+v, want admitted", good.Lint)
	}
	if got := beadsState(t, tr, "g/po-1", good.IID); got.State != beads.Ready || got.HasLabel("needs-triage") {
		t.Fatalf("admitted issue = %s %v, want ready", got.State, got.Labels)
	}

	bad, err := File(tr, "g/po-1", tracker.NewIssue{Title: "Missing its kind", Description: goodBody, Milestone: sprint,
		Labels: Labels("data", "", "", "po-1")}, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	got := beadsState(t, tr, "g/po-1", bad.IID)
	if bad.Lint.Admitted || got.State != beads.NeedsTriage || !got.HasLabel("needs-triage") || got.StateActor != beads.ActorLint {
		t.Fatalf("failing issue = %+v, state %s by %s %v, want needs-triage by lint", bad.Lint, got.State, got.StateActor, got.Labels)
	}
	issue, _ := tr.GetIssue("g/po-1", bad.IID)
	if again, err := Apply(tr, issue, Lint(issue, Defaults()), Defaults(), false); err != nil || again.Changed {
		t.Fatalf("second lint = %+v, %v, want no change", again, err)
	}
	if notes, _ := tr.ListNotes("g/po-1", bad.IID); len(notes) != 1 || !strings.Contains(notes[0].Body, "kind::") {
		t.Fatalf("notes = %+v, want the reasons posted once", notes)
	}

	issue, err = tr.UpdateIssue("g/po-1", bad.IID, tracker.IssueUpdate{AddLabels: []string{"kind::build"}})
	if err != nil {
		t.Fatal(err)
	}
	if r, err := Apply(tr, issue, Lint(issue, Defaults()), Defaults(), false); err != nil || !r.Admitted || !r.Changed {
		t.Fatalf("lint after the fix = %+v, %v, want admitted and changed", r, err)
	}
	if got := beadsState(t, tr, "g/po-1", bad.IID); got.State != beads.Ready || got.HasLabel("needs-triage") {
		t.Fatalf("fixed issue = %s %v, want ready", got.State, got.Labels)
	}
}

// TestNonDataIssueKeepsItsStateThroughAdmission: only a data issue enters
// the backlog (srd041 R3.1), so a management issue that fails admission
// carries the needs-triage label but stays open, and its author can still
// pop it.
func TestNonDataIssueKeepsItsStateThroughAdmission(t *testing.T) {
	tr, checkout := newBeadsTracker(t, "alice")
	filed, err := File(tr, "g/po-1", tracker.NewIssue{Title: "Move release 01.1", Labels: Labels("management", "", "", "")}, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	got := beadsState(t, tr, "g/po-1", filed.IID)
	if filed.Lint.Admitted || got.State != beads.Open || !got.HasLabel("needs-triage") {
		t.Fatalf("management issue = %+v, state %s %v, want open carrying needs-triage", filed.Lint, got.State, got.Labels)
	}
	if _, err := Pop(tr, ExecGit{}, "g/po-1", filed.IID, checkout, Defaults(), false); err != nil {
		t.Fatalf("pop of the management issue = %v", err)
	}
}
