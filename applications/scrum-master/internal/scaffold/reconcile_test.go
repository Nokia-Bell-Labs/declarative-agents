// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/backlog"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// byReconcileProject is the entry for one project in a reconcile result.
func byReconcileProject(t *testing.T, r ReconcileResult, p string) ReconcileEntry {
	t.Helper()
	for _, e := range r.Repositories {
		if e.Project == p {
			return e
		}
	}
	t.Fatalf("%s is not in the result: %+v", p, r.Repositories)
	return ReconcileEntry{}
}

// hasFinding reports whether any finding contains every given substring.
func hasFinding(e ReconcileEntry, parts ...string) bool {
	for _, f := range e.Findings {
		all := true
		for _, p := range parts {
			if !strings.Contains(f, p) {
				all = false
			}
		}
		if all {
			return true
		}
	}
	return false
}

// A clean program reconciles ok: a reachable registered repository with a
// matching checkout files nothing, an unreachable one and one whose
// checkout's origin names another project each get one issue in the system
// repository naming the finding, and a rerun reports the open issues and
// files nothing twice (GH-2822).
func TestReconcileFilesOneIssuePerDriftedRepository(t *testing.T) {
	root, origins, system := checkoutsProgram(t, "g/pods/p1/tpm", "g/pods/p2/tpm", "g/analytics/model-evaluation")
	if _, err := Checkouts(system, false); err != nil {
		t.Fatal(err)
	}
	// p2's origin vanishes from the forge, as a move outside the registry
	// would leave it; model-evaluation's checkout points elsewhere.
	os.RemoveAll(filepath.Join(origins, "g", "pods", "p2"))
	checkoutsGit(t, filepath.Join(root, "g", "analytics", "model-evaluation"), "remote", "set-url", "origin", "git@example.com:g/analytics/renamed.git")

	f := fake.New()
	r, err := Reconcile(f, system, false)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if e := byReconcileProject(t, r, "g/pods/p1/tpm"); e.Action != "ok" || len(e.Findings) != 0 {
		t.Errorf("the clean repository = %+v, want ok", e)
	}
	unreachable := byReconcileProject(t, r, "g/pods/p2/tpm")
	if unreachable.Action != "filed" || !hasFinding(unreachable, "unreachable", "update the row in srd") {
		t.Errorf("the unreachable repository = %+v", unreachable)
	}
	moved := byReconcileProject(t, r, "g/analytics/model-evaluation")
	if moved.Action != "filed" || !hasFinding(moved, "origin g/analytics/renamed", "not the registered g/analytics/model-evaluation") {
		t.Errorf("the repointed checkout = %+v", moved)
	}
	issues, _ := f.ListIssues("g/program", tracker.IssueQuery{State: "opened"})
	if len(issues) != 2 {
		t.Fatalf("%d issues in the system repository, want 2: %+v", len(issues), issues)
	}
	for _, i := range issues {
		if !i.HasLabel("op::reconcile") || !i.HasLabel("plane::management") {
			t.Errorf("the issue lacks its labels: %v", i.Labels)
		}
	}

	again, err := Reconcile(f, system, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"g/pods/p2/tpm", "g/analytics/model-evaluation"} {
		if e := byReconcileProject(t, again, p); e.Action != "open" || e.Issue == "" {
			t.Errorf("rerun for %s = %+v, want open with its issue", p, e)
		}
	}
	if issues, _ := f.ListIssues("g/program", tracker.IssueQuery{State: "opened"}); len(issues) != 2 {
		t.Errorf("the rerun filed more issues: %d", len(issues))
	}
}

