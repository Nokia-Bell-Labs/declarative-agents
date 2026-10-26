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
	integrationTargets = append(integrationTargets, Integration{}.ImplementProfile)
}

// ImplementProfile runs the pinned declarative-agents runtime over the
// implement profile in the rig (GH-135, srd036 R6): a pop the admission
// refuses ends the run in the Refused state with scrum-master's reason, and, when
// SCRUM_MASTER_IMPLEMENT_MODEL names a model the OLLAMA_URL provider serves, a full
// run pops, works, merges, and closes an issue (test: implement refusal,
// implement profile run).
func (Integration) ImplementProfile() error {
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
	file := func(title, body string) (program.FileResult, error) {
		path := filepath.Join(r.run.Dir, "implement-"+program.Slug(title)+".md")
		writeFileTo(path, body)
		var res program.FileResult
		return res, r.wordsJSON(&res, "issue", "file", "--project", po1, "--title", title, "--body-file", path,
			"--plane", "data", "--kind", "build", "--component", "po-1", "--milestone", rigMilestone)
	}
	c := &check{}

	// A data issue admission does not admit: the pop's refusal is terminal.
	triaged, err := file("Implement profile refusal", "## Requirements\n\n1. Something.\n")
	if err != nil {
		return err
	}
	out, runErr := r.runImplementAgent(rel, po1, clone, clone, triaged.IID, "")
	c.expect(runErr != nil, "a refused run must not report success: %s", tail(out))
	c.expect(strings.Contains(out, "Refused"), "the run must end in the Refused state: %s", tail(out))
	c.expect(strings.Contains(out, "not admitted"), "the Refused state must carry scrum-master's reason: %s", tail(out))

	model := os.Getenv("SCRUM_MASTER_IMPLEMENT_MODEL") //nolint:forbidigo // The opt-in rig reads its tokens and endpoints from the environment (eng01).
	if model == "" {
		fmt.Println("implement profile full run skipped: set SCRUM_MASTER_IMPLEMENT_MODEL (and OLLAMA_URL) to a model that can work the rig issue")
		return c.err("implementProfile")
	}

	work, err := file("Add grader notes",
		"## Requirements\n\n1. Create GRADER.md at the repository root with the heading '# Grader notes'.\n\n## Acceptance Criteria\n\n- GRADER.md exists with the heading (test: notes_exist; environment: local)\n")
	if err != nil {
		return err
	}
	// The runtime's workspace is fixed at start while pop opens the worktree
	// mid-run, so the runner pre-pops — pop is idempotent (srd036 R2.2) — and
	// starts the agent in the worktree; the machine's Pop stays the claim
	// gate (GH-135 pilot finding).
	var popped program.PopResult
	if err := r.wordsJSON(&popped, "issue", "pop", "--project", po1, "--issue", fmt.Sprint(work.IID), "--dir", clone); err != nil {
		return err
	}
	out, runErr = r.runImplementAgent(rel, po1, clone, popped.Worktree, work.IID, model)
	c.expect(runErr == nil, "the full run failed: %v: %s", runErr, tail(out))
	c.expect(strings.Contains(out, "Done"), "the run must end in the Done state: %s", tail(out))
	c.expect(r.closed(po1, work.IID), "the merge must close %s#%d", po1, work.IID)
	return c.err("implementProfile")
}

// runImplementAgent runs the release's agent over the implement profile for
// one issue, as the rig user, with the rig's tracker — the environment the profile's declarations name —
// and no scrum-master on PATH: the agent serves the words in process (GH-28).
func (r *rigEnv) runImplementAgent(rel runtime.Release, project, checkout, workspace string, iid int, model string) (string, error) {
	outDir, err := os.MkdirTemp(r.run.Dir, "implement-agent-*")
	if err != nil {
		return "", err
	}
	cmd := exec.Command(rel.Agent,
		"--profile", filepath.Join(r.root, "plugin", "agents", "implement", "profile.yaml"),
		"--core-root", rel.CoreRoot(),
		"--directory", workspace,
		"--output", outDir,
	)
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(),
		"PATH="+pathWithout("tpm", "scrum-master"),
		"SCRUM_MASTER_TRACKER="+filepath.Join(r.run.Dir, "tracker.git"),
		"SCRUM_MASTER_TRACKER_CLONE="+filepath.Join(r.run.Dir, "tracker-"+r.user),
		"SCRUM_MASTER_USER="+r.user,
		"SCRUM_MASTER_WEB_BASE="+r.webBase,
		"SCRUM_MASTER_PROJECT="+project,
		"SCRUM_MASTER_ISSUE="+fmt.Sprint(iid),
		"SCRUM_MASTER_CHECKOUT="+checkout,
	)
	if model != "" {
		cmd.Env = append(cmd.Env, "SCRUM_MASTER_MODEL="+model)
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	return out.String(), err
}

// tail is the last part of a run's output, enough to carry its terminal
// state and reason into a finding.
func tail(out string) string {
	const n = 800
	if len(out) <= n {
		return out
	}
	return "…" + out[len(out)-n:]
}
