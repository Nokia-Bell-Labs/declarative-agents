// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"path/filepath"
	"slices"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
)

func init() {
	integrationTargets = append(integrationTargets,
		Integration{}.Labels,
		Integration{}.Push,
	)
}

// Labels runs scrum-master issue labels on the rig group twice: the first run creates the schema; the second changes nothing (test-rel00.0-push).
func (Integration) Labels() error {
	r, err := newRigEnv()
	if err != nil {
		return err
	}
	c := &check{}
	group := r.group
	var first, second []program.LabelResult
	if err := r.wordsJSON(&first, "issue", "labels", "--group", group); err != nil {
		return err
	}
	count := func(res []program.LabelResult, action string) int {
		n := 0
		for _, x := range res {
			if x.Action == action {
				n++
			}
		}
		return n
	}
	c.expect(count(first, "created") > 0, "first run created nothing: %+v", first)
	c.expect(count(first, "updated") == 0, "first run updated labels without --update")
	if err := r.wordsJSON(&second, "issue", "labels", "--group", group); err != nil {
		return err
	}
	c.expect(count(second, "created") == 0 && count(second, "updated") == 0, "second run changed labels: %+v", second)

	labels, err := r.tr.GroupLabels(group)
	if err != nil {
		return err
	}
	var names []string
	for _, l := range labels {
		names = append(names, l.Name)
	}
	for _, want := range []string{"kind::spec", "kind::build", "needs-triage", "plane::data", "type::recurring"} {
		c.expect(slices.Contains(names, want), "group lacks %s", want)
	}
	return c.err("labels")
}

// Push files a project request and a docs/ request through scrum-master issue file --path, without --milestone, and proves plane, kind, component, the active sprint milestone, admission, and no assignee (test-rel00.0-push).
func (Integration) Push() error {
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
	if err := r.seed(system, map[string]string{
		"docs/specs/software-requirements/srd010-po-1.md": subsystemSRD("srd010-po-1", "po-1", r.webBase+"/"+po1),
	}); err != nil {
		return err
	}
	if err := r.milestone(rigMilestone, rigDue); err != nil {
		return err
	}
	if _, err := r.runWords("issue", "labels", "--group", r.group); err != nil {
		return err
	}
	c := &check{}
	dir := r.run.Dir
	body := "## Requirements\n\n1. Score one agent's trajectories.\n\n## Acceptance Criteria\n\n- One agent is scored (test: grader_scores_agent; environment: dev)\n"
	writeFileTo(filepath.Join(dir, "push.md"), body)
	file := func(title string, extra ...string) (program.FileResult, error) {
		var res program.FileResult
		args := append([]string{"issue", "file", "--project", po1, "--system", system, "--ref", seedBranch,
			"--title", title, "--body-file", filepath.Join(dir, "push.md")}, extra...)
		return res, r.wordsJSON(&res, args...)
	}

	project, err := file("Grader for one agent", "--path", "internal/grader/grader.go", "--kind", "build")
	if err != nil {
		return err
	}
	c.expect(project.Plane == "data" && project.Kind == "build", "project request: plane %s kind %s", project.Plane, project.Kind)
	design, err := file("Split the scoring use case", "--path", "docs/specs/use-cases/rel01.0-uc001-x.md")
	if err != nil {
		return err
	}
	c.expect(design.Plane == "data" && design.Kind == "spec", "design request: plane %s kind %s", design.Plane, design.Kind)

	for _, res := range []program.FileResult{project, design} {
		c.expect(res.Lint.Admitted, "#%d not admitted: %v", res.IID, res.Lint.Reasons)
		c.expect(res.Component == "po-1", "#%d reports component %q, want po-1", res.IID, res.Component)
		c.expect(res.Milestone == rigMilestone, "#%d milestone %q (%s), want the active sprint %s", res.IID, res.Milestone, res.MilestoneReason, rigMilestone)
		i, err := r.issue(po1, res.IID)
		if err != nil {
			return err
		}
		c.expect(len(i.Assignees) == 0, "#%d is assigned", res.IID)
		c.expect(slices.Contains(i.Labels, "component::po-1"), "#%d labels %v lack component::po-1", res.IID, i.Labels)
		c.expect(slices.Contains(i.Labels, "plane::data"), "#%d labels %v lack plane::data", res.IID, i.Labels)
		c.expect(slices.Contains(i.Labels, "op::file"), "#%d labels %v lack op::file", res.IID, i.Labels)
	}
	return c.err("push")
}
