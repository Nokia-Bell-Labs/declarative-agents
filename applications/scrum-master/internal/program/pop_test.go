// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// recGit records git calls; rev-parse --show-toplevel answers root, and
// rev-parse --verify of a local branch fails unless localBranch is set.
type recGit struct {
	root        string
	localBranch bool
	calls       []string
}

func (g *recGit) Run(dir string, args ...string) (string, error) {
	call := strings.Join(args, " ")
	g.calls = append(g.calls, call)
	switch {
	case call == "rev-parse --show-toplevel":
		return g.root + "\n", nil
	case strings.HasPrefix(call, "rev-parse --verify"):
		if g.localBranch {
			return "", nil
		}
		return "", errors.New("no such branch")
	case strings.HasPrefix(call, "worktree add"):
		return "", os.MkdirAll(args[len(args)-2], 0o755)
	}
	return "", nil
}

func admitted(f *fake.Fake, title string) tracker.Issue {
	i, _ := f.CreateIssue("g/po-1", tracker.NewIssue{Title: title, Milestone: "m", Description: goodBody,
		Labels: []string{"plane::data", "kind::build", "component::po-1"}})
	return i
}

func newPopFixture(t *testing.T) (*fake.Fake, *recGit) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "po-1")
	return fake.New(), &recGit{root: root}
}

func TestPopAssignsOnlyTheCaller(t *testing.T) {
	f, g := newPopFixture(t)
	i := admitted(f, "Score one agent")
	r, err := Pop(f, g, "g/po-1", i.IID, ".", Defaults(), false)
	if err != nil || !r.Assigned {
		t.Fatalf("r=%+v err=%v", r, err)
	}
	got, _ := f.GetIssue("g/po-1", i.IID)
	if len(got.Assignees) != 1 || got.Assignees[0] != "me" {
		t.Fatalf("assignees = %v", got.Assignees)
	}
}

func TestPopRefusesHeldOrUnadmitted(t *testing.T) {
	f, g := newPopFixture(t)
	held := admitted(f, "Held")
	f.Issues["g/po-1"][held.IID-1].Assignees = []string{"someone"}
	closed := admitted(f, "Closed")
	f.Issues["g/po-1"][closed.IID-1].State = "closed"
	triage, _ := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "No criteria", Labels: []string{"plane::data", "kind::build"}})
	for _, c := range []struct {
		iid  int
		want string
	}{{held.IID, "assigned to someone"}, {closed.IID, "is closed"}, {triage.IID, "not admitted"}} {
		writes := f.Writes
		if _, err := Pop(f, g, "g/po-1", c.iid, ".", Defaults(), false); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("#%d: err = %v, want %q", c.iid, err, c.want)
		}
		if f.Writes != writes {
			t.Errorf("#%d: refusal wrote to the tracker", c.iid)
		}
	}
}

// TestPopReportsUnadmittedManagementIssue pops a management issue that
// fails admission, which admission does not gate, and reports why it fails
// (srd013 R7.1).
func TestPopReportsUnadmittedManagementIssue(t *testing.T) {
	f, g := newPopFixture(t)
	i, _ := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Plan the next release", Description: goodBody,
		Labels: []string{"plane::management"}})
	r, err := Pop(f, g, "g/po-1", i.IID, ".", Defaults(), false)
	if err != nil || !r.Assigned || len(r.NotAdmitted) != 1 || !strings.Contains(r.NotAdmitted[0], "component") {
		t.Fatalf("r=%+v err=%v", r, err)
	}
	ok := admitted(f, "Admitted")
	if r, err := Pop(f, g, "g/po-1", ok.IID, ".", Defaults(), false); err != nil || len(r.NotAdmitted) != 0 {
		t.Fatalf("admitted issue: r=%+v err=%v", r, err)
	}
}

func TestPopCreatesBranchAndWorktree(t *testing.T) {
	f, g := newPopFixture(t)
	i := admitted(f, "Score one agent: trajectories & rubric")

	dry, err := Pop(f, g, "g/po-1", i.IID, ".", Defaults(), true)
	if err != nil || f.Writes != 1 || !dry.Created || !dry.Added {
		t.Fatalf("dry run: r=%+v writes=%d err=%v", dry, f.Writes, err)
	}

	r, err := Pop(f, g, "g/po-1", i.IID, ".", Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Branch != "1-score-one-agent-trajectories-rubric" || f.Branches["g/po-1@"+r.Branch] != "main" {
		t.Fatalf("branch %q branches %v", r.Branch, f.Branches)
	}
	if r.Worktree != filepath.Join(filepath.Dir(g.root), "po-1-"+r.Branch) || !r.Added {
		t.Fatalf("worktree %q", r.Worktree)
	}
	if calls := strings.Join(g.calls, "\n"); !strings.Contains(calls, "worktree add --no-checkout --track -b "+r.Branch+" "+r.Worktree+" origin/"+r.Branch) ||
		!strings.HasSuffix(calls, "\ncheckout") {
		t.Fatalf("git calls: %q", g.calls)
	}

	writes := f.Writes
	again, err := Pop(f, g, "g/po-1", i.IID, ".", Defaults(), false)
	if err != nil || again.Assigned || again.Created || again.Added || f.Writes != writes {
		t.Fatalf("second pop: r=%+v writes=%d err=%v", again, f.Writes-writes, err)
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Score one agent":                              "score-one-agent",
		"  Fix: the  reader!! ":                        "fix-the-reader",
		"A very long title that keeps going and going": "a-very-long-title-that-keeps-going-and-g",
		"!!!": "issue",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestPopInAnotherRepository claims a status issue in the person's project
// and opens the branch the issue names in the reports repository.
func TestPopInAnotherRepository(t *testing.T) {
	f, g := newPopFixture(t)
	i, _ := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Weekly status", Milestone: "m", Description: goodBody,
		Labels: []string{"plane::data", "kind::review", "component::po-1"}})
	r, err := PopIn(f, g, "g/reports", "g/po-1", i.IID, "2026-W42-po-1", ".", Defaults(), false)
	if err != nil || r.Project != "g/reports" || r.IssueProject != "g/po-1" || r.Branch != "2026-W42-po-1" || !r.Assigned {
		t.Fatalf("r=%+v err=%v", r, err)
	}
	if got, _ := f.GetIssue("g/po-1", i.IID); len(got.Assignees) != 1 {
		t.Fatalf("the issue was not claimed: %v", got.Assignees)
	}
	if _, ok := f.Branches["g/reports@2026-W42-po-1"]; !ok {
		t.Fatalf("branches %v, want 2026-W42-po-1 in g/reports", f.Branches)
	}
}
