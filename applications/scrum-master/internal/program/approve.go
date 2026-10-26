// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// approvalPrefix opens the marker an approval comment carries; the commit
// the reviewer read completes it (srd013 R5.6).
const approvalPrefix = "<!-- scrum-master program approve commit="

func approvalMarker(commit string) string { return approvalPrefix + commit + " -->" }

// ApproveResult is what approve reports.
type ApproveResult struct {
	Project      string `json:"project"`
	IssueProject string `json:"issue_project,omitempty"`
	IID          int    `json:"issue"`
	Branch       string `json:"branch"`
	Commit       string `json:"commit"`
	By           string `json:"by"`
	Posted       bool   `json:"posted"` // approve posted the comment now
}

// Approve posts a reviewer's approval of a branch on its issue, naming the
// tip of the branch at its origin, the commit the reviewer read (srd013
// R5.6). The issue's assignee cannot approve their own work, and a second
// approval of the same commit by the same person posts nothing. branch
// defaults to the issue's pop branch; dir is any checkout of project.
func Approve(t tracker.Tracker, git Git, project, issueProject string, iid int, branch, dir string) (ApproveResult, error) {
	r := ApproveResult{Project: project, IID: iid}
	if issueProject != project {
		r.IssueProject = issueProject
	}
	i, err := t.GetIssue(issueProject, iid)
	if err != nil {
		return r, err
	}
	if i.State == "closed" {
		return r, fmt.Errorf("%s#%d is closed", issueProject, iid)
	}
	me, err := tracker.UserIn(t, issueProject)
	if err != nil {
		return r, err
	}
	r.By = me.Username
	if slices.Contains(i.Assignees, me.Username) {
		return r, fmt.Errorf("%s#%d is assigned to %s; an approval comes from someone other than the issue's assignee (srd013 R5.6)", issueProject, iid, me.Username)
	}
	r.Branch = branch
	if r.Branch == "" {
		r.Branch = fmt.Sprintf("%d-%s", iid, Slug(i.Title))
	}
	git.Run(dir, "fetch", "-q", "origin", r.Branch)
	for _, ref := range []string{"refs/remotes/origin/" + r.Branch, "refs/heads/" + r.Branch} {
		if out, err := git.Run(dir, "rev-parse", "--verify", "-q", ref); err == nil {
			r.Commit = strings.TrimSpace(out)
			break
		}
	}
	if r.Commit == "" {
		return r, fmt.Errorf("no branch %s to approve in %s", r.Branch, project)
	}
	r.Posted, err = deliveryFor(t, git, project).approve(issueProject, iid, r.Branch, r.Commit, me.Username)
	return r, err
}
