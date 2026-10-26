// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.Pop)
}

// Pop files an admitted issue in po-1, pops it into a worktree of a local clone, proves the refusals, commits, delivers it with scrum-master issue merge, and proves the issue closed on the merge (test-rel00.1-pop).
func (Integration) Pop() error {
	r, err := newRigEnv()
	if err != nil {
		return err
	}
	po1, err := r.project("product-owners/po-1")
	if err != nil {
		return err
	}
	if err := r.milestone(rigMilestone, rigDue); err != nil {
		return err
	}
	if _, err := r.runWords("issue", "labels", "--group", r.group); err != nil {
		return err
	}
	c := &check{}
	run := r.run.Dir
	good := "## Requirements\n\n1. Add the grader notes.\n\n## Acceptance Criteria\n\n- The notes exist (test: notes_exist; environment: local)\n"
	bad := "## Requirements\n\n1. Something.\n"
	file := func(title, body string) (program.FileResult, error) {
		path := filepath.Join(run, "pop-"+program.Slug(title)+".md")
		writeFileTo(path, body)
		var res program.FileResult
		return res, r.wordsJSON(&res, "issue", "file", "--project", po1, "--title", title, "--body-file", path,
			"--plane", "data", "--kind", "build", "--component", "po-1", "--milestone", rigMilestone)
	}

	work, err := file("Add grader notes", good)
	if err != nil {
		return err
	}
	clone, err := r.clone(po1, "po-1")
	if err != nil {
		return err
	}
	if _, err := r.runWords("install", "--agent", "claude", "--dir", clone); err != nil {
		return err
	}

	var popped program.PopResult
	if err := r.wordsJSON(&popped, "issue", "pop", "--project", po1, "--issue", fmt.Sprint(work.IID), "--dir", clone); err != nil {
		return err
	}
	i, err := r.issue(po1, work.IID)
	if err != nil {
		return err
	}
	c.expect(len(i.Assignees) == 1 && i.Assignees[0] == r.user, "assignees after pop: %v", i.Assignees)
	c.expect(r.remoteBranch(po1, popped.Branch), "branch %s not on the remote", popped.Branch)
	_, serr := os.Stat(popped.Worktree)
	c.expect(serr == nil && strings.HasPrefix(popped.Worktree, run), "worktree %s missing or outside the run directory", popped.Worktree)
	_, ierr := os.Stat(filepath.Join(popped.Worktree, ".claude/skills/implement/SKILL.md"))
	c.expect(slices.Equal(popped.Installed, []string{"claude"}) && ierr == nil, "skills not installed in the worktree: %v %q %v", popped.Installed, popped.InstallError, ierr)
	status, _ := exec.Command("git", "-C", popped.Worktree, "status", "--porcelain", "--untracked-files=all").Output()
	c.expect(len(status) == 0, "worktree not clean after the install: %q", status)

	var again program.PopResult
	if err := r.wordsJSON(&again, "issue", "pop", "--project", po1, "--issue", fmt.Sprint(work.IID), "--dir", clone); err != nil {
		return err
	}
	c.expect(!again.Assigned && !again.Created && !again.Added, "second pop changed something: %+v", again)

	closed, err := file("Already closed", good)
	if err != nil {
		return err
	}
	closeState := "close"
	if _, err := r.tr.UpdateIssue(po1, closed.IID, tracker.IssueUpdate{State: &closeState}); err != nil {
		return err
	}
	_, err = r.runWords("issue", "pop", "--project", po1, "--issue", fmt.Sprint(closed.IID), "--dir", clone)
	c.expect(err != nil && strings.Contains(err.Error(), "is closed"), "pop of a closed issue: %v", err)
	triaged, err := file("No acceptance criteria", bad)
	if err != nil {
		return err
	}
	_, err = r.runWords("issue", "pop", "--project", po1, "--issue", fmt.Sprint(triaged.IID), "--dir", clone)
	c.expect(err != nil && strings.Contains(err.Error(), "not admitted"), "pop of a needs-triage issue: %v", err)

	writeFileTo(filepath.Join(popped.Worktree, "GRADER.md"), "# Grader notes\n")
	if err := r.commitPush(popped.Worktree, "Add grader notes", "GRADER.md"); err != nil {
		return err
	}
	// The default policy merges without an approval; one that requires
	// approval refuses until a reviewer gives one (srd013 R5.5).
	var unapproved program.MergeResult
	err = r.wordsJSON(&unapproved, "issue", "merge", "--project", po1, "--issue", fmt.Sprint(work.IID), "--dir", popped.Worktree, "--dry-run")
	c.expect(err == nil && unapproved.DryRun && unapproved.ApprovedBy == "", "dry-run merge under the default policy: %+v, %v", unapproved, err)
	system, err := r.project("system")
	if err != nil {
		return err
	}
	if err := r.seed(system, map[string]string{program.PolicyFile: "---\nid: policy\ntitle: Rig policy\n---\n\n<!-- field: admission -->\n## Admission\n\n" +
		"<!-- field: require_approval -->\n### Require approval\n\ntrue\n"}); err != nil {
		return err
	}
	requiring := []string{"issue", "merge", "--project", po1, "--issue", fmt.Sprint(work.IID), "--dir", popped.Worktree, "--control", system, "--ref", seedBranch}
	_, err = r.runWords(requiring...)
	c.expect(err != nil && strings.Contains(err.Error(), "no approval"), "merge with no approval under require_approval: %v", err)
	if err := r.approve(po1, po1, work.IID, ""); err != nil {
		return err
	}
	var merged program.MergeResult
	if err := r.wordsJSON(&merged, requiring...); err != nil {
		return err
	}
	c.expect(merged.Merged && merged.Commit != "" && merged.ApprovedBy == rigReviewer, "merge: %+v", merged)
	c.expect(r.closed(po1, work.IID), "issue #%d still open after its merge", work.IID)
	return c.err("pop")
}
