// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package github

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

const fleet = "acme/fleet"

// TestIssuesRoundTrip files, reads, lists, updates, comments on, assigns,
// and closes an issue, with its due date and milestone kept where GitHub
// has no field for them (#133).
func TestIssuesRoundTrip(t *testing.T) {
	h := newHub()
	tr := New(t.TempDir(), h.as("alice"), nil)
	if res, err := tr.EnsureMilestone(fleet, tracker.Milestone{Title: "01.0", Description: "Release 01.0", StartDate: "2026-11-02", DueDate: "2026-11-20"}); err != nil || res != tracker.MilestoneCreated {
		t.Fatalf("milestone = %s, %v", res, err)
	}
	m, err := tr.FindMilestone(fleet, "01.0")
	if err != nil || m.StartDate != "2026-11-02" || m.DueDate != "2026-11-20" || m.Description != "Release 01.0" {
		t.Fatalf("milestone read back = %+v, %v", m, err)
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
	open, _ := tr.ListIssues(fleet, tracker.IssueQuery{State: "opened", Labels: []string{"kind::build"}, Search: "score"})
	if len(open) != 1 || open[0].IID != i.IID {
		t.Fatalf("list = %+v", open)
	}
	closed := "close"
	if _, err := tr.UpdateIssue(fleet, i.IID, tracker.IssueUpdate{AddLabels: []string{"needs-triage"}, RemoveLabels: []string{"kind::build"}, State: &closed}); err != nil {
		t.Fatal(err)
	}
	got, _ = tr.GetIssue(fleet, i.IID)
	if got.State != "closed" || got.ClosedAt != "2026-11-06T10:00:00Z" || !slices.Equal(got.Labels, []string{"plane::data", "needs-triage"}) || got.DueDate != "2026-11-13" {
		t.Fatalf("after update = %+v", got)
	}
	if err := tr.CreateNote(fleet, i.IID, "a ledger event"); err != nil {
		t.Fatal(err)
	}
	if notes, _ := tr.ListNotes(fleet, i.IID); len(notes) != 1 || notes[0].Author != "alice" || notes[0].Body != "a ledger event" {
		t.Fatalf("notes = %+v", notes)
	}
	if err := tr.AssignIssue(fleet, i.IID, tracker.User{Username: "alice"}); err != nil {
		t.Fatal(err)
	}
	if got, _ = tr.GetIssue(fleet, i.IID); !slices.Equal(got.Assignees, []string{"alice"}) {
		t.Fatalf("assignees = %v", got.Assignees)
	}
	h.repo(fleet).reactions[i.IID] = []map[string]any{{"content": "+1", "user": map[string]any{"login": "bob"}}}
	if aw, _ := tr.IssueAwards(fleet, i.IID); len(aw) != 1 || aw[0] != (tracker.Award{Name: "thumbsup", User: "bob"}) {
		t.Fatalf("awards = %+v", aw)
	}
	if me, err := tr.CurrentUser(); err != nil || me.Username != "alice" {
		t.Fatalf("current user = %+v, %v", me, err)
	}
}

// TestGroupMilestonesAndLabelsGoToEachRepository writes a group's
// milestone and labels into each routed repository, since GitHub has no
// group-level ones, and counts a label as the group's once all carry it.
func TestGroupMilestonesAndLabelsGoToEachRepository(t *testing.T) {
	h := newHub()
	tr := New(t.TempDir(), h.as("alice"), []string{"acme/fleet", "acme/scoring"})
	m := tracker.Milestone{Title: "01.0", StartDate: "2026-11-02", DueDate: "2026-11-20"}
	for _, want := range []string{tracker.MilestoneCreated, tracker.MilestoneUnchanged} {
		if res, err := tr.EnsureMilestone("acme", m); err != nil || res != want {
			t.Fatalf("ensure = %s, %v; want %s", res, err, want)
		}
	}
	m.DueDate = "2026-11-27"
	if res, _ := tr.EnsureMilestone("acme", m); res != tracker.MilestoneUpdated {
		t.Fatalf("a moved due date = %s, want updated", res)
	}
	for _, repo := range []string{"acme/fleet", "acme/scoring"} {
		if got, err := tr.FindMilestone(repo, "01.0"); err != nil || got.DueDate != "2026-11-27" {
			t.Fatalf("%s milestone = %+v, %v", repo, got, err)
		}
	}
	if err := tr.CreateGroupLabel("acme", tracker.Label{Name: "plane::data", Color: "#0e8a16", Description: "Work"}); err != nil {
		t.Fatal(err)
	}
	h.repo("acme/fleet").labels = append(h.repo("acme/fleet").labels, map[string]any{"name": "fleet-only", "color": "ffffff"})
	ls, err := tr.GroupLabels("acme")
	if err != nil || len(ls) != 1 || ls[0] != (tracker.Label{Name: "plane::data", Color: "#0e8a16", Description: "Work"}) {
		t.Fatalf("group labels = %+v, %v", ls, err)
	}
}

const workBody = `## Requirements

1. Add the grader notes.

## Acceptance Criteria

- The notes exist (test: notes_exist; environment: local)
`

// TestDeliveryThroughPullRequests is the loop on the gh system, offline:
// pop opens the branch with a draft pull request; the author cannot
// approve; another person's approval of the head lets merge land it on
// GitHub, after which the issue closes on the merge commit in the local
// checkout; a head that moved after the approval is refused (#133).
func TestDeliveryThroughPullRequests(t *testing.T) {
	root := t.TempDir()
	checkout := beadstest.CodeRepository(t, root, "fleet", "alice")
	h := newHub()
	h.origins[fleet] = filepath.Join(root, "fleet.git")
	alice, bob := New(checkout, h.as("alice"), nil), New(checkout, h.as("bob"), nil)
	if _, err := alice.EnsureMilestone(fleet, tracker.Milestone{Title: "m", StartDate: "2026-01-01", DueDate: "2099-01-01"}); err != nil {
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
	if !strings.Contains(p.Review, "/pull/") || h.repo(fleet).pulls[2]["draft"] != true {
		t.Fatalf("pop opened no draft pull request: %+v", p)
	}
	again, err := program.Pop(alice, program.ExecGit{}, fleet, work.IID, checkout, program.Defaults(), false)
	if err != nil || again.Review != p.Review || len(h.repo(fleet).pulls) != 1 {
		t.Fatalf("second pop = %+v, %v; want the same review", again, err)
	}
	if err := os.WriteFile(filepath.Join(p.Worktree, "GRADER.md"), []byte("# Notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	beadstest.Git(t, p.Worktree, "add", "-A")
	beadstest.Git(t, p.Worktree, "commit", "-q", "-m", "Add grader notes")
	beadstest.Git(t, p.Worktree, "push", "-q", "-u", "origin", "HEAD")

	gate := program.MergeGate{Command: "true", Dir: p.Worktree, RequireApproval: true}
	if _, err := program.Merge(alice, program.ExecGit{}, fleet, fleet, work.IID, p.Branch, p.Worktree, gate, nil, false); err == nil || !strings.Contains(err.Error(), "no approval") {
		t.Fatalf("merge without approval = %v, want a refusal", err)
	}
	if _, err := alice.Approve(fleet, p.Branch, beadstest.Git(t, p.Worktree, "rev-parse", "HEAD")); err == nil || !strings.Contains(err.Error(), "own pull request") {
		t.Fatalf("the author's approval = %v, want GitHub's refusal", err)
	}
	if r, err := program.Approve(bob, program.ExecGit{}, fleet, fleet, work.IID, p.Branch, checkout); err != nil || !r.Posted || r.By != "bob" {
		t.Fatalf("bob's approval = %+v, %v", r, err)
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
	if notes, _ := alice.ListNotes(fleet, work.IID); !slices.ContainsFunc(notes, func(n tracker.Note) bool { return strings.Contains(n.Body, head) }) {
		t.Fatal("the issue does not cite its merge commit")
	}

	moved := file("Score a batch")
	q, err := program.Pop(alice, program.ExecGit{}, fleet, moved.IID, checkout, program.Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"BATCH.md", "MORE.md"} {
		if err := os.WriteFile(filepath.Join(q.Worktree, f), []byte(f+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		beadstest.Git(t, q.Worktree, "add", "-A")
		beadstest.Git(t, q.Worktree, "commit", "-q", "-m", "Add "+f)
		beadstest.Git(t, q.Worktree, "push", "-q", "-u", "origin", "HEAD")
		if f == "BATCH.md" {
			if _, err := program.Approve(bob, program.ExecGit{}, fleet, fleet, moved.IID, q.Branch, checkout); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := program.Merge(alice, program.ExecGit{}, fleet, fleet, moved.IID, q.Branch, q.Worktree, program.MergeGate{Command: "true", Dir: q.Worktree, RequireApproval: true}, nil, false); err == nil || !strings.Contains(err.Error(), "now at") {
		t.Fatalf("merge after the head moved = %v, want a refusal naming the stale approval", err)
	}
	// Without the policy's requirement, the author merges their own pull request:
	// the stale approval is not reported, and the issue closes.
	m, err = program.Merge(alice, program.ExecGit{}, fleet, fleet, moved.IID, q.Branch, q.Worktree, program.MergeGate{Command: "true", Dir: q.Worktree}, nil, false)
	if err != nil || !m.Merged || m.ApprovedBy != "" {
		t.Fatalf("merge under the default policy = %+v, %v; want merged with no approver", m, err)
	}
	if got, _ := alice.GetIssue(fleet, moved.IID); got.State != "closed" {
		t.Fatalf("issue %s after the default-policy merge", got.State)
	}
}