// A story pinned to a document the pinned repository's checkout does not
// hold, and an initiative item naming a missing document, are findings in
// the repository's issue (GH-2822).
func TestReconcileFindsDeadPinsAndMissingInitiatives(t *testing.T) {
	root, _, system := checkoutsProgram(t, "g/pods/p1/tpm", "g/analytics/model-evaluation")
	if _, err := Checkouts(system, false); err != nil {
		t.Fatal(err)
	}
	// The registry declares model-evaluation an initiative's repository,
	// naming a document the system repository does not hold.
	srd := filepath.Join(system, "docs", "specs", "software-requirements", "srd003-team1.md")
	b, _ := os.ReadFile(srd)
	os.WriteFile(srd, []byte(strings.Replace(string(b), "- owner: o1", "- owner: o1\n- initiative: in009-missing", 1)), 0o644)
	// p1's story pins a feature model-evaluation's checkout does not hold.
	stories := filepath.Join(root, "g", "pods", "p1", "tpm", "docs", "specs", "stories")
	os.MkdirAll(stories, 0o755)
	os.WriteFile(filepath.Join(stories, "st001-x.md"), []byte("---\nid: st001-x\ntitle: X\nstatus: agreed\n---\n\n<!-- field: context | form: \"tree\" -->\n## Context\n\n- owner: o0\n- parent: https://example.com/g/analytics/model-evaluation/-/blob/main/docs/specs/features/ft009-gone.md\n- as_a: a\n- i_want: b\n- so_that: c\n- points: 3\n"), 0o644)
	// Another story pins a repository the registry does not name.
	os.WriteFile(filepath.Join(stories, "st002-y.md"), []byte("---\nid: st002-y\ntitle: Y\nstatus: agreed\n---\n\n<!-- field: context | form: \"tree\" -->\n## Context\n\n- owner: o0\n- parent: https://example.com/g/elsewhere/thing/-/blob/main/docs/specs/epics/ep001-e.md\n- as_a: a\n- i_want: b\n- so_that: c\n- points: 3\n"), 0o644)

	f := fake.New()
	r, err := Reconcile(f, system, false)
	if err != nil {
		t.Fatal(err)
	}
	tpm := byReconcileProject(t, r, "g/pods/p1/tpm")
	if !hasFinding(tpm, "st001-x", "docs/specs/features/ft009-gone.md", "does not hold") {
		t.Errorf("no dead-pin finding: %+v", tpm.Findings)
	}
	if !hasFinding(tpm, "st002-y", "g/elsewhere/thing", "registry does not name") {
		t.Errorf("no unregistered-pin finding: %+v", tpm.Findings)
	}
	me := byReconcileProject(t, r, "g/analytics/model-evaluation")
	if !hasFinding(me, "in009-missing", "does not hold") {
		t.Errorf("no missing-initiative finding: %+v", me.Findings)
	}
	issues, _ := f.ListIssues("g/program", tracker.IssueQuery{State: "opened"})
	if len(issues) != 2 {
		t.Fatalf("%d issues, want one per drifted repository: %+v", len(issues), issues)
	}
}

// Where the host lists the group's projects, one the registry does not
// name is a finding with onboard as the remedy; where it cannot, the
// output says so (GH-2822).
func TestReconcileSweepsTheGroupWhereTheHostListsIt(t *testing.T) {
	_, _, system := checkoutsProgram(t, "g/pods/p1/tpm")
	if _, err := Checkouts(system, false); err != nil {
		t.Fatal(err)
	}
	f := fake.New()
	f.Projects["g"] = []string{"g/program", "g/pods/p1/tpm", "g/pods/p9/stray", "g/reports"}
	r, err := Reconcile(f, system, false)
	if err != nil {
		t.Fatal(err)
	}
	group := byReconcileProject(t, r, "g")
	if group.Action != "filed" || !hasFinding(group, "g/pods/p9/stray", "onboard") || !hasFinding(group, "g/reports") || hasFinding(group, "g/pods/p1/tpm,") {
		t.Errorf("the group sweep = %+v", group)
	}

	type noList struct{ tracker.Tracker }
	r, err = Reconcile(noList{fake.New()}, system, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(r.Notes, "\n"), "does not list a group's projects") {
		t.Errorf("no note for a host that cannot list: %v", r.Notes)
	}
}

// A dry run reports every finding and files nothing (GH-2822).
func TestReconcileDryRunFilesNothing(t *testing.T) {
	_, origins, system := checkoutsProgram(t, "g/pods/p1/tpm")
	if _, err := Checkouts(system, false); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(filepath.Join(origins, "g", "pods", "p1"))
	f := fake.New()
	r, err := Reconcile(f, system, true)
	if err != nil {
		t.Fatal(err)
	}
	e := byReconcileProject(t, r, "g/pods/p1/tpm")
	if !r.DryRun || e.Action != "filed" || !strings.Contains(e.Detail, "would file") {
		t.Fatalf("dry run = %+v", r)
	}
	if f.Writes != 0 {
		t.Errorf("a dry run wrote %d times to the tracker", f.Writes)
	}
	if h := r.Human(); !strings.Contains(h, "(dry run)") || !strings.Contains(h, "unreachable") {
		t.Errorf("Human = %q", h)
	}
}

