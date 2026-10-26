// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// trackerBranch puts an orphan tracker branch holding an empty issues file
// on the origin of the code checkout, as a program of one repository has.
func trackerBranch(t *testing.T, checkout, branch string) {
	t.Helper()
	beadstest.Git(t, checkout, "checkout", "-q", "--orphan", branch)
	beadstest.Git(t, checkout, "rm", "-rqf", ".")
	if err := os.MkdirAll(filepath.Join(checkout, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, IssuesFile), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	beadstest.Git(t, checkout, "add", "-A")
	beadstest.Git(t, checkout, "commit", "-q", "-m", "seed the tracker branch")
	beadstest.Git(t, checkout, "push", "-q", "origin", "HEAD:"+branch)
	beadstest.Git(t, checkout, "checkout", "-q", "main")
}

// TestTrackerOnABranchOfTheCodeRepository holds srd042 R3.1 for a program of
// one repository: the tracker opened on a branch of the code repository is a
// single-branch clone of it, its writes land on that branch, and the code's
// main never moves.
func TestTrackerOnABranchOfTheCodeRepository(t *testing.T) {
	root := t.TempDir()
	checkout := beadstest.CodeRepository(t, root, "po-1", "alice")
	origin := filepath.Join(root, "po-1.git")
	const branch = "scrum-master/tracker"
	trackerBranch(t, checkout, branch)
	main := beadstest.Git(t, root, "--git-dir", origin, "rev-parse", "main")

	clone := filepath.Join(root, "clone")
	t.Setenv("SCRUM_MASTER_TRACKER_CLONE", clone)
	t.Setenv("SCRUM_MASTER_USER", "alice")
	t.Setenv(BranchEnv, "")
	t.Chdir(checkout)
	tr, err := OpenRepository(origin, branch)
	if err != nil {
		t.Fatal(err)
	}
	beadstest.Identity(t, clone, "alice")
	if _, err := tr.CreateIssue("po-1", tracker.NewIssue{Title: "first"}); err != nil {
		t.Fatal(err)
	}
	if got := beadstest.Git(t, clone, "branch", "-r"); got != "origin/"+branch {
		t.Fatalf("the clone tracks %q; want only origin/%s", got, branch)
	}
	if after := beadstest.Git(t, root, "--git-dir", origin, "rev-parse", "main"); after != main {
		t.Fatalf("the code's main moved from %s to %s", main, after)
	}
	head := beadstest.Git(t, root, "--git-dir", origin, "rev-parse", branch)
	if read, ok := tr.LastRead(); !ok || read.Commit != head {
		t.Fatalf("read %+v after the write; the tracker branch is at %s", read, head)
	}
	if _, err := os.Stat(filepath.Join(checkout, ".beads")); !os.IsNotExist(err) {
		t.Fatalf("the code checkout holds .beads/: %v", err)
	}
}

// TestTrackerBranchFromTheEnvironment: SCRUM_MASTER_TRACKER_BRANCH names the
// branch for one person's run, in place of the one the caller passes.
func TestTrackerBranchFromTheEnvironment(t *testing.T) {
	root := t.TempDir()
	checkout := beadstest.CodeRepository(t, root, "po-1", "alice")
	origin := filepath.Join(root, "po-1.git")
	trackerBranch(t, checkout, "elsewhere")
	t.Setenv("SCRUM_MASTER_TRACKER_CLONE", filepath.Join(root, "clone"))
	t.Setenv("SCRUM_MASTER_USER", "alice")
	t.Setenv(BranchEnv, "elsewhere")
	t.Chdir(checkout)
	tr, err := OpenRepository(origin, "main")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Store.Branch != "elsewhere" {
		t.Fatalf("branch %q; want the environment's", tr.Store.Branch)
	}
}
