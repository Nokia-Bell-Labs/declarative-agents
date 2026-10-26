// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/magefile/mage/mg"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/events"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/goals"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/objectives"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Integration holds the release integration targets the test suites name.
// They run against the current rig (mage regression:up), except
// objectivesAudit, which is offline.
type Integration mg.Namespace

func init() {
	integrationTargets = append(integrationTargets,
		Integration{}.ObjectivesAudit,
		Integration{}.Tasks,
		Integration{}.Program,
	)
}

const (
	rigRelease   = "01.0"
	rigTag       = "v0.1.0"
	rigMilestone = "rig-01.0"
	rigDue       = "2026-11-06"
)

// ObjectivesAudit proves the agent audit passes a well-formed objective document and the gate fails a malformed one (test-rel01.0-objectives).
func (Integration) ObjectivesAudit() error {
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	c := &check{}
	audit := func(fixture string) ([]objectives.Finding, error) {
		tmp, err := os.MkdirTemp("", "scrum-master-objectives-*")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tmp)
		if err := runStreaming(root, "cp", "-R", "docs", tmp); err != nil {
			return nil, err
		}
		dst := filepath.Join(tmp, objectives.Dir, filepath.Base(fixture))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		b, err := os.ReadFile(filepath.Join(root, fixture))
		if err != nil {
			return nil, err
		}
		writeFileTo(dst, string(b))
		docs, err := objectives.LoadDir(filepath.Join(tmp, objectives.Dir))
		if err != nil {
			return nil, err
		}
		var findings []objectives.Finding
		for _, d := range docs {
			findings = append(findings, objectives.Validate(d)...)
		}
		if len(findings) == 0 {
			return nil, runCorpusAudit(tmp, runStreaming)
		}
		return findings, nil
	}
	f, err := audit("testdata/objectives/valid/obj001-reliable-agents.md")
	c.expect(err == nil && len(f) == 0, "well-formed document: findings=%v err=%v", f, err)
	malformed, _ := filepath.Glob(filepath.Join(root, "testdata/objectives/malformed/*.md"))
	for _, m := range malformed {
		rel, _ := filepath.Rel(root, m)
		f, err := audit(rel)
		c.expect(err == nil && len(f) > 0, "%s: expected a finding, got findings=%v err=%v", rel, f, err)
	}
	return c.err("objectivesAudit")
}

// Tasks seeds the rig's system repository and proves tasks files one dated, pinned Sprint Goal per team, once (test-rel01.0-objectives, test-rel04.0-agile-hierarchy).
func (Integration) Tasks() error {
	r, err := newRigEnv()
	if err != nil {
		return err
	}
	system, err := r.project("system")
	if err != nil {
		return err
	}
	po1, err := r.project("product-owners/po-1")
	if err != nil {
		return err
	}
	po2, err := r.project("product-owners/po-2")
	if err != nil {
		return err
	}
	if err := r.seedSystem(system, po1, po2); err != nil {
		return err
	}
	c := &check{}
	base := []string{"--system", system, "--release", rigRelease, "--ref", seedBranch}

	var plan goals.Plan
	if err := r.wordsJSON(&plan, append([]string{"goals", "plan"}, base...)...); err != nil {
		return err
	}
	// One entry per item: R1's two items, then R2's one.
	c.expect(len(plan.Objectives) == 3, "plan has %d items, want 3", len(plan.Objectives))
	if len(plan.Objectives) != 3 {
		return c.err("tasks")
	}
	for _, o := range plan.Objectives[:2] {
		c.expect(o.Group == "R1" && o.Repository == po1 && o.Environment == "dev", "R1 item %+v, want recommended %s in dev", o, po1)
	}
	r2 := len(plan.Objectives) - 1
	c.expect(plan.Objectives[r2].Repository == "", "R2 recommended %q, want none", plan.Objectives[r2].Repository)

	// The person confirms R1 and names po-2 for R2, as the skill relays.
	plan.Objectives[r2].Repository = po2
	planFile := filepath.Join(r.run.Dir, "plan.json")
	writeJSONTo(planFile, plan)
	push := append([]string{"goals", "push", "--plan", planFile}, base...)

	if _, err := r.runWords(append(push, "--dry-run")...); err != nil {
		return err
	}
	for _, p := range []string{po1, po2} {
		is, err := r.issues(p)
		c.expect(err == nil && len(is) == 0, "dry run filed %d issues in %s (err %v)", len(is), p, err)
	}

	var pushed []goals.Pushed
	if err := r.wordsJSON(&pushed, push...); err != nil {
		return err
	}
	for _, p := range pushed {
		c.expect(p.Action == "created" && p.Planned, "%s: action %s planned %v reasons %v", p.Key, p.Action, p.Planned, p.Reasons)
	}
	for _, p := range []string{po1, po2} {
		is, err := r.issues(p)
		if err != nil || len(is) != 1 {
			c.expect(false, "%s has %d Sprint Goals (err %v), want 1", p, len(is), err)
			continue
		}
		i := is[0]
		due := i.DueDate
		c.expect(len(i.Assignees) == 0, "%s#%d is assigned", p, i.IID)
		c.expect(slices.Contains(i.Labels, "plane::management") && !slices.Contains(i.Labels, "needs-triage"), "%s#%d labels %v", p, i.IID, i.Labels)
		c.expect(due != "", "%s#%d has no due date", p, i.IID)
		c.expect(strings.Contains(i.Description, "/-/blob/"+rigTag+"/docs/specs/objectives/obj001-reliable-agents.md#R"), "%s#%d Cites not pinned to %s", p, i.IID, rigTag)
		c.expect(strings.Contains(i.Description, "Environment: dev") && strings.Contains(i.Description, "## Notes"), "%s#%d body lacks Environment or Notes", p, i.IID)
		c.expect(strings.HasPrefix(i.Title, "Sprint Goal: ") && strings.Contains(i.Description, "## Sprint Backlog") && strings.Contains(i.Description, "<!-- scrum-master goals sprint-goal release="+rigRelease),
			"%s#%d is not a Sprint Goal: %q", p, i.IID, i.Title)
		ns, _ := r.notes(p, i.IID)
		planned := 0
		for _, n := range ns {
			if e, ok := events.Parse(n.Body); ok && e.Name == "planned" {
				planned++
			}
		}
		c.expect(planned == 1, "%s#%d has %d planned events", p, i.IID, planned)
	}

	var again []goals.Pushed
	if err := r.wordsJSON(&again, push...); err != nil {
		return err
	}
	for _, p := range again {
		c.expect(p.Action == "unchanged" && !p.Planned, "second push: %s %s planned %v", p.Key, p.Action, p.Planned)
	}

	plan.Objectives[r2].Environment = ""
	writeJSONTo(planFile, plan)
	var refused []goals.Pushed
	if err := r.wordsJSON(&refused, push...); err != nil {
		return err
	}
	// One result per outcome and issue (#152): the refused outcome is found
	// by its key, not by its plan entry's index.
	refusedR2 := slices.ContainsFunc(refused, func(p goals.Pushed) bool { return p.Key == plan.Objectives[r2].Key && p.Action == "refused" })
	c.expect(len(refused) < len(plan.Objectives) && refusedR2, "missing environment not refused: %+v", refused)
	return c.err("tasks")
}