// A scrum repository's stories carry their issues record, and reconcile
// files the drift (srd043 R8.4): an implementing issue with no entry, an
// entry whose state lags the tracking system, and an entry whose issue it
// does not show are each findings naming scrum-master backlog record; a
// recorded story reconciles clean.
func TestReconcileFindsLedgerDrift(t *testing.T) {
	_, _, system := checkoutsProgram(t, "g/pods/p1/tpm")
	if _, err := Checkouts(system, false); err != nil {
		t.Fatal(err)
	}
	scrum := filepath.Join(system, "..", "pods", "p1", "tpm")
	story := filepath.Join(scrum, "docs", "specs", "stories", "st001-score.md")
	os.MkdirAll(filepath.Dir(story), 0o755)
	doc := []byte("---\nid: st001-score\ntitle: Score\n---\n\n<!-- field: problem -->\n## Problem\n\nP.\n")
	stale := backlog.WriteLedger(doc, []backlog.LedgerEntry{
		{Ref: "g/pods/p1/tpm#5", State: "open", Opened: "2026-11-01"},
		{Ref: "g/pods/p1/tpm#99", State: "open"},
	})
	if err := os.WriteFile(story, stale, 0o644); err != nil {
		t.Fatal(err)
	}
	f := fake.New()
	f.Issues["g/pods/p1/tpm"] = []*tracker.Issue{
		{Project: "g/pods/p1/tpm", IID: 5, Title: "Build it", State: "closed", Description: "Implements: st001-score",
			Assignees: []string{"ana"}, CreatedAt: "2026-11-01T08:00:00Z", ClosedAt: "2026-11-03T08:00:00Z"},
		{Project: "g/pods/p1/tpm", IID: 6, Title: "Check it", State: "opened", Description: "Implements: st001-score",
			Author: "bo", CreatedAt: "2026-11-02T08:00:00Z"},
	}
	r, err := Reconcile(f, system, false)
	if err != nil {
		t.Fatal(err)
	}
	e := byReconcileProject(t, r, "g/pods/p1/tpm")
	if !hasFinding(e, "g/pods/p1/tpm#6 implements st001-score with no entry", "backlog record") {
		t.Errorf("no finding for the unrecorded issue: %+v", e.Findings)
	}
	if !hasFinding(e, "holds g/pods/p1/tpm#5 as open", "closed on 2026-11-03") {
		t.Errorf("no finding for the lagging entry: %+v", e.Findings)
	}
	if !hasFinding(e, "names g/pods/p1/tpm#99", "does not show") {
		t.Errorf("no finding for the gone issue: %+v", e.Findings)
	}
	// Recorded, the story reconciles clean.
	recorded := backlog.WriteLedger(doc, []backlog.LedgerEntry{
		backlog.EntryFor("g/pods/p1/tpm", *f.Issues["g/pods/p1/tpm"][0]),
		backlog.EntryFor("g/pods/p1/tpm", *f.Issues["g/pods/p1/tpm"][1]),
	})
	if err := os.WriteFile(story, recorded, 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := Reconcile(f, system, false)
	if err != nil {
		t.Fatal(err)
	}
	if e := byReconcileProject(t, again, "g/pods/p1/tpm"); hasFinding(e, "issues record") {
		t.Errorf("a recorded story still drifts: %+v", e.Findings)
	}
}

// accountHost is a host whose group is a whole account, as GitHub's is.
type accountHost struct{ *fake.Fake }

func (accountHost) GroupIsAccount() bool { return true }

// Where a group is a whole account, what else the owner holds is no finding:
// reconcile skips its sweep and says so (GH-2903).
func TestReconcileSkipsTheSweepOfAWholeAccount(t *testing.T) {
	_, system := rollProgram(t, "g/pods/p1/tpm")
	f := fake.New()
	f.Projects = map[string][]string{"g": {"g/system", "g/pods/p1/tpm", "g/someones-notes"}}
	r, err := Reconcile(accountHost{f}, system, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range r.Repositories {
		if strings.Contains(strings.Join(e.Findings, "\n"), "someones-notes") {
			t.Errorf("the sweep reported what else the owner holds: %+v", e)
		}
	}
	if !strings.Contains(strings.Join(r.Notes, "\n"), "is a whole account") {
		t.Errorf("notes = %v", r.Notes)
	}
}
