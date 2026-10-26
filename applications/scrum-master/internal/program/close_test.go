// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// TestPopAndCloseRunOnBeads drives the real Pop and Close against a beads
// tracker and a code repository, both on local disk (srd042 R1.1, R2.3):
// pop assigns the caller and opens the branch at origin and the worktree;
// close refuses an unmerged branch, closes on the local merge citing the
// merge commit, and changes nothing the second time.
func TestPopAndCloseRunOnBeads(t *testing.T) {
	root := t.TempDir()
	remote := beadstest.TrackerRepository(t, root)
	trackerClone := filepath.Join(root, "alice-tracker")
	beadstest.Clone(t, remote, trackerClone, "alice")
	checkout := beadstest.CodeRepository(t, root, "po-1", "alice")
	tr := beads.NewTracker(trackerClone, "alice", checkout)

	i, err := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "Score one agent", Milestone: "m", Description: goodBody,
		Labels: []string{"plane::data", "kind::build", "component::po-1"}})
	if err != nil {
		t.Fatal(err)
	}

	p, err := Pop(tr, ExecGit{}, "g/po-1", i.IID, checkout, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Assigned || !p.Created || !p.Added {
		t.Fatalf("pop = %+v, want assigned, branch created, worktree added", p)
	}
	if got, _ := tr.GetIssue("g/po-1", i.IID); len(got.Assignees) != 1 || got.Assignees[0] != "alice" {
		t.Fatalf("assignees = %v, want alice", got.Assignees)
	}
	if out := beadstest.Git(t, checkout, "ls-remote", "--heads", "origin", p.Branch); out == "" {
		t.Fatalf("origin has no branch %s", p.Branch)
	}

	if _, err := Close(tr, ExecGit{}, "g/po-1", i.IID, checkout, "", false); err == nil || !strings.Contains(err.Error(), "not merged") {
		t.Fatalf("close before the merge = %v, want a refusal", err)
	}

	if err := os.WriteFile(filepath.Join(p.Worktree, "score.go"), []byte("package score\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	beadstest.Git(t, p.Worktree, "add", "-A")
	beadstest.Git(t, p.Worktree, "commit", "-q", "-m", "Score one agent")
	beadstest.Git(t, checkout, "merge", "-q", "--no-ff", p.Branch, "-m", "Merge g/po-1#1")
	merge := beadstest.Git(t, checkout, "rev-parse", "HEAD")

	c, err := Close(tr, ExecGit{}, "g/po-1", i.IID, checkout, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Closed || c.Commit != merge {
		t.Fatalf("close = %+v, want closed on %s", c, merge)
	}
	got, _ := tr.GetIssue("g/po-1", i.IID)
	notes, _ := tr.ListNotes("g/po-1", i.IID)
	if got.State != "closed" || len(notes) == 0 || !strings.Contains(notes[len(notes)-1].Body, merge) {
		t.Fatalf("issue = %+v, notes = %+v, want closed citing %s", got, notes, merge)
	}

	again, err := Close(tr, ExecGit{}, "g/po-1", i.IID, checkout, "", false)
	if err != nil || again.Closed {
		t.Fatalf("second close = %+v, %v, want no change", again, err)
	}
}

// TestPopOfflineIsProvisional: a pop taken while the tracker repository is
// unreachable reports the assignment as provisional (eng04-beads-sync).
func TestPopOfflineIsProvisional(t *testing.T) {
	root := t.TempDir()
	remote := beadstest.TrackerRepository(t, root)
	trackerClone := filepath.Join(root, "alice-tracker")
	beadstest.Clone(t, remote, trackerClone, "alice")
	checkout := beadstest.CodeRepository(t, root, "po-1", "alice")
	tr := beads.NewTracker(trackerClone, "alice", checkout)
	i, err := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "Score one agent", Milestone: "m", Description: goodBody,
		Labels: []string{"plane::data", "kind::build", "component::po-1"}})
	if err != nil {
		t.Fatal(err)
	}
	beadstest.Git(t, trackerClone, "remote", "set-url", "origin", filepath.Join(root, "unreachable.git"))

	p, err := Pop(tr, ExecGit{}, "g/po-1", i.IID, checkout, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Assigned || !p.Provisional {
		t.Fatalf("offline pop = %+v, want assigned and provisional", p)
	}
}

// TestCloseNeedsTheBeadsTracker: on a tracker that does not record merges,
// close refuses rather than closing an issue by hand (srd041 R9.3).
func TestCloseNeedsTheBeadsTracker(t *testing.T) {
	f := fake.New()
	i := admitted(f, "Score one agent")
	if _, err := Close(f, ExecGit{}, "g/po-1", i.IID, ".", "", false); err == nil || !strings.Contains(err.Error(), "scrum-master issue close serves the beads tracker") {
		t.Fatalf("close on a tracker without merges = %v, want a refusal naming the beads tracker", err)
	}
}

// TestCloseAnIssueTheMergeCites closes an issue that was never popped, such
// as an objective a sprint-end merge closes: with no pop branch of its own,
// it closes on the first merge commit on the default branch that cites it
// made after the issue was created (srd013 R5.1).
func TestCloseAnIssueTheMergeCites(t *testing.T) {
	tr, checkout := newBeadsTracker(t, "alice")
	objective, _ := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "Objective R1", Labels: []string{"plane::management"}})
	end, _ := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "Sprint end", Milestone: "m", Description: goodBody,
		Labels: []string{"plane::data", "kind::build", "component::po-1"}})

	if _, err := Close(tr, ExecGit{}, "g/po-1", objective.IID, checkout, "", false); err == nil || !strings.Contains(err.Error(), "no merge") {
		t.Fatalf("close before any merge cites it = %v, want a refusal", err)
	}
	beadstest.Git(t, checkout, "checkout", "-q", "-b", "other", "main")
	beadstest.Git(t, checkout, "commit", "-q", "--allow-empty", "-m", "other work")
	beadstest.Git(t, checkout, "checkout", "-q", "main")
	beadstest.Git(t, checkout, "merge", "-q", "--no-ff", "other", "-m", fmt.Sprintf("Merge other (g/po-1#%d0)", objective.IID))
	if _, err := Close(tr, ExecGit{}, "g/po-1", objective.IID, checkout, "", false); err == nil {
		t.Fatalf("a merge citing #%d0 closed #%d", objective.IID, objective.IID)
	}
	// A merge citing the same reference from before the issue existed, as a
	// re-seeded tracker's earlier run leaves, is not its delivery.
	beadstest.Git(t, checkout, "checkout", "-q", "-b", "earlier", "main")
	beadstest.Git(t, checkout, "commit", "-q", "--allow-empty", "-m", "earlier run")
	beadstest.Git(t, checkout, "checkout", "-q", "main")
	old := exec.Command("git", "merge", "-q", "--no-ff", "earlier", "-m", fmt.Sprintf("Merge earlier (g/po-1#%d)", objective.IID))
	old.Dir, old.Env = checkout, append(os.Environ(), "GIT_COMMITTER_DATE=2000-01-01T00:00:00Z", "GIT_AUTHOR_DATE=2000-01-01T00:00:00Z")
	if out, err := old.CombinedOutput(); err != nil {
		t.Fatalf("earlier merge: %v: %s", err, out)
	}
	if _, err := Close(tr, ExecGit{}, "g/po-1", objective.IID, checkout, "", false); err == nil || !strings.Contains(err.Error(), "no merge") {
		t.Fatalf("close on a merge older than the issue = %v, want a refusal", err)
	}

	p, err := Pop(tr, ExecGit{}, "g/po-1", end.IID, checkout, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Worktree, "summary.md"), []byte("# Sprint end\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	beadstest.Git(t, p.Worktree, "add", "-A")
	beadstest.Git(t, p.Worktree, "commit", "-q", "-m", "Sprint end summary")
	beadstest.Git(t, checkout, "merge", "-q", "--no-ff", p.Branch, "-m",
		fmt.Sprintf("Merge %s (g/po-1#%d, closes g/po-1#%d)", p.Branch, end.IID, objective.IID))
	merge := beadstest.Git(t, checkout, "rev-parse", "HEAD")

	for _, iid := range []int{end.IID, objective.IID} {
		c, err := Close(tr, ExecGit{}, "g/po-1", iid, checkout, "", false)
		if err != nil || !c.Closed || c.Commit != merge {
			t.Fatalf("close #%d = %+v, %v, want closed on %s", iid, c, err, merge)
		}
	}
	if got, _ := tr.GetIssue("g/po-1", objective.IID); got.State != "closed" {
		t.Fatalf("objective = %s, want closed", got.State)
	}
}
