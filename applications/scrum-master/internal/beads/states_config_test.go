// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// statesDocument renders rows the way control/states.md holds them, so the
// parser is tested against the document shape init writes.
func statesDocument(rows string) []byte {
	return []byte("---\nid: states\ntitle: Issue states\n---\n\n" +
		"<!-- field: version -->\n## Version\n\n1\n\n" +
		"<!-- field: rows -->\n## Rows\n\n" +
		"| from | to | actor | assignee | requires | kind |\n| --- | --- | --- | --- | --- | --- |\n" +
		rows)
}

func TestParseStatesReadsTheDefaultShape(t *testing.T) {
	rows, err := ParseStates(statesDocument(
		"| open | taken | pop | set |  |  |\n" +
			"| any-open | closed | merge |  |  |  |\n" +
			"| any-open | +needs-run | handoff | none | type::recurring |  |\n" +
			"| any-open | closed | track-goals | none |  | sprint-goal |\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(rows))
	}
	if rows[0].Assignee != EffectSet || rows[0].From[0] != Open || rows[0].To != Taken {
		t.Errorf("row 0 = %+v", rows[0])
	}
	if len(rows[1].From) != len(AnyOpen) || rows[1].Assignee != EffectNone {
		t.Errorf("any-open row = %+v", rows[1])
	}
	if rows[2].Requires != "type::recurring" || rows[2].To != "+"+NeedsRun {
		t.Errorf("overlay row = %+v", rows[2])
	}
	if rows[3].Kind != KindSprintGoal {
		t.Errorf("kind row = %+v", rows[3])
	}
}

func TestParseStatesRefusesWhatTheFormatDoesNotList(t *testing.T) {
	for name, row := range map[string]string{
		"unknown from":     "| limbo | taken | pop | set |  |  |\n",
		"unknown to":       "| open | limbo | pop | set |  |  |\n",
		"unknown actor":    "| open | taken | robot | set |  |  |\n",
		"unknown assignee": "| open | taken | pop | steal |  |  |\n",
		"unknown kind":     "| open | taken | pop | set |  | epic |\n",
		"bare overlay":     "| open | + | person | none |  |  |\n",
	} {
		if _, err := ParseStates(statesDocument(row)); err == nil {
			t.Errorf("%s: not refused", name)
		}
	}
	if _, err := ParseStates(statesDocument("")); err == nil {
		t.Error("a document with no rows: not refused")
	}
}

// A program's own overlay row fires, and a default-table row absent from
// the program's control/states.md is refused: the file replaces the table,
// it does not extend it (GH-130; srd041 R8).
func TestProgramStatesRowsReplaceTheDefault(t *testing.T) {
	s := NewStore(t.TempDir(), "casey")
	s.LoadStatesFrom(func() ([]Row, error) {
		return ParseStates(statesDocument(
			"| any-open | +urgent | person | none |  |  |\n" +
				"| any-open | -urgent | person | none |  |  |\n"))
	})
	i := Issue{ID: "scrum-master-1", State: Open}
	if err := s.transition(&i, ActorPerson, "+urgent", "casey"); err != nil {
		t.Fatalf("the program's own overlay row: %v", err)
	}
	if !i.HasLabel("urgent") {
		t.Fatalf("labels = %v, want urgent", i.Labels)
	}
	if err := s.transition(&i, ActorPop, Taken, "casey"); err == nil {
		t.Fatal("a default row absent from the program's file must be refused")
	}
	if err := s.transition(&i, ActorPerson, "-urgent", "casey"); err != nil || i.HasLabel("urgent") {
		t.Fatalf("clearing the overlay: %v, labels %v", err, i.Labels)
	}
}

// A control/states.md that does not parse refuses every transition with its
// reason rather than silently falling back to the default.
func TestMalformedStatesRefuseTransitions(t *testing.T) {
	s := NewStore(t.TempDir(), "casey")
	s.LoadStatesFrom(func() ([]Row, error) {
		return ParseStates(statesDocument("| open | taken | robot | set |  |  |\n"))
	})
	i := Issue{ID: "scrum-master-1", State: Open}
	err := s.transition(&i, ActorPop, Taken, "casey")
	if err == nil || !strings.Contains(err.Error(), StatesFile) {
		t.Fatalf("err = %v, want the file named", err)
	}
}

// Without a loader, or with one that finds no file, the embedded default
// table is in force.
func TestAbsentStatesFileKeepsTheDefault(t *testing.T) {
	for name, s := range map[string]*Store{
		"no loader": NewStore(t.TempDir(), "casey"),
		"no file": func() *Store {
			s := NewStore(t.TempDir(), "casey")
			s.LoadStatesFrom(func() ([]Row, error) { return nil, nil })
			return s
		}(),
	} {
		i := Issue{ID: "scrum-master-1", State: Open}
		if err := s.transition(&i, ActorPop, Taken, "casey"); err != nil {
			t.Errorf("%s: default table should let pop take an open issue: %v", name, err)
		}
	}
}

// The tracker reads the checkout's control/states.md from main on the first
// transition, so a program's own row fires end to end (GH-130; srd041 R8).
func TestTrackerLoadsStatesFromTheCheckout(t *testing.T) {
	root := t.TempDir()
	remote := beadstest.TrackerRepository(t, root)
	clone := filepath.Join(root, "alice-tracker")
	beadstest.Clone(t, remote, clone, "alice")
	code := beadstest.CodeRepository(t, root, "po-1", "alice")
	doc := statesDocument(
		"| open | taken | pop | set |  |  |\n" +
			"| any-open | +urgent | person | none |  |  |\n")
	if err := os.MkdirAll(filepath.Join(code, "control"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(code, StatesFile), doc, 0o644); err != nil {
		t.Fatal(err)
	}
	beadstest.Git(t, code, "add", "-A")
	beadstest.Git(t, code, "commit", "-q", "-m", "states")
	beadstest.Git(t, code, "push", "-q", "origin", "main")

	tr := NewTracker(clone, "alice", code)
	created, err := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "one"})
	if err != nil {
		t.Fatal(err)
	}
	i, ok, err := tr.Store.Find("g/po-1", created.IID)
	if err != nil || !ok {
		t.Fatalf("find: %v %v", ok, err)
	}
	if err := tr.Store.Fire(i.ID, ActorPerson, "+urgent"); err != nil {
		t.Fatalf("the program's own row must fire: %v", err)
	}
}

// A person's close may carry its reason, recorded as close_reason with the
// close time, as a merge's close records its commit (GH-20).
func TestCloseRecordsItsReason(t *testing.T) {
	tr := newTestTracker(t, "alice")
	created, err := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "one"})
	if err != nil {
		t.Fatal(err)
	}
	closeState, reason := "close", "closed by scrum-master issue recur without its work"
	if _, err := tr.UpdateIssue("g/po-1", created.IID, tracker.IssueUpdate{State: &closeState, CloseReason: &reason}); err != nil {
		t.Fatal(err)
	}
	i, ok, err := tr.Store.Find("g/po-1", created.IID)
	if err != nil || !ok {
		t.Fatalf("find: %v %v", ok, err)
	}
	if i.State != Closed || i.CloseReason != reason || i.ClosedAt == "" {
		t.Fatalf("closed issue = state %s reason %q at %q", i.State, i.CloseReason, i.ClosedAt)
	}
}
