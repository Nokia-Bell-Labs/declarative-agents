// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/events"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/goals"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/plan"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/release"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintend"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.Plan, Integration{}.SprintEnd)
}

// planPolicy is the system repository's policy for release 01.2's targets:
// the documents-only corpus gate the plan tag passes, here the rig's audit,
// and the grader gate a Product Owner's sprint-end release passes.
const planPolicy = `---
id: policy
title: Rig policy
---

<!-- field: releases -->
## Releases

<!-- field: tag_format -->
### Tag format

{artifact}/v{version}

<!-- field: definition_of_done -->
## Definition of done

<!-- field: corpus -->
### Corpus

| kind | environment | command |
| --- | --- | --- |
| custom | local | ` + rigAudit + ` |

<!-- field: grader -->
### Grader

| kind | environment | command |
| --- | --- | --- |
| custom | local | test -d .git |
`

// obj002 is a second agreed objective document, so the next release has an
// objective item to place.
const obj002 = `---
id: obj002-fast-scoring
title: Fast scoring
status: agreed
---

<!-- field: context | form: "tree" -->
## Context

- owner: technical program manager
- horizon: 2026-W43..2027-W03

<!-- field: problem -->
## Problem

Scoring takes a day.

<!-- field: goals -->
## Goals

- G1: A score is ready within an hour.

<!-- req: R1 -->
## R1 - Score in an hour

<!-- field: applicability | form: "tree" -->
### Applicability

- due: "2026-11-27"

<!-- req: R1.1 -->
### R1.1

A score is ready within an hour of a trajectory.

<!-- field: non_goals -->
## Non-goals

- Scoring models.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

A score is ready within an hour.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)

<!-- field: tests -->
#### Tests

- scorer_meets_the_hour

<!-- field: applicability -->
#### Applicability

dev
`

