// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/runtime"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.MergeProfile)
}

// MergeProfile runs the pinned runtime over the merge profile in the rig
// (GH-201, srd013 R5.7) — the first machine with no model state: a popped
// issue with committed work gates, merges, pushes, and closes through the
// three scrum-master words, ending in Done with the issue closed; a branch with
// nothing over the base ends in Refused with scrum-master's reason; and from a
// team checkout beside the system repository's, whose policy requires
// approval, an unapproved branch ends in Refused and merges once a reviewer
// approves it (GH-46) (test: merge profile run, merge profile refusal, team
// merge requires approval; environment: local).
func (Integration) MergeProfile() error {
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
	rel, err := repoRuntime()
	if err != nil {
		return err
	}
	clone, err := r.clone(po1, "po-1")
	if err != nil {
		return err
	}
	file := func(title string) (program.FileResult, error) {
		path := filepath.Join(r.run.Dir, "merge-"+program.Slug(title)+".md")
		writeFileTo(path, "## Requirements\n\n1. Add the note.\n\n## Acceptance Criteria\n\n- The note exists (test: note_exists; environment: local)\n")
		var res program.FileResult
		return res, r.wordsJSON(&res, "issue", "file", "--project", po1, "--title", title, "--body-file", path,
			"--plane", "data", "--kind", "build", "--component", "po-1", "--milestone", rigMilestone)
	}
	pop := func(iid int) (program.PopResult, error) {
		var popped program.PopResult
		err := r.wordsJSON(&popped, "issue", "pop", "--project", po1, "--issue", fmt.Sprint(iid), "--dir", clone)
		return popped, err
	}
	c := &check{}

	// The full delivery: Gate → Deliver → Close → Done, and the issue closes.
	work, err := file("Merge profile delivery")
	if err != nil {
		return err
	}
	popped, err := pop(work.IID)
	if err != nil {
		return err
	}
	writeFileTo(filepath.Join(popped.Worktree, "NOTE.md"), "# The note\n")
	if err := r.commitPush(popped.Worktree, "Add the note", "NOTE.md"); err != nil {
		return err
	}
	out, runErr := r.runMergeAgent(rel, po1, popped.Worktree, work.IID)
	c.expect(runErr == nil, "the delivery run failed: %v: %s", runErr, tail16(out))
	c.expect(strings.Contains(out, "final machine state: Done"), "the run must end in Done: %s", tail16(out))
	c.expect(r.closed(po1, work.IID), "the delivery must close %s#%d", po1, work.IID)
	// The run reports every word, the gate's audit among them (GH-49).
	c.expect(strings.Contains(out, `"audit_reason"`) && strings.Contains(out, `"merged": true`) && strings.Contains(out, `"closed"`),
		"the delivery run must report the gate, the merge, and the close: %s", out)

	// The refusal: a branch with nothing over the base ends in Refused.
	empty, err := file("Merge profile refusal")
	if err != nil {
		return err
	}
	popped, err = pop(empty.IID)
	if err != nil {
		return err
	}
	out, runErr = r.runMergeAgent(rel, po1, popped.Worktree, empty.IID)
	c.expect(runErr != nil, "a refused run must not report success: %s", tail16(out))
	c.expect(strings.Contains(out, "final machine state: Refused"), "the run must end in Refused: %s", tail16(out))
	c.expect(strings.Contains(out, "no commits over"), "the Refused state must carry scrum-master's reason: %s", tail16(out))
	c.expect(!r.closed(po1, empty.IID), "the refused issue must stay open")

	// The system's policy governs a team repository's merge (GH-46): the
	// system checkout holds the trackers document and a policy requiring
	// approval, and the team's checkout sits beside it.
	system, err := r.project("system")
	if err != nil {
		return err
	}
	sys := r.checkout(system)
	if err := gitIn(sys, "fetch", "-q", "origin"); err != nil {
		return err
	}
	if err := gitIn(sys, "checkout", "-q", "-B", "main", "origin/main"); err != nil {
		return err
	}
	writeFileTo(filepath.Join(sys, "control", "trackers.md"), "---\nid: trackers\ntitle: Tracking systems\n---\n\n<!-- field: version -->\n## Version\n\n1\n\n"+
		"<!-- field: default -->\n## Default\n\n<!-- field: system -->\n### System\n\nbd\n\n<!-- field: tracker_repository -->\n### Tracker repository\n\n"+
		filepath.Join(r.run.Dir, "tracker.git")+"\n")
	writeFileTo(filepath.Join(sys, program.PolicyFile), "---\nid: policy\ntitle: Rig policy\n---\n\n<!-- field: admission -->\n## Admission\n\n"+
		"<!-- field: require_approval -->\n### Require approval\n\ntrue\n")
	if err := r.commitPush(sys, "Require approval before merge", "control/trackers.md", program.PolicyFile); err != nil {
		return err
	}
	team := filepath.Join(r.run.Dir, "checkouts", "po-1-team")
	if err := gitIn(r.run.Dir, "clone", "-q", r.origin(po1), team); err != nil {
		return err
	}
	governed, err := file("Merge profile under the system policy")
	if err != nil {
		return err
	}
	if err := r.wordsJSON(&popped, "issue", "pop", "--project", po1, "--issue", fmt.Sprint(governed.IID), "--dir", team); err != nil {
		return err
	}
	writeFileTo(filepath.Join(popped.Worktree, "GOVERNED.md"), "# Governed\n")
	if err := r.commitPush(popped.Worktree, "Add the governed note", "GOVERNED.md"); err != nil {
		return err
	}
	out, runErr = r.runMergeAgent(rel, po1, popped.Worktree, governed.IID)
	c.expect(runErr != nil && strings.Contains(out, "final machine state: Refused") && strings.Contains(out, "approv"),
		"an unapproved team merge under the system's policy must end in Refused for the approval: %s", tail16(out))
	c.expect(!r.closed(po1, governed.IID), "the unapproved team merge must leave its issue open")
	if err := r.approve(po1, po1, governed.IID, ""); err != nil {
		return err
	}
	out, runErr = r.runMergeAgent(rel, po1, popped.Worktree, governed.IID)
	c.expect(runErr == nil && strings.Contains(out, "final machine state: Done"), "the approved team merge must end in Done: %v: %s", runErr, tail16(out))
	c.expect(r.closed(po1, governed.IID), "the approved team merge must close %s#%d", po1, governed.IID)
	return c.err("mergeProfile")
}

