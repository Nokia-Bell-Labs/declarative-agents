// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// TestEpicStoriesInRankOrder reads an epic's stories, directly or through
// a feature, in rank order, and leaves out another epic's (#155).
func TestEpicStoriesInRankOrder(t *testing.T) {
	docs, err := hierarchy.Load("../../testdata/hierarchy/valid")
	if err != nil {
		t.Fatal(err)
	}
	got := storiesUnder(docs, "ep001-agent-evaluation")
	if want := []string{"st001-score-trajectories", "st002-store-scores", "st003-scorecard-page"}; !slices.Equal(got, want) {
		t.Fatalf("stories under ep001 = %v, want %v", got, want)
	}
	if docID("https://gitlab.example.com/g/x/-/blob/v1.0.0/docs/specs/epics/ep002-rubrics.md#R1") != "ep002-rubrics" {
		t.Fatal("a pinned URL's document id")
	}
}

// TestImplementAStory works a story's issues the way implement does:
// list them in order with a blocked one apart, pop the first into a
// worktree, claim the second into that worktree on the same branch,
// refuse an issue someone else holds, and close both with one approved
// merge (#155).
func TestImplementAStory(t *testing.T) {
	tr, checkout := newBeadsTracker(t, "alice")
	story := "st001-score-trajectories"
	file := func(title, extra string) tracker.Issue {
		t.Helper()
		i, err := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: title, Milestone: "m",
			Description: goodBody + "\nImplements: " + story + "\n" + extra,
			Labels:      []string{"plane::data", "kind::build", "component::po-1"}})
		if err != nil {
			t.Fatal(err)
		}
		return i
	}
	first, second := file("Score one trajectory", ""), file("Score a batch", "")
	blocker, _ := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "Upstream schema", Labels: []string{"plane::management"}})
	held := file("Waits on the schema", fmt.Sprintf("Blocked-by: g/po-1#%d\n", blocker.IID))
	if _, err := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "Another story's", Description: goodBody + "\nImplements: st002-store-scores\n",
		Labels: []string{"plane::data", "kind::build", "component::po-1"}, Milestone: "m"}); err != nil {
		t.Fatal(err)
	}

	l, err := ListImplementing(tr, "g/po-1", "", story, "")
	if err != nil {
		t.Fatal(err)
	}
	var refs []int
	for _, i := range l.Issues {
		refs = append(refs, i.IID)
	}
	if !slices.Equal(refs, []int{first.IID, second.IID}) || len(l.Blocked) != 1 || l.Blocked[0].IID != held.IID {
		t.Fatalf("list = %+v, want %d and %d ready and %d blocked", l, first.IID, second.IID, held.IID)
	}

	p, err := Pop(tr, ExecGit{}, "g/po-1", first.IID, checkout, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	q, err := PopInto(tr, ExecGit{}, "g/po-1", "g/po-1", second.IID, p.Worktree, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	if q.Branch != p.Branch || q.Worktree != p.Worktree || !q.Assigned || q.Created || q.Added {
		t.Fatalf("pop into the worktree = %+v, want %s on %s, assigned, nothing opened", q, p.Worktree, p.Branch)
	}
	bob := reviewer(t, tr, checkout, "bob")
	other := file("Bob's own", "")
	if _, err := Pop(bob, ExecGit{}, "g/po-1", other.IID, checkout, Defaults(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := PopInto(tr, ExecGit{}, "g/po-1", "g/po-1", other.IID, p.Worktree, Defaults(), false); err == nil || !strings.Contains(err.Error(), "assigned to bob") {
		t.Fatalf("pop of bob's issue into the worktree = %v, want a refusal", err)
	}

	for n, f := range []string{"one.go", "batch.go"} {
		if err := os.WriteFile(filepath.Join(p.Worktree, f), []byte("package scoring\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		beadstest.Git(t, p.Worktree, "add", "-A")
		beadstest.Git(t, p.Worktree, "commit", "-q", "-m", fmt.Sprintf("Unit %d", n+1))
	}
	beadstest.Git(t, p.Worktree, "push", "-q", "-u", "origin", "HEAD")
	if _, err := Approve(bob, ExecGit{}, "g/po-1", "g/po-1", first.IID, p.Branch, checkout); err != nil {
		t.Fatal(err)
	}
	m, err := Merge(tr, ExecGit{}, "g/po-1", "g/po-1", first.IID, p.Branch, p.Worktree, MergeGate{Command: "true", Dir: p.Worktree},
		[]string{fmt.Sprintf("g/po-1#%d", second.IID)}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, iid := range []int{first.IID, second.IID} {
		if got, _ := tr.GetIssue("g/po-1", iid); got.State != "closed" {
			t.Fatalf("g/po-1#%d is %s after the story's merge %+v, want closed", iid, got.State, m)
		}
	}
	if got, _ := tr.GetIssue("g/po-1", held.IID); got.State == "closed" {
		t.Fatal("the blocked issue closed")
	}
}
