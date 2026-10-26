// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// MergeCloser is a tracker that closes an issue on a merge it is shown,
// citing the merge commit. Only the beads tracker is one (srd042 R2.3);
// elsewhere a merge closes nothing on its own (srd041 R9.3).
type MergeCloser interface {
	CloseOnMerge(project string, iid int, commit string) (bool, error)
}

// CloseResult is what close reports.
type CloseResult struct {
	Project string `json:"project"`
	IID     int    `json:"iid"`
	Branch  string `json:"branch"`
	Commit  string `json:"commit"`
	Closed  bool   `json:"closed"` // close changed the issue now
	DryRun  bool   `json:"dry_run"`
}

// Close closes an issue on the local merge that delivered it (srd042 R2.3):
// a merge commit on the default branch of the checkout around dir that
// cites project#iid. When the issue has a pop branch, the merge must bring
// that branch in, so a fast-forward or a branch with nothing of its own is
// not a delivery. at, when set, is the merge that delivered it, as scrum-master
// issue merge knows the merge it made, and must be one of those merges;
// otherwise an issue never popped, such as an objective a sprint-end merge
// closes, closes on the first merge citing it made after the issue was
// created, so an earlier merge citing the same reference is not taken for
// its delivery (srd013 R5.1). A second close changes nothing.
func Close(t tracker.Tracker, git Git, project string, iid int, dir, at string, dryRun bool) (CloseResult, error) {
	r := CloseResult{Project: project, IID: iid, DryRun: dryRun}
	c, ok := tracker.For(t, project).(MergeCloser)
	if !ok {
		return r, fmt.Errorf("%s#%d closes on its merge only on a tracker that records it; scrum-master issue close serves the beads tracker", project, iid)
	}
	i, err := t.GetIssue(project, iid)
	if err != nil {
		return r, err
	}
	r.Branch = fmt.Sprintf("%d-%s", iid, Slug(i.Title))
	base, err := t.DefaultBranch(project)
	if err != nil {
		return r, err
	}
	cite := fmt.Sprintf("%s#%d", project, iid)
	path := "refs/heads/" + base
	if tip, err := git.Run(dir, "rev-parse", "--verify", "-q", "refs/heads/"+r.Branch); err == nil {
		path = strings.TrimSpace(tip) + ".." + path
	} else {
		r.Branch = ""
	}
	// project#1 must not match project#12.
	grep := "--grep=" + regexp.QuoteMeta(cite) + "([^0-9]|$)"
	merges, err := git.Run(dir, "rev-list", "--ancestry-path", "--merges", "--reverse", "-E", grep, path)
	if r.Branch == "" {
		args := []string{"rev-list", "--first-parent", "--merges", "--reverse", "-E", grep}
		if at == "" && i.CreatedAt != "" {
			args = append(args, "--since="+i.CreatedAt)
		}
		merges, err = git.Run(dir, append(args, path)...)
	}
	if err != nil {
		return r, err
	}
	list := strings.Fields(merges)
	first := ""
	if len(list) > 0 {
		first = list[0]
	}
	if at != "" {
		if !slices.Contains(list, at) {
			return r, fmt.Errorf("the merge %s does not deliver %s on %s", at, cite, base)
		}
		first = at
	}
	switch {
	case first == "" && r.Branch != "":
		return r, fmt.Errorf("%s is not merged into %s by a merge commit citing %s (git merge --no-ff)", r.Branch, base, cite)
	case first == "":
		return r, fmt.Errorf("no merge commit on %s cites %s, and it has no pop branch", base, cite)
	}
	r.Commit = first
	if dryRun {
		return r, nil
	}
	r.Closed, err = c.CloseOnMerge(project, iid, r.Commit)
	return r, err
}