// runMergeAgent runs the release's agent over the merge profile for one
// issue, from its worktree, with the rig's tracker — the environment the profile's declarations name —
// and no scrum-master on PATH: the agent serves the words in process (GH-28).
func (r *rigEnv) runMergeAgent(rel runtime.Release, project, worktree string, iid int) (string, error) {
	outDir, err := os.MkdirTemp(r.run.Dir, "merge-agent-*")
	if err != nil {
		return "", err
	}
	cmd := exec.Command(rel.Agent,
		"--profile", filepath.Join(r.root, "plugin", "agents", "merge", "profile.yaml"),
		"--core-root", rel.CoreRoot(),
		"--directory", worktree,
		"--output", outDir,
	)
	cmd.Dir = worktree
	cmd.Env = append(os.Environ(),
		"PATH="+pathWithout("tpm", "scrum-master"),
		"SCRUM_MASTER_TRACKER="+filepath.Join(r.run.Dir, "tracker.git"),
		"SCRUM_MASTER_TRACKER_CLONE="+filepath.Join(r.run.Dir, "tracker-"+r.user),
		"SCRUM_MASTER_USER="+r.user,
		"SCRUM_MASTER_WEB_BASE="+r.webBase,
		"SCRUM_MASTER_PROJECT="+project,
		"SCRUM_MASTER_ISSUE="+fmt.Sprint(iid),
	)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	return out.String(), err
}

// tail16 is the last part of a run's output, enough to carry its terminal
// state and reason into a finding.
func tail16(out string) string {
	const n = 800
	if len(out) <= n {
		return out
	}
	return "…" + out[len(out)-n:]
}
