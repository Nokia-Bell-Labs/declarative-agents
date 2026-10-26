// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// CommentResult is what scrum-master issue comment reports.
type CommentResult struct {
	Project string `json:"project"`
	IID     int    `json:"iid"`
	Posted  bool   `json:"posted"`
	DryRun  bool   `json:"dry_run"`
}

// Comment posts body on the issue as the person running it, through the
// tracking system serving the project: the one write a skill makes to an
// issue that no other command makes, such as a completed issue's Actual
// LOC line (srd013 R9). The issue must exist; a dry run posts nothing.
func Comment(t tracker.Tracker, project string, iid int, body string, dryRun bool) (CommentResult, error) {
	r := CommentResult{Project: project, IID: iid, DryRun: dryRun}
	if _, err := t.GetIssue(project, iid); err != nil {
		return r, fmt.Errorf("%s#%d: %w", project, iid, err)
	}
	if dryRun {
		return r, nil
	}
	if err := t.CreateNote(project, iid, body); err != nil {
		return r, err
	}
	r.Posted = true
	return r, nil
}
