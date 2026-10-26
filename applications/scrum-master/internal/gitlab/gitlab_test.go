// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package gitlab

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

const fleet = "acme/delivery/fleet"

// TestIssuesRoundTrip files, reads, lists, updates, comments on, assigns,
// and closes an issue under a group milestone its project finds (#132).
func TestIssuesRoundTrip(t *testing.T) {
	l := newLab()
	tr := New(t.TempDir(), l.as("alice"))
	m := tracker.Milestone{Title: "01.0", Description: "Release 01.0", StartDate: "2026-11-02", DueDate: "2026-11-20"}
	for _, want := range []string{tracker.MilestoneCreated, tracker.MilestoneUnchanged} {
		if res, err := tr.EnsureMilestone("acme", m); err != nil || res != want {
			t.Fatalf("ensure = %s, %v; want %s", res, err, want)
		}
	}
	m.DueDate = "2026-11-27"
	if res, _ := tr.EnsureMilestone("acme", m); res != tracker.MilestoneUpdated {
		t.Fatalf("a moved due date = %s, want updated", res)
	}
	if got, err := tr.FindMilestone(fleet, "01.0"); err != nil || got != m {
		t.Fatalf("the project finds its ancestor group's milestone: %+v, %v", got, err)
	}
	if active, _ := tr.ActiveMilestones(fleet); len(active) != 1 {
		t.Fatalf("active milestones = %+v", active)
	}
	i, err := tr.CreateIssue(fleet, tracker.NewIssue{Title: "Score one agent", Description: "Body", Labels: []string{"plane::data", "kind::build"},
		DueDate: "2026-11-13", Milestone: "01.0"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := tr.GetIssue(fleet, i.IID)
	if err != nil || got.DueDate != "2026-11-13" || got.Milestone != "01.0" || got.State != "opened" || got.Author != "alice" ||
		!slices.Equal(got.Labels, []string{"plane::data", "kind::build"}) {
		t.Fatalf("issue = %+v, %v", got, err)
	}
	if open, _ := tr.ListIssues(fleet, tracker.IssueQuery{State: "opened", Labels: []string{"kind::build"}, Search: "score"}); len(open) != 1 {
		t.Fatalf("list = %+v", open)
	}
	closed := "close"
	if _, err := tr.UpdateIssue(fleet, i.IID, tracker.IssueUpdate{AddLabels: []string{"needs-triage"}, RemoveLabels: []string{"kind::build"}, State: &closed}); err != nil {
		t.Fatal(err)
	}
	if got, _ = tr.GetIssue(fleet, i.IID); got.State != "closed" || got.ClosedAt != "2026-11-06T10:00:00Z" || !slices.Equal(got.Labels, []string{"plane::data", "needs-triage"}) {
		t.Fatalf("after update = %+v", got)
	}
	if err := tr.CreateNote(fleet, i.IID, "a ledger event"); err != nil {
		t.Fatal(err)
	}
	if notes, _ := tr.ListNotes(fleet, i.IID); len(notes) != 1 || notes[0].Author != "alice" {
		t.Fatalf("notes = %+v", notes)
	}
	l.as("bob")
	if err := tr.AssignIssue(fleet, i.IID, tracker.User{Username: "bob"}); err != nil {
		t.Fatal(err)
	}
	if got, _ = tr.GetIssue(fleet, i.IID); !slices.Equal(got.Assignees, []string{"bob"}) {
		t.Fatalf("assignees = %v", got.Assignees)
	}
	l.project(fleet).awards[i.IID] = []map[string]any{{"name": "thumbsup", "user": map[string]any{"username": "bob"}}}
	if aw, _ := tr.IssueAwards(fleet, i.IID); len(aw) != 1 || aw[0] != (tracker.Award{Name: "thumbsup", User: "bob"}) {
		t.Fatalf("awards = %+v", aw)
	}
	if err := tr.CreateGroupLabel("acme", tracker.Label{Name: "plane::data", Color: "#0e8a16", Description: "Work"}); err != nil {
		t.Fatal(err)
	}
	if err := tr.UpdateGroupLabel("acme", tracker.Label{Name: "plane::data", Color: "#00aa00", Description: "Work"}); err != nil {
		t.Fatal(err)
	}
	if ls, _ := tr.GroupLabels("acme"); len(ls) != 1 || ls[0].Color != "#00aa00" {
		t.Fatalf("group labels = %+v", ls)
	}
}

const workBody = `## Requirements

1. Add the grader notes.

## Acceptance Criteria

- The notes exist (test: notes_exist; environment: local)
`

// TestDeliveryThroughMergeRequests is the loop on the glab system,
// offline: pop opens the branch with a draft merge request; another
// person's approval, pinned to the head commit, lets merge land it on
// GitLab, after which the issue closes on the merge commit in the local
// checkout; an approval of an earlier commit is refused (#132).
func TestDeliveryThroughMergeRequests(t *testing.T) {
	root := t.TempDir()
	checkout := beadstest.CodeRepository(t, root, "fleet", "alice")
	l := newLab()
	l.origins[fleet] = filepath.Join(root, "fleet.git")
	alice, bob := New(checkout, l.as("alice")), New(checkout, l.as("bob"))
	if _, err := alice.EnsureMilestone("acme/delivery", tracker.Milestone{Title: "m", StartDate: "2026-01-01", DueDate: "2099-01-01"}); err != nil {
		t.Fatal(err)
	}
	file := func(title string) tracker.Issue {
		i, err := alice.CreateIssue(fleet, tracker.NewIssue{Title: title, Description: workBody, Milestone: "m",
			Labels: []string{"plane::data", "kind::build", "component::fleet"}})
		if err != nil {
			t.Fatal(err)
		}
		return i
	}
	work := file("Add grader notes")
	p, err := program.Pop(alice, program.ExecGit{}, fleet, work.IID, checkout, program.Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	if mr := l.project(fleet).mrs[1]; !strings.Contains(p.Review, "/merge_requests/") || mr == nil || mr["draft"] != true {
		t.Fatalf("pop opened no draft merge request: %+v", p)
	}
	if again, err := program.Pop(alice, program.ExecGit{}, fleet, work.IID, checkout, program.Defaults(), false); err != nil || again.Review != p.Review || len(l.project(fleet).mrs) != 1 {
		t.Fatalf("second pop = %+v, %v; want the same review", again, err)
	}
	commit := func(dir, f string) {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(f+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		beadstest.Git(t, dir, "add", "-A")
		beadstest.Git(t, dir, "commit", "-q", "-m", "Add "+f)
		beadstest.Git(t, dir, "push", "-q", "-u", "origin", "HEAD")
	}
	commit(p.Worktree, "GRADER.md")
	gate := program.MergeGate{Command: "true", Dir: p.Worktree, RequireApproval: true}
	if _, err := program.Merge(alice, program.ExecGit{}, fleet, fleet, work.IID, p.Branch, p.Worktree, gate, nil, false); err == nil || !strings.Contains(err.Error(), "no approval") {
		t.Fatalf("merge without approval = %v, want a refusal", err)
	}
	if r, err := program.Approve(bob, program.ExecGit{}, fleet, fleet, work.IID, p.Branch, checkout); err != nil || !r.Posted || r.By != "bob" {
		t.Fatalf("bob's approval = %+v, %v", r, err)
	}
	if r, _ := program.Approve(bob, program.ExecGit{}, fleet, fleet, work.IID, p.Branch, checkout); r.Posted {
		t.Fatal("a second approval of the same commit posted again")
	}
	m, err := program.Merge(alice, program.ExecGit{}, fleet, fleet, work.IID, p.Branch, p.Worktree, gate, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	head := beadstest.Git(t, checkout, "rev-parse", "main")
	message := beadstest.Git(t, checkout, "log", "-1", "--format=%B", "main")
	if !m.Merged || m.Commit != head || m.ApprovedBy != "bob" || !strings.Contains(message, fmt.Sprintf("%s#%d", fleet, work.IID)) || !strings.Contains(message, "Merged-by: alice") {
		t.Fatalf("merge = %+v, main at %s with %q", m, head, message)
	}
	if got, _ := alice.GetIssue(fleet, work.IID); got.State != "closed" {
		t.Fatalf("issue %s after the merge", got.State)
	}

	moved := file("Score a batch")
	q, err := program.Pop(alice, program.ExecGit{}, fleet, moved.IID, checkout, program.Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	commit(q.Worktree, "BATCH.md")
	if _, err := program.Approve(bob, program.ExecGit{}, fleet, fleet, moved.IID, q.Branch, checkout); err != nil {
		t.Fatal(err)
	}
	commit(q.Worktree, "MORE.md")
	if _, err := program.Merge(alice, program.ExecGit{}, fleet, fleet, moved.IID, q.Branch, q.Worktree, program.MergeGate{Command: "true", Dir: q.Worktree, RequireApproval: true}, nil, false); err == nil || !strings.Contains(err.Error(), "now at") {
		t.Fatalf("merge after the head moved = %v, want a refusal naming the stale approval", err)
	}
	// Without the policy's requirement, the author merges their own merge request:
	// the stale approval is not reported, and the issue closes.
	m, err = program.Merge(alice, program.ExecGit{}, fleet, fleet, moved.IID, q.Branch, q.Worktree, program.MergeGate{Command: "true", Dir: q.Worktree}, nil, false)
	if err != nil || !m.Merged || m.ApprovedBy != "" {
		t.Fatalf("merge under the default policy = %+v, %v; want merged with no approver", m, err)
	}
	if got, _ := alice.GetIssue(fleet, moved.IID); got.State != "closed" {
		t.Fatalf("issue %s after the default-policy merge", got.State)
	}
}
