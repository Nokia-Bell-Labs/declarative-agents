// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Cleanup is what the clean-up word did after a delivery (GH-2796): the
// issue's worktree, its branch in the main checkout, and the branch on
// origin, each removed, kept with the reason, or already gone.
type Cleanup struct {
	Worktree       string   `json:"worktree,omitempty"`
	WorktreeAction string   `json:"worktree_action"` // removed, kept, none
	Branch         string   `json:"branch"`
	BranchAction   string   `json:"branch_action"` // deleted, kept, none
	RemoteAction   string   `json:"remote_action"` // deleted, kept, none
	Reasons        []string `json:"reasons"`
	DryRun         bool     `json:"dry_run"`
}

// CleanUp removes what pop opened for an issue once its branch is merged
// (GH-2796): the worktree that has branch checked out, unless it holds a
// change git status shows; the branch in the main checkout, which git
// branch -d keeps unless it is merged; and the branch on origin, once the
// local one is gone. It runs from the main checkout of the repository dir
// belongs to, so it may remove the worktree it was started in. Nothing it
// cannot do is an error: it is reported with the reason, since the
// delivery it follows has already landed. A dry run removes nothing.
func CleanUp(git Git, dir, branch string, dryRun bool) (Cleanup, error) {
	c := Cleanup{Branch: branch, WorktreeAction: "none", BranchAction: "none", RemoteAction: "none", Reasons: []string{}, DryRun: dryRun}
	run := func(in string, args ...string) (string, error) {
		out, err := git.Run(in, args...)
		return strings.TrimSpace(out), err
	}
	common, err := run(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return c, fmt.Errorf("clean-up needs a local checkout: %w", err)
	}
	main := strings.TrimSuffix(strings.TrimSuffix(common, "/"), "/.git")
	if current, _ := run(main, "branch", "--show-current"); current == branch {
		c.BranchAction, c.RemoteAction = "kept", "kept"
		c.Reasons = append(c.Reasons, branch+" is the main checkout's branch")
		return c, nil
	}
	// Nothing is removed for a branch the main checkout has not merged: its
	// work would be left only in a branch with no worktree.
	if _, err := run(main, "rev-parse", "--verify", "-q", "refs/heads/"+branch); err == nil {
		if _, err := run(main, "merge-base", "--is-ancestor", "refs/heads/"+branch, "HEAD"); err != nil {
			c.WorktreeAction, c.BranchAction, c.RemoteAction = "kept", "kept", "kept"
			c.Reasons = append(c.Reasons, branch+" is not merged into the main checkout's branch, so nothing is removed")
			return c, nil
		}
	}
	list, err := run(main, "worktree", "list", "--porcelain")
	if err != nil {
		return c, err
	}
	for _, block := range strings.Split(list, "\n\n") {
		var wt, head string
		for _, line := range strings.Split(block, "\n") {
			if v, ok := strings.CutPrefix(line, "worktree "); ok {
				wt = v
			} else if v, ok := strings.CutPrefix(line, "branch "); ok {
				head = v
			}
		}
		if head == "refs/heads/"+branch && !samePath(wt, main) {
			c.Worktree = wt
		}
	}
	if c.Worktree != "" {
		switch status, err := run(c.Worktree, "status", "--porcelain"); {
		case err != nil:
			c.WorktreeAction = "kept"
			c.Reasons = append(c.Reasons, fmt.Sprintf("%s: git status failed: %v", c.Worktree, err))
		case status != "":
			c.WorktreeAction = "kept"
			c.Reasons = append(c.Reasons, c.Worktree+" holds changes git status shows; commit or discard them, then remove it")
		case dryRun:
			c.WorktreeAction = "removed"
		default:
			if _, err := run(main, "worktree", "remove", c.Worktree); err != nil {
				c.WorktreeAction = "kept"
				c.Reasons = append(c.Reasons, fmt.Sprintf("%s: %v", c.Worktree, err))
			} else {
				c.WorktreeAction = "removed"
			}
		}
	}
	if c.WorktreeAction == "kept" {
		c.BranchAction, c.RemoteAction = "kept", "kept"
		c.Reasons = append(c.Reasons, branch+" is kept with its worktree")
		return c, nil
	}
	if _, err := run(main, "rev-parse", "--verify", "-q", "refs/heads/"+branch); err == nil {
		switch {
		case dryRun:
			c.BranchAction = "deleted"
		default:
			// -d, never -D: git keeps a branch the checkout has not merged.
			if _, err := run(main, "branch", "-d", branch); err != nil {
				c.BranchAction, c.RemoteAction = "kept", "kept"
				c.Reasons = append(c.Reasons, fmt.Sprintf("%s is not merged into the main checkout's branch, so it is kept: %v", branch, err))
				return c, nil
			}
			c.BranchAction = "deleted"
		}
	}
	if heads, err := run(main, "ls-remote", "--heads", "origin", "refs/heads/"+branch); err != nil {
		c.Reasons = append(c.Reasons, fmt.Sprintf("origin could not be read: %v", err))
	} else if heads != "" {
		if dryRun {
			c.RemoteAction = "deleted"
		} else if _, err := run(main, "push", "-q", "origin", "--delete", branch); err != nil {
			c.RemoteAction = "kept"
			c.Reasons = append(c.Reasons, fmt.Sprintf("origin's %s could not be deleted: %v", branch, err))
		} else {
			c.RemoteAction = "deleted"
		}
	}
	return c, nil
}

// samePath reports whether two paths name the same directory, symbolic
// links resolved, as git prints a worktree's real path.
func samePath(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

// Human is the clean-up's line in a merge's text output.
func (c Cleanup) Human() string {
	parts := []string{}
	if c.Worktree != "" {
		parts = append(parts, "worktree "+c.WorktreeAction)
	}
	parts = append(parts, "branch "+c.BranchAction, "origin's branch "+c.RemoteAction)
	line := "clean-up: " + strings.Join(parts, ", ")
	if len(c.Reasons) > 0 {
		line += " (" + strings.Join(c.Reasons, "; ") + ")"
	}
	return line
}
