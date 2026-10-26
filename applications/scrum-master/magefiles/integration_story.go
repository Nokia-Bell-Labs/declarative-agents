// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.Story)
}

// Story files a story's issues in po-1, one blocked behind an open issue,
// lists them with scrum-master issue list --implements, pops the first, claims the
// second into its worktree with scrum-master issue pop --worktree, builds both on
// the one branch, and proves one approved merge closes them both while the
// blocked one stays open (srd036 R5, srd022 R6).
func (Integration) Story() error {
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
	const story = "st001-score-trajectories"
	file := func(title, extra string) (program.FileResult, error) {
		path := filepath.Join(r.run.Dir, "story-"+program.Slug(title)+".md")
		writeFileTo(path, "## Requirements\n\n1. "+title+".\n\n## Acceptance Criteria\n\n- It is done (test: done; environment: local)\n\nImplements: "+story+"\n"+extra)
		var res program.FileResult
		return res, r.wordsJSON(&res, "issue", "file", "--project", po1, "--title", title, "--body-file", path,
			"--plane", "data", "--kind", "build", "--component", "po-1", "--milestone", rigMilestone)
	}
	first, err := file("Score one trajectory", "")
	if err != nil {
		return err
	}
	second, err := file("Score a batch", "")
	if err != nil {
		return err
	}
	upstream, err := file("Upstream schema", "")
	if err != nil {
		return err
	}
	held, err := file("Wait on the schema", fmt.Sprintf("Blocked-by: %s#%d\n", po1, upstream.IID))
	if err != nil {
		return err
	}

	var listed program.ListResult
	if err := r.wordsJSON(&listed, "issue", "list", "--project", po1, "--implements", story); err != nil {
		return err
	}
	var order []int
	for _, i := range listed.Issues {
		order = append(order, i.IID)
	}
	c.expect(slices.Equal(order, []int{first.IID, second.IID, upstream.IID}) && len(listed.Blocked) == 1 && listed.Blocked[0].IID == held.IID,
		"list: issues %v, blocked %+v", order, listed.Blocked)

	clone, err := r.clone(po1, "po-1")
	if err != nil {
		return err
	}
	var popped, joined program.PopResult
	if err := r.wordsJSON(&popped, "issue", "pop", "--project", po1, "--issue", fmt.Sprint(first.IID), "--dir", clone); err != nil {
		return err
	}
	if err := r.wordsJSON(&joined, "issue", "pop", "--project", po1, "--issue", fmt.Sprint(second.IID), "--worktree", popped.Worktree); err != nil {
		return err
	}
	c.expect(joined.Branch == popped.Branch && joined.Worktree == popped.Worktree && joined.Assigned && !joined.Created && !joined.Added,
		"pop --worktree: %+v, want %s on %s", joined, popped.Worktree, popped.Branch)
	for _, f := range []string{"ONE.md", "BATCH.md"} {
		writeFileTo(filepath.Join(popped.Worktree, f), "# "+f+"\n")
		if err := r.commitPush(popped.Worktree, "Add "+f, f); err != nil {
			return err
		}
	}
	if err := r.approve(po1, po1, first.IID, ""); err != nil {
		return err
	}
	var merged program.MergeResult
	if err := r.wordsJSON(&merged, "issue", "merge", "--project", po1, "--issue", fmt.Sprint(first.IID), "--dir", popped.Worktree,
		"--closes", fmt.Sprintf("%s#%d", po1, second.IID)); err != nil {
		return err
	}
	c.expect(merged.Merged, "merge: %+v", merged)
	c.expect(r.closed(po1, first.IID) && r.closed(po1, second.IID), "the story's two issues are not both closed after one merge")
	c.expect(!r.closed(po1, held.IID), "the blocked issue closed")
	return c.err("story")
}
