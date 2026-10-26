// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// delivery is how a popped branch is approved and merged (srd013 R5.5,
// R5.6). The local delivery is the beads one: an approval is a comment on
// the issue naming the commit the reviewer read, and the merge is a --no-ff
// merge in the project's main checkout, pushed to its origin. A tracking
// system whose host reviews pull or merge requests supplies its own.
type delivery interface {
	// approve records by's approval of branch at commit on the issue, and
	// reports whether it recorded anything now: a second approval of the
	// same commit by the same person records nothing.
	approve(issueProject string, iid int, branch, commit, by string) (bool, error)
	// approval names who approved branch at tip, other than the issue's
	// assignees or, for an issue nobody took, other than merger. Without
	// one, the error names what is missing.
	approval(issueProject string, iid int, branch, tip, merger string) (string, error)
	// merge merges branch into base in the main checkout at main with
	// message, publishes it, and returns the merge commit.
	merge(main, base, branch, message string) (string, error)
}

// deliveryFor is the delivery of the system serving project: its host's
// review when it offers one, else the local one. Tests replace it to watch
// the calls.
var deliveryFor = func(t tracker.Tracker, git Git, project string) delivery {
	if rv, ok := tracker.For(t, project).(tracker.Reviewer); ok {
		return reviewDelivery{t: t, rv: rv, git: git, project: project}
	}
	return localDelivery{t: t, git: git}
}

// localDelivery approves by comment and merges in the local checkout.
type localDelivery struct {
	t   tracker.Issues
	git Git
}

func (d localDelivery) approve(issueProject string, iid int, branch, commit, by string) (bool, error) {
	notes, err := d.t.ListNotes(issueProject, iid)
	if err != nil {
		return false, err
	}
	for _, n := range notes {
		if n.Author == by && strings.Contains(n.Body, approvalMarker(commit)) {
			return false, nil
		}
	}
	return true, d.t.CreateNote(issueProject, iid, fmt.Sprintf("Approved %s at %s for merge.\n\n%s", branch, commit, approvalMarker(commit)))
}

func (d localDelivery) approval(issueProject string, iid int, branch, tip, merger string) (string, error) {
	i, err := d.t.GetIssue(issueProject, iid)
	if err != nil {
		return "", err
	}
	excluded := i.Assignees
	if len(excluded) == 0 {
		excluded = []string{merger}
	}
	notes, err := d.t.ListNotes(issueProject, iid)
	if err != nil {
		return "", err
	}
	stale := ""
	for _, n := range notes {
		if slices.Contains(excluded, n.Author) {
			continue
		}
		if strings.Contains(n.Body, approvalMarker(tip)) {
			return n.Author, nil
		}
		if at := strings.Index(n.Body, approvalPrefix); at >= 0 {
			stale, _, _ = strings.Cut(n.Body[at+len(approvalPrefix):], " ")
		}
	}
	short := func(c string) string { return c[:min(12, len(c))] }
	if stale != "" {
		return "", fmt.Errorf("the approval of %s#%d is for %s, and %s is now at %s; ask for a new approval with the approve skill (srd013 R5.5)",
			issueProject, iid, short(stale), branch, short(tip))
	}
	return "", fmt.Errorf("no approval of %s at %s from someone other than %s; ask a reviewer to approve it with the approve skill (srd013 R5.5)",
		branch, short(tip), strings.Join(excluded, ", "))
}

func (d localDelivery) merge(main, base, branch, message string) (string, error) {
	run := func(args ...string) (string, error) {
		out, err := d.git.Run(main, args...)
		return strings.TrimSpace(out), err
	}
	if current, _ := run("branch", "--show-current"); current != base {
		if _, err := run("checkout", "-q", base); err != nil {
			return "", err
		}
	}
	run("merge", "-q", "--ff-only", "refs/remotes/origin/"+base)
	if _, err := run("merge", "-q", "--no-ff", branch, "-m", message); err != nil {
		return "", err
	}
	if _, err := run("push", "-q", "origin", base); err != nil {
		return "", fmt.Errorf("merged locally but could not push %s: %w", base, err)
	}
	return run("rev-parse", "HEAD")
}

// reviewDelivery approves and merges through the host's review: a review
// approves the head commit, and the host merges, after which the local
// checkout is brought level so the merge can be read and closed on.
type reviewDelivery struct {
	t       tracker.Issues
	rv      tracker.Reviewer
	git     Git
	project string
}

func (d reviewDelivery) approve(_ string, _ int, branch, commit, _ string) (bool, error) {
	return d.rv.Approve(d.project, branch, commit)
}

func (d reviewDelivery) approval(issueProject string, iid int, branch, tip, merger string) (string, error) {
	i, err := d.t.GetIssue(issueProject, iid)
	if err != nil {
		return "", err
	}
	excluded := i.Assignees
	if len(excluded) == 0 {
		excluded = []string{merger}
	}
	by, stale, err := d.rv.Approval(d.project, branch, tip, excluded)
	if err != nil || by != "" {
		return by, err
	}
	short := func(c string) string { return c[:min(12, len(c))] }
	if stale != "" {
		return "", fmt.Errorf("the review of %s approved %s, and it is now at %s; ask for a new approval with the approve skill (srd013 R5.5)",
			branch, short(stale), short(tip))
	}
	return "", fmt.Errorf("no approval of %s at %s from someone other than %s; ask a reviewer to approve it with the approve skill (srd013 R5.5)",
		branch, short(tip), strings.Join(excluded, ", "))
}

func (d reviewDelivery) merge(main, base, branch, message string) (string, error) {
	tip, err := d.git.Run(main, "rev-parse", "refs/heads/"+branch)
	if err != nil {
		return "", err
	}
	commit, err := d.rv.Merge(d.project, branch, strings.TrimSpace(tip), message)
	if err != nil {
		return "", fmt.Errorf("the host refused to merge %s; if its branch protection requires a review, ask for one with the approve skill: %w", branch, err)
	}
	run := func(args ...string) (string, error) {
		out, err := d.git.Run(main, args...)
		return strings.TrimSpace(out), err
	}
	if _, err := run("fetch", "-q", "origin", base); err != nil {
		return commit, err
	}
	if current, _ := run("branch", "--show-current"); current != base {
		if _, err := run("checkout", "-q", base); err != nil {
			return commit, err
		}
	}
	_, err = run("merge", "-q", "--ff-only", "refs/remotes/origin/"+base)
	return commit, err
}
