// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.DocsChange)
}

// rigAudit is the corpus gate the rig's repositories run: they carry no
// magefile, so a marker file stands in for a failing mage audit.
const rigAudit = "test ! -e docs/BROKEN"

// DocsChange works a kind::spec design change in po-1 and a planning change in the system repository through the audit gate, and proves who may file each (test-rel00.2-docs-change).
func (Integration) DocsChange() error {
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
	if err := r.milestone(rigMilestone, rigDue); err != nil {
		return err
	}
	if _, err := r.runWords("issue", "labels", "--group", r.group); err != nil {
		return err
	}
	policy := func(filer string) error {
		return r.seed(system, map[string]string{
			"docs/specs/software-requirements/srd010-po-1.md": subsystemSRD("srd010-po-1", "po-1", r.webBase+"/"+po1),
			program.PolicyFile: "---\nid: policy\ntitle: Rig policy\n---\n\n<!-- field: admission -->\n## Admission\n\n" +
				"<!-- field: require_milestone -->\n### Require milestone\n\ntrue\n\n" +
				"<!-- field: management_filers -->\n### Management filers\n\n- " + filer + "\n",
		})
	}
	if err := policy("someone-else"); err != nil {
		return err
	}
	c := &check{}
	run := r.run.Dir
	body := "## Requirements\n\n1. Change the document.\n\n## Acceptance Criteria\n\n- mage audit passes (test: mage_audit; environment: local)\n"
	writeFileTo(filepath.Join(run, "docs-change.md"), body)
	file := func(project, title, path string, extra ...string) (program.FileResult, error) {
		var res program.FileResult
		args := append([]string{"issue", "file", "--project", project, "--system", system, "--ref", seedBranch,
			"--title", title, "--body-file", filepath.Join(run, "docs-change.md"), "--path", path}, extra...)
		return res, r.wordsJSON(&res, args...)
	}

	// A design change: kind::spec, anyone's, gated by the audit.
	spec, err := file(po1, "Split the scoring use case", "docs/specs/use-cases/rel01.0-uc001-x.md", "--milestone", rigMilestone)
	if err != nil {
		return err
	}
	c.expect(spec.Plane == "data" && spec.Kind == "spec" && spec.Lint.Admitted, "spec issue: %+v", spec)
	if err := r.workDocsChange(c, po1, "po-1", spec.IID, "docs/specs/use-cases/rel01.0-uc001-x.md", true); err != nil {
		return err
	}

	// A planning change: refused while the policy names another filer.
	_, err = file(system, "Move release 01.1", "docs/road-map.md")
	c.expect(err != nil && strings.Contains(err.Error(), "may not file a plane::management issue"), "non-lead planning issue: %v", err)
	if err := policy(r.user); err != nil {
		return err
	}
	plan, err := file(system, "Move release 01.1", "docs/road-map.md")
	if err != nil {
		return err
	}
	c.expect(plan.Plane == "management" && plan.Kind == "", "planning issue: %+v", plan)
	i, err := r.issue(system, plan.IID)
	if err != nil {
		return err
	}
	c.expect(slices.Contains(i.Labels, "plane::management") && !slices.ContainsFunc(i.Labels, func(l string) bool { return strings.HasPrefix(l, "kind::") }), "planning labels %v", i.Labels)
	if err := r.workDocsChange(c, system, "system", plan.IID, "docs/road-map.md", false); err != nil {
		return err
	}
	return c.err("docsChange")
}

// workDocsChange pops an issue, changes path under docs/ in its worktree,
// and delivers it through the audit gate with scrum-master issue merge. With
// failFirst, the first commit carries the marker that fails the audit, and
// merge must refuse it with nothing merged.
func (r *rigEnv) workDocsChange(c *check, project, name string, iid int, path string, failFirst bool) error {
	clone, err := r.clone(project, name)
	if err != nil {
		return err
	}
	var popped program.PopResult
	if err := r.wordsJSON(&popped, "issue", "pop", "--project", project, "--issue", fmt.Sprint(iid), "--dir", clone); err != nil {
		return err
	}
	wt := popped.Worktree
	writeFileTo(filepath.Join(wt, path), "---\nid: changed-by-rig\n---\n")
	files := []string{path}
	if failFirst {
		writeFileTo(filepath.Join(wt, "docs/BROKEN"), "the audit fails while this file exists\n")
		files = append(files, "docs/BROKEN")
	}
	if err := r.commitPush(wt, "Change "+path, files...); err != nil {
		return err
	}
	merge := func() (program.MergeResult, error) {
		var res program.MergeResult
		return res, r.wordsJSON(&res, "issue", "merge", "--project", project, "--issue", fmt.Sprint(iid),
			"--branch", popped.Branch, "--dir", wt, "--audit", rigAudit)
	}
	if failFirst {
		_, err := merge()
		c.expect(err != nil && strings.Contains(err.Error(), "nothing merged"), "%s#%d: failing audit merged: %v", project, iid, err)
		if err := r.git(wt, "rm", "--quiet", "docs/BROKEN"); err != nil {
			return err
		}
		if err := r.commitPush(wt, "Fix the corpus"); err != nil {
			return err
		}
	}
	if err := r.approve(project, project, iid, popped.Branch); err != nil {
		return err
	}
	merged, err := merge()
	if err != nil {
		return err
	}
	c.expect(merged.Merged && merged.Audit, "%s#%d merge: %+v", project, iid, merged)
	c.expect(r.closed(project, iid), "%s#%d still open after its merge", project, iid)
	return nil
}
