// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

func newTestTracker(t *testing.T, user string) *Tracker {
	t.Helper()
	root := t.TempDir()
	remote := beadstest.TrackerRepository(t, root)
	clone := filepath.Join(root, user+"-tracker")
	beadstest.Clone(t, remote, clone, user)
	return NewTracker(clone, user, beadstest.CodeRepository(t, root, "po-1", user))
}

// TestTrackerMapsIssuesAndNotes holds the issue half of the adapter to the
// srd042 R2 mapping: a created issue reads back with its fields, queries
// filter by repository, state, label, and search, notes are comments, a
// person's close and reopen are srd041 rows, and the status label fires the
// assignee's in-progress transition.
func TestTrackerMapsIssuesAndNotes(t *testing.T) {
	tr := newTestTracker(t, "alice")
	a, err := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "first", Description: "body", Labels: []string{"plane::data"},
		DueDate: "2026-11-27", Milestone: "01.1"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "second"})
	other, _ := tr.CreateIssue("g/po-2", tracker.NewIssue{Title: "elsewhere"})
	if a.IID != 1 || b.IID != 2 || other.IID != 1 {
		t.Fatalf("iids = %d, %d, %d, want 1, 2 per repository and 1 in the other", a.IID, b.IID, other.IID)
	}

	got, err := tr.GetIssue("g/po-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "first" || got.Description != "body" || got.State != "opened" || got.Milestone != "01.1" ||
		got.DueDate != "2026-11-27" || !got.HasLabel("plane::data") || got.Project != "g/po-1" {
		t.Fatalf("issue = %+v, want the created fields back", got)
	}
	if _, err := tr.GetIssue("g/po-1", 9); !errors.Is(err, tracker.ErrNotFound) {
		t.Fatalf("missing issue = %v, want ErrNotFound", err)
	}

	list, _ := tr.ListIssues("g/po-1", tracker.IssueQuery{State: "opened", Labels: []string{"plane::data"}})
	if len(list) != 1 || list[0].IID != 1 {
		t.Fatalf("label query = %+v, want issue 1 only", list)
	}
	if list, _ := tr.ListIssues("g/po-1", tracker.IssueQuery{Search: "second"}); len(list) != 1 || list[0].IID != 2 {
		t.Fatalf("search = %+v, want issue 2 only", list)
	}

	if err := tr.CreateNote("g/po-1", 1, "a note"); err != nil {
		t.Fatal(err)
	}
	if notes, _ := tr.ListNotes("g/po-1", 1); len(notes) != 1 || notes[0].Body != "a note" || notes[0].Author != "alice" {
		t.Fatalf("notes = %+v", notes)
	}

	closeState, reopen := "close", "reopen"
	if got, err := tr.UpdateIssue("g/po-1", 2, tracker.IssueUpdate{State: &closeState}); err != nil || got.State != "closed" {
		t.Fatalf("close = %+v, %v", got, err)
	}
	if list, _ := tr.ListIssues("g/po-1", tracker.IssueQuery{State: "opened"}); len(list) != 1 {
		t.Fatalf("open issues after close = %+v, want 1", list)
	}
	if got, err := tr.UpdateIssue("g/po-1", 2, tracker.IssueUpdate{State: &reopen}); err != nil || got.State != "opened" {
		t.Fatalf("reopen = %+v, %v", got, err)
	}

	me, _ := tr.CurrentUser()
	if err := tr.AssignIssue("g/po-1", 1, me); err != nil {
		t.Fatal(err)
	}
	got, err = tr.UpdateIssue("g/po-1", 1, tracker.IssueUpdate{AddLabels: []string{"status::in-progress"}})
	if err != nil || !slices.Contains(got.Labels, "status::in-progress") {
		t.Fatalf("start = %+v, %v, want status::in-progress", got, err)
	}

}

// TestCreatesRenumberOnCollidingIIDs: two clones create an issue in the same
// repository between pulls and both pick the next IID; the earlier push
// keeps it and the later one moves to the next free number.
func TestCreatesRenumberOnCollidingIIDs(t *testing.T) {
	theirs := Issue{ID: "sm-aaaa", IID: 1, Repository: "g/po-1", Title: "theirs"}
	ours := Issue{ID: "sm-bbbb", IID: 1, Repository: "g/po-1", Title: "ours"}
	elsewhere := Issue{ID: "sm-cccc", IID: 1, Repository: "g/po-2", Title: "elsewhere"}
	merged := mergeIssues(Table, nil, []Issue{ours, elsewhere}, []Issue{theirs}, "2026-11-02T09:00:00Z")
	iid := map[string]int{}
	for _, i := range merged {
		iid[i.ID] = i.IID
	}
	if iid["sm-aaaa"] != 1 || iid["sm-bbbb"] != 2 || iid["sm-cccc"] != 1 {
		t.Fatalf("iids = %v, want theirs 1, ours renumbered to 2, the other repository untouched", iid)
	}
}

// TestGetFileReadsThroughTextconv reads a file as the repository's textconv
// shows it, as git-crypt decrypts, and a file with none as stored.
func TestGetFileReadsThroughTextconv(t *testing.T) {
	tr := newTestTracker(t, "alice")
	code := tr.Code.Dir
	beadstest.Git(t, code, "config", "diff.upper.textconv", "tr a-z A-Z <")
	for name, body := range map[string]string{".gitattributes": "policy.md diff=upper\n", "policy.md": "secret\n", "plain.md": "plain\n"} {
		if err := os.WriteFile(filepath.Join(code, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	beadstest.Git(t, code, "add", "-A")
	beadstest.Git(t, code, "commit", "-q", "-m", "files")
	beadstest.Git(t, code, "push", "-q", "origin", "main")
	if b, err := tr.GetFile("g/po-1", "policy.md", "main"); err != nil || string(b) != "SECRET\n" {
		t.Fatalf("policy.md = %q, %v; want it through the textconv", b, err)
	}
	if b, err := tr.GetFile("g/po-1", "plain.md", "main"); err != nil || string(b) != "plain\n" {
		t.Fatalf("plain.md = %q, %v", b, err)
	}
	if _, err := tr.GetFile("g/po-1", "missing.md", "main"); !errors.Is(err, tracker.ErrNotFound) {
		t.Fatalf("missing file: %v", err)
	}
}

// TestReadNamesTheCommitAfterAWrite reports, after a write, the tracker
// commit that write made, not the pull before it.
func TestReadNamesTheCommitAfterAWrite(t *testing.T) {
	tr := newTestTracker(t, "alice")
	if _, err := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "first"}); err != nil {
		t.Fatal(err)
	}
	remote := beadstest.Git(t, tr.Store.Dir, "rev-parse", "origin/main")
	if read, ok := tr.LastRead(); !ok || read.Commit != remote {
		t.Fatalf("read %+v after the write; the tracker is at %s", read, remote)
	}
}