// Plan plans release 01.1 in the system repository: candidates, the road-map write merged through a planning issue with scrum-master issue merge, the plan tagged behind the documents-only gate, the period pushed (milestone, sprint issue, sprint-end issues) once, and tasks reading the plan at its tag (test-rel01.2-sprint-end).
func (Integration) Plan() error {
	e, err := newWeeklyEnv("reports-plan")
	if err != nil {
		return err
	}
	r, system := e.rigEnv, e.system
	if err := r.seed(system, map[string]string{program.PolicyFile: planPolicy, "docs/specs/objectives/obj002-fast-scoring.md": obj002}); err != nil {
		return err
	}
	po2, err := r.project("product-owners/po-2")
	if err != nil {
		return err
	}
	c := &check{}
	clone, err := r.clone(system, "system-plan")
	if err != nil {
		return err
	}
	const branch = "plan-01-1"
	if err := r.git(clone, "fetch", "--quiet", "origin"); err != nil {
		return err
	}
	// The plan branch starts from main, which earlier targets may have
	// changed, and takes the seed's fixtures as its first commit, so its
	// merge applies to main whatever main holds.
	if err := r.git(clone, "checkout", "--quiet", "-B", branch, "origin/main"); err != nil {
		return err
	}
	if err := r.git(clone, "checkout", "origin/"+seedBranch, "--", "."); err != nil {
		return err
	}
	if out, _ := (program.ExecGit{}).Run(clone, "status", "--porcelain"); strings.TrimSpace(out) != "" {
		if err := r.git(clone, "commit", "--quiet", "-m", "Bring the seed fixtures"); err != nil {
			return err
		}
	}
	control := []string{"--system", system, "--control", system, "--ref", seedBranch, "--dir", clone}

	var cand plan.Candidates
	if err := r.wordsJSON(&cand, append([]string{"plan", "candidates"}, control...)...); err != nil {
		return err
	}
	c.expect(cand.Version == "01.1" && cand.Tag == "corpus/v0.1.0" && len(cand.Items) > 0 && cand.Items[0].ID == "obj002-fast-scoring#R1.1",
		"candidates: %+v", cand)

	body := filepath.Join(r.run.Dir, "plan.md")
	writeFileTo(body, "## Requirements\n\n1. Plan release 01.1.\n\n## Acceptance Criteria\n\n- mage audit passes (test: mage_audit; environment: local)\n")
	var filed program.FileResult
	if err := r.wordsJSON(&filed, "issue", "file", "--project", system, "--system", system, "--ref", seedBranch,
		"--title", "Plan release 01.1", "--body-file", body, "--path", "docs/road-map.md"); err != nil {
		return err
	}
	issue := fmt.Sprintf("%s#%d", system, filed.IID)

	var written plan.Written
	if err := r.wordsJSON(&written, append([]string{"plan", "write", "--version", "01.1", "--tag", cand.Tag,
		"--items", "obj002-fast-scoring#R1.1", "--issue", issue}, control...)...); err != nil {
		return err
	}
	c.expect(written.Action == "added" && slices.Equal(written.Release.Items, []string{"obj002-fast-scoring#R1.1"}), "write: %+v", written)
	if err := r.git(clone, "add", plan.RoadMap); err != nil {
		return err
	}
	if err := r.git(clone, "commit", "--quiet", "-m", "Plan release 01.1"); err != nil {
		return err
	}
	if err := r.git(clone, "push", "--quiet", "--force", "-u", "origin", branch); err != nil {
		return err
	}
	if err := r.approve(system, system, filed.IID, branch); err != nil {
		return err
	}
	var merged program.MergeResult
	if err := r.wordsJSON(&merged, "issue", "merge", "--project", system, "--issue", fmt.Sprint(filed.IID),
		"--branch", branch, "--dir", clone, "--audit", rigAudit); err != nil {
		return err
	}
	c.expect(merged.Merged && merged.Audit, "plan merge: %+v", merged)
	c.expect(r.closed(system, filed.IID), "%s still open after the plan merged", issue)

	// The plan is published at its tag, gated by the documents-only check.
	if err := r.git(clone, "checkout", "--quiet", "-B", "main", "origin/main"); err != nil {
		return err
	}
	if err := r.git(clone, "pull", "--quiet", "origin", "main"); err != nil {
		return err
	}
	gateArgs := []string{"--artifact", "corpus", "--version", "0.1.0", "--issue", issue, "--control", system, "--ref", seedBranch, "--dir", clone}
	var gate release.ReleaseGateResult
	if err := r.wordsJSON(&gate, append([]string{"release", "gate"}, gateArgs...)...); err != nil {
		return err
	}
	var tagged release.TagResult
	if err := r.wordsJSON(&tagged, append([]string{"release", "tag"}, gateArgs...)...); err != nil {
		return err
	}
	c.expect(gate.Passed && tagged.Pushed && tagged.Tag == cand.Tag, "plan tag: gate %+v tag %+v", gate, tagged)

	end := time.Now().AddDate(0, 0, 21).Format(time.DateOnly)
	push := append([]string{"plan", "push", "--version", "01.1", "--end", end, "--group", r.group}, control...)
	var pushed plan.PushResult
	if err := r.wordsJSON(&pushed, push...); err != nil {
		return err
	}
	repos := []string{}
	for _, s := range pushed.SprintEnds {
		repos = append(repos, s.Project)
		c.expect(s.Action == "filed" && s.Admitted, "sprint-end issue %+v", s)
	}
	c.expect(pushed.Milestone == "created" && pushed.Sprint.Action == "filed", "first push: %+v", pushed)
	c.expect(slices.Contains(repos, e.po1) && slices.Contains(repos, po2), "sprint-end issues in %v, want %s and %s", repos, e.po1, po2)
	if err := r.wordsJSON(&pushed, push...); err != nil {
		return err
	}
	c.expect(pushed.Milestone == "unchanged" && pushed.Sprint.Action == "unchanged" && pushed.SprintEnds[0].Action == "unchanged",
		"second push changed something: %+v", pushed)

	// tasks reads the plan at its tag.
	var p goals.Plan
	if err := r.wordsJSON(&p, "goals", "plan", "--system", system, "--release", "01.1", "--ref", "main"); err != nil {
		return err
	}
	c.expect(p.Tag == cand.Tag && len(p.Objectives) == 1 && p.Objectives[0].Key == "obj002-fast-scoring#R1", "tasks plan: %+v", p)
	return c.err("plan")
}

