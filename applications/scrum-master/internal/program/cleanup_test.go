// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// cleanupRepo is a main checkout on main with an origin, and a worktree
// beside it for each named branch, each with one commit pushed to origin.
func cleanupRepo(t *testing.T, branches ...string) (main string, worktrees map[string]string, git func(dir string, args ...string) string) {
	t.Helper()
	base, _ := filepath.EvalSymlinks(t.TempDir())
	origin, main := filepath.Join(base, "origin.git"), filepath.Join(base, "po-1")
	git = func(dir string, args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git(base, "init", "-q", "--bare", "-b", "main", origin)
	git(base, "clone", "-q", origin, main)
	git(main, "commit", "-q", "--allow-empty", "-m", "first")
	git(main, "push", "-q", "origin", "main")
	worktrees = map[string]string{}
	for _, b := range branches {
		wt := filepath.Join(base, "po-1-"+b)
		git(main, "worktree", "add", "-q", "-b", b, wt)
		os.WriteFile(filepath.Join(wt, b+".txt"), []byte(b+"\n"), 0o644)
		git(wt, "add", "-A")
		git(wt, "commit", "-q", "-m", b)
		git(wt, "push", "-q", "origin", b)
		worktrees[b] = wt
	}
	return main, worktrees, git
}

// Once its branch is merged, clean-up removes the issue's worktree, the
// branch, and origin's branch, run from the worktree itself (GH-2796).
func TestCleanUpRemovesAMergedIssuesWorktreeAndBranches(t *testing.T) {
	main, wts, git := cleanupRepo(t, "3-issue")
	git(main, "merge", "-q", "--no-ff", "3-issue", "-m", "Merge 3-issue")
	git(main, "push", "-q", "origin", "main")

	dry, err := CleanUp(ExecGit{}, wts["3-issue"], "3-issue", true)
	if err != nil || dry.WorktreeAction != "removed" || dry.BranchAction != "deleted" || dry.RemoteAction != "deleted" || !dry.DryRun {
		t.Fatalf("dry run = %+v, %v", dry, err)
	}
	if _, err := os.Stat(wts["3-issue"]); err != nil {
		t.Fatalf("a dry run removed the worktree: %v", err)
	}

	c, err := CleanUp(ExecGit{}, wts["3-issue"], "3-issue", false)
	if err != nil || c.WorktreeAction != "removed" || c.BranchAction != "deleted" || c.RemoteAction != "deleted" || len(c.Reasons) != 0 {
		t.Fatalf("clean-up = %+v, %v", c, err)
	}
	if _, err := os.Stat(wts["3-issue"]); !os.IsNotExist(err) {
		t.Errorf("the worktree is still there: %v", err)
	}
	if b := git(main, "branch", "--list", "3-issue"); b != "" {
		t.Errorf("the branch is still there: %q", b)
	}
	if r := git(main, "ls-remote", "--heads", "origin", "3-issue"); r != "" {
		t.Errorf("origin's branch is still there: %q", r)
	}
	// A second run finds nothing left, and says so without failing.
	again, err := CleanUp(ExecGit{}, main, "3-issue", false)
	if err != nil || again.WorktreeAction != "none" || again.BranchAction != "none" || again.RemoteAction != "none" {
		t.Errorf("a second clean-up = %+v, %v", again, err)
	}
}

// A worktree holding a change, or a branch not merged, is kept with its
// branches, and clean-up says why (GH-2796).
func TestCleanUpKeepsWhatIsNotDelivered(t *testing.T) {
	main, wts, git := cleanupRepo(t, "4-dirty", "5-unmerged")
	git(main, "merge", "-q", "--no-ff", "4-dirty", "-m", "Merge 4-dirty")
	os.WriteFile(filepath.Join(wts["4-dirty"], "notes.txt"), []byte("not committed\n"), 0o644)

	c, err := CleanUp(ExecGit{}, wts["4-dirty"], "4-dirty", false)
	if err != nil || c.WorktreeAction != "kept" || c.BranchAction != "kept" || c.RemoteAction != "kept" || !strings.Contains(strings.Join(c.Reasons, " "), "holds changes") {
		t.Fatalf("a worktree with a change = %+v, %v", c, err)
	}
	if _, err := os.Stat(wts["4-dirty"]); err != nil {
		t.Errorf("the worktree with a change was removed: %v", err)
	}

	c, err = CleanUp(ExecGit{}, wts["5-unmerged"], "5-unmerged", false)
	if err != nil || c.WorktreeAction != "kept" || c.BranchAction != "kept" || !strings.Contains(strings.Join(c.Reasons, " "), "not merged") {
		t.Fatalf("an unmerged branch = %+v, %v", c, err)
	}
	if b := git(main, "branch", "--list", "5-unmerged"); b == "" {
		t.Error("the unmerged branch was deleted")
	}
	if r := git(main, "ls-remote", "--heads", "origin", "5-unmerged"); r == "" {
		t.Error("origin's unmerged branch was deleted")
	}
}