// Program proves file and lint against a rig repository: needs-triage with reasons, cleared once fixed, and nothing written on a dry run (test-rel01.0-objectives).
func (Integration) Program() error {
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
	c := &check{}
	dir := r.run.Dir
	bad := "Unpinned work\n\nCites: " + r.webBase + "/" + po1 + "/-/blob/main/README.md#R1.1\n\n## Requirements\n\n1. Do it.\n\n## Acceptance Criteria\n\n- It is done\n"
	good := strings.Replace(strings.Replace(bad, "/-/blob/main/", "/-/blob/v0.0.1/", 1), "- It is done", "- It is done (test: it_is_done; environment: dev)", 1)
	writeFileTo(filepath.Join(dir, "bad.md"), bad)
	file := []string{"issue", "file", "--project", po1, "--title", "Lint probe", "--body-file", filepath.Join(dir, "bad.md"),
		"--plane", "data", "--kind", "build", "--component", "po-1", "--milestone", rigMilestone}

	before, _ := r.issues(po1)
	if _, err := r.runWords(append(file, "--dry-run")...); err != nil {
		return err
	}
	after, _ := r.issues(po1)
	c.expect(len(after) == len(before), "dry-run file created an issue")

	var filed program.FileResult
	if err := r.wordsJSON(&filed, file...); err != nil {
		return err
	}
	c.expect(!filed.Lint.Admitted, "bad issue admitted")
	reasons := strings.Join(filed.Lint.Reasons, "\n")
	c.expect(strings.Contains(reasons, "not pinned") && strings.Contains(reasons, "names no (test: T; environment: E)"), "reasons: %s", reasons)
	i, err := r.issue(po1, filed.IID)
	if err != nil {
		return err
	}
	c.expect(slices.Contains(i.Labels, "needs-triage") && len(i.Assignees) == 0, "after file: labels %v assignees %d", i.Labels, len(i.Assignees))
	ns, _ := r.notes(po1, i.IID)
	c.expect(len(ns) == 1 && strings.Contains(ns[0].Body, "Admission failed"), "lint comments: %d", len(ns))

	if _, err := r.tr.UpdateIssue(po1, i.IID, tracker.IssueUpdate{Description: &good}); err != nil {
		return err
	}
	var linted program.LintResult
	if err := r.wordsJSON(&linted, "issue", "lint", "--project", po1, "--issue", fmt.Sprint(i.IID)); err != nil {
		return err
	}
	c.expect(linted.Admitted, "fixed issue not admitted: %+v", linted)
	i, _ = r.issue(po1, i.IID)
	c.expect(!slices.Contains(i.Labels, "needs-triage"), "needs-triage not cleared: %v", i.Labels)
	return c.err("program")
}

// All runs the rig's full integration suite — up, every target, then down —
// the aggregate the repository's release gate dispatches (GH-190).
func (Integration) All() error { return Regression{}.All() }
