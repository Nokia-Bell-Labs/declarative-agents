// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// recording is a delivery that records each call and passes it to the
// local delivery.
type recording struct {
	local delivery
	calls *[]string
}

func (r recording) approve(p string, iid int, branch, commit, by string) (bool, error) {
	*r.calls = append(*r.calls, "approve")
	return r.local.approve(p, iid, branch, commit, by)
}

func (r recording) approval(p string, iid int, branch, tip, merger string) (string, error) {
	*r.calls = append(*r.calls, "approval")
	return r.local.approval(p, iid, branch, tip, merger)
}

func (r recording) merge(main, base, branch, message string) (string, error) {
	*r.calls = append(*r.calls, "merge")
	return r.local.merge(main, base, branch, message)
}

// TestApproveAndMergeGoThroughTheDelivery proves approve records its
// approval, and merge checks the approval and merges, only through the
// delivery, the seam a hosted tracking system's review replaces (#157).
func TestApproveAndMergeGoThroughTheDelivery(t *testing.T) {
	var calls []string
	prev := deliveryFor
	deliveryFor = func(t tracker.Tracker, git Git, _ string) delivery {
		return recording{local: localDelivery{t: t, git: git}, calls: &calls}
	}
	t.Cleanup(func() { deliveryFor = prev })

	tr, checkout := newBeadsTracker(t, "alice")
	work, _ := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "Build the reader", Milestone: "m", Description: goodBody,
		Labels: []string{"plane::data", "kind::build", "component::po-1"}})
	p, err := Pop(tr, ExecGit{}, "g/po-1", work.IID, checkout, Defaults(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Worktree, "reader.go"), []byte("package reader\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	beadstest.Git(t, p.Worktree, "add", "-A")
	beadstest.Git(t, p.Worktree, "commit", "-q", "-m", "Build the reader")
	beadstest.Git(t, p.Worktree, "push", "-q", "-u", "origin", "HEAD")
	if _, err := Approve(reviewer(t, tr, checkout, "bob"), ExecGit{}, "g/po-1", "g/po-1", work.IID, p.Branch, checkout); err != nil {
		t.Fatal(err)
	}
	m, err := Merge(tr, ExecGit{}, "g/po-1", "g/po-1", work.IID, p.Branch, p.Worktree, MergeGate{Command: "true", Dir: p.Worktree}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls, []string{"approve", "approval", "merge"}) {
		t.Fatalf("delivery calls = %v, want approve, approval, merge", calls)
	}
	if head := beadstest.Git(t, checkout, "rev-parse", "main"); !m.Merged || m.Commit != head || m.ApprovedBy != "bob" {
		t.Fatalf("merge = %+v, main at %s", m, head)
	}
}