// SprintEnd works po-1's sprint-end issue: facts with the objective issue read as a Sprint Goal and reported complete, the stories done recorded in the summary, a summary merged with scrum-master issue merge that closes the issue and the objective with no met event and no contradiction signal, and a release gated and tagged on main after the merge (test-rel01.2-sprint-end).
func (Integration) SprintEnd() error {
	e, err := newWeeklyEnv("reports-sprint-end")
	if err != nil {
		return err
	}
	r, po1, system := e.rigEnv, e.po1, e.system
	if err := r.seed(system, map[string]string{program.PolicyFile: planPolicy}); err != nil {
		return err
	}
	c := &check{}
	objRef, objIID, err := e.objectiveIssue(po1, "Objective R1 for the sprint end")
	if err != nil {
		return err
	}
	now := time.Now()
	week := reports.ISOWeek(reports.WeekStart(now))
	writeFileTo(filepath.Join(e.clone, reports.ReportPath(reports.WeekStart(now), e.user)), reports.Render(e.user, week,
		[]reports.Entry{{"objective": objRef, "component": "po-1", "status": "done", "complete": "yes", "problems": "none"}}, nil))

	body := filepath.Join(r.run.Dir, "sprint-end.md")
	writeFileTo(body, plan.SprintEndBody("01.0", rigDue))
	var filed program.FileResult
	if err := r.wordsJSON(&filed, "issue", "file", "--project", po1, "--system", system, "--ref", seedBranch, "--control", system,
		"--title", "Recurring: sprint end 01.0", "--body-file", body, "--kind", "release", "--type", "recurring", "--milestone", rigMilestone, "--due", rigDue); err != nil {
		return err
	}
	c.expect(filed.Lint.Admitted, "sprint-end issue not admitted: %+v", filed.Lint)

	clone, err := r.clone(po1, "po-1-sprint-end")
	if err != nil {
		return err
	}
	var facts sprintend.Facts
	if err := r.wordsJSON(&facts, "sprint-end", "facts", "--project", po1, "--reports", e.clone, "--from", week, "--to", week,
		"--artifact", "grader", "--control", system, "--ref", seedBranch, "--dir", clone); err != nil {
		return err
	}
	var obj *sprintend.SprintGoal
	for i := range facts.SprintGoals {
		if facts.SprintGoals[i].Issue == objRef {
			obj = &facts.SprintGoals[i]
		}
	}
	c.expect(obj != nil && obj.ReportedComplete && !obj.Met && obj.Legacy && facts.Recommended != "", "facts: %+v", facts)

	var popped program.PopResult
	if err := r.wordsJSON(&popped, "issue", "pop", "--project", po1, "--issue", fmt.Sprint(filed.IID), "--dir", clone); err != nil {
		return err
	}
	writeFileTo(filepath.Join(popped.Worktree, "docs/sprint-ends/01.0.md"), "# Sprint end 01.0\n\nThe Sprint Goal is reported complete.\n")
	var recorded struct {
		Changed bool `json:"changed"`
	}
	if err := r.wordsJSON(&recorded, "sprint-end", "stories", "--dir", popped.Worktree, "--release", "01.0", "--project", po1, "--done", "st001-score-trajectories"); err != nil {
		return err
	}
	summary, _ := os.ReadFile(filepath.Join(popped.Worktree, "docs/sprint-ends/01.0.md"))
	c.expect(recorded.Changed && strings.Contains(string(summary), "## Stories done\n\n- "+po1+"#st001-score-trajectories\n"), "stories done not recorded:\n%s", summary)
	if err := r.commitPush(popped.Worktree, "Summarize release 01.0's period", "docs/sprint-ends/01.0.md"); err != nil {
		return err
	}
	if err := r.approve(po1, po1, filed.IID, popped.Branch); err != nil {
		return err
	}
	var merged program.MergeResult
	if err := r.wordsJSON(&merged, "issue", "merge", "--project", po1, "--issue", fmt.Sprint(filed.IID),
		"--branch", popped.Branch, "--dir", popped.Worktree, "--audit", rigAudit, "--closes", objRef); err != nil {
		return err
	}
	c.expect(merged.Merged && slices.Contains(merged.Closed, objRef), "sprint-end merge does not close %s: %+v", objRef, merged)
	c.expect(slices.Contains(merged.Reported, objRef), "sprint-end merge records no reported_complete on %s: %+v", objRef, merged)
	c.expect(r.closed(po1, filed.IID), "sprint-end issue still open after its merge")
	c.expect(r.closed(po1, objIID), "%s still open after the sprint-end merge", objRef)
	notes, err := r.notes(po1, objIID)
	if err != nil {
		return err
	}
	for _, n := range notes {
		if ev, ok := events.Parse(n.Body); ok && ev.Name == "met" {
			c.expect(false, "%s has a met event", objRef)
		}
	}
	var agg reports.Aggregation
	if err := r.wordsJSON(&agg, "reports", "aggregate", "--dir", e.clone, "--from", week, "--to", week); err != nil {
		return err
	}
	for _, s := range agg.Signals {
		c.expect(!(s.Kind == reports.SignalContradiction && s.Subject == objRef), "contradiction after the sprint-end close: %+v", s)
	}

	// The confirmed release is gated and tagged on main after the merge.
	if err := r.git(clone, "fetch", "--quiet", "origin"); err != nil {
		return err
	}
	if err := r.git(clone, "checkout", "--quiet", "-B", "main", "origin/main"); err != nil {
		return err
	}
	ref := fmt.Sprintf("%s#%d", po1, filed.IID)
	gateArgs := []string{"--artifact", "grader", "--version", facts.Recommended, "--issue", ref, "--control", system, "--ref", seedBranch, "--dir", clone}
	var gate release.ReleaseGateResult
	if err := r.wordsJSON(&gate, append([]string{"release", "gate"}, gateArgs...)...); err != nil {
		return err
	}
	var tagged release.TagResult
	if err := r.wordsJSON(&tagged, append([]string{"release", "tag"}, gateArgs...)...); err != nil {
		return err
	}
	head, err := program.ExecGit{}.Run(clone, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	tag, err := r.remoteTag(po1, tagged.Tag)
	c.expect(err == nil && gate.Passed && tagged.Pushed && tag == strings.TrimSpace(head),
		"release after the merge: gate %+v tag %+v on %v", gate, tagged, err)
	return c.err("sprintEnd")
}
