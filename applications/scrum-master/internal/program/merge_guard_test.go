// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package program

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// A checkout on the default branch, or on another issue's branch, is not
// the issue's branch: merge refuses to infer it (GH-22).
func TestInferBranchRefusesAForeignCheckout(t *testing.T) {
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "-c", "user.name=t", "-c", "user.email=t@e", "commit", "-q", "--allow-empty", "-m", "first")
	if _, err := inferBranch(dir, 18); err == nil || !strings.Contains(err.Error(), "not the issue's 18-<slug> branch") {
		t.Fatalf("on main: err = %v, want the refusal", err)
	}
	gitIn(t, dir, "checkout", "-q", "-b", "7-other-issue")
	if _, err := inferBranch(dir, 18); err == nil {
		t.Fatal("on another issue's branch: want a refusal")
	}
	gitIn(t, dir, "checkout", "-q", "-b", "18-release-corpus")
	if b, err := inferBranch(dir, 18); err != nil || b != "18-release-corpus" {
		t.Fatalf("on the issue's branch: %q, %v", b, err)
	}
}

// A merge of the default branch is refused before it can take an earlier
// merge citing the issue as its own (GH-22).
func TestMergeRefusesTheDefaultBranch(t *testing.T) {
	tr, checkout := newBeadsTracker(t, "alice")
	work, _ := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "Change the use case", Milestone: "m", Description: goodBody,
		Labels: []string{"plane::data", "kind::spec", "component::po-1"}})
	_, err := MergeBranch(tr, ExecGit{}, "g/po-1", "g/po-1", work.IID, "main", checkout, MergeGate{}, nil, true)
	if err == nil || !strings.Contains(err.Error(), "is the default branch") {
		t.Fatalf("err = %v, want the default branch refused", err)
	}
}

// Under --json a refusal is JSON too, with the reason, and the error still
// carries the non-zero exit (GH-22).
func TestRefusalIsJSONUnderJSON(t *testing.T) {
	var out bytes.Buffer
	ctx := &cli.Context{JSON: true, Out: &out}
	err := refusedJSON(ctx, MergeResult{Project: "g/po-1", IID: 17}, errors.New("already delivered"))
	if err == nil || !strings.Contains(out.String(), `"refused": "already delivered"`) {
		t.Fatalf("err = %v, out = %s", err, out.String())
	}
}

// A dry run names only the --closes issues still open (GH-22).
func TestOpenRefsLeavesTheClosedOut(t *testing.T) {
	got := openRefs([]string{"a/x#1", "a/y#1"}, []string{"a/x#1"})
	if !slices.Equal(got, []string{"a/y#1"}) {
		t.Fatalf("open = %v", got)
	}
}

// A refusal before the merge result exists, such as a checkout on the
// default branch, is JSON under --json too, naming the project and issue
// the command was given (GH-24).
func TestEarlyRefusalIsJSONUnderJSON(t *testing.T) {
	var out bytes.Buffer
	ctx := &cli.Context{JSON: true, Out: &out}
	err := refusedJSON(ctx, MergeResult{Project: "g/po-1", IID: 17, Branch: ""}, errors.New(". is checked out on main, not the issue's 17-<slug> branch"))
	if err == nil || !strings.Contains(out.String(), `"refused": ". is checked out on main`) || !strings.Contains(out.String(), `"issue": 17`) {
		t.Fatalf("err = %v, out = %s", err, out.String())
	}
}
