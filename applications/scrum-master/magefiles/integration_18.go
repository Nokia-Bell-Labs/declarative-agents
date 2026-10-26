// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/plan"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/runtime"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.PlanPushProfile)
}

// PlanPushProfile runs the pinned runtime over the plan-push profile in the
// rig (GH-206, srd031 R4.7) — the third model-free machine: a written
// release's milestone, sprint issue, sprint-end issues, and planned events
// land through the four scrum-master words, ending in Done; a second run of the
// machine, and the one-command scrum-master plan push after it, change nothing; a
// road map without the release ends in Refused with scrum-master's reason (test:
// plan push profile run, plan push profile refusal; environment: local).
func (Integration) PlanPushProfile() error {
	e, err := newWeeklyEnv("reports-planpush")
	if err != nil {
		return err
	}
	r, system := e.rigEnv, e.system
	if err := r.seed(system, map[string]string{program.PolicyFile: planPolicy, "docs/specs/objectives/obj002-fast-scoring.md": obj002}); err != nil {
		return err
	}
	rel, err := repoRuntime()
	if err != nil {
		return err
	}
	c := &check{}
	clone, err := r.clone(system, "system-planpush")
	if err != nil {
		return err
	}
	// The checkout holds the seed's fixtures and the written release: push
	// reads the road map from the checkout, so no merge is needed here.
	if err := r.git(clone, "fetch", "--quiet", "origin"); err != nil {
		return err
	}
	if err := r.git(clone, "checkout", "origin/"+seedBranch, "--", "."); err != nil {
		return err
	}
	control := []string{"--system", system, "--control", system, "--ref", seedBranch, "--dir", clone}
	var written plan.Written
	if err := r.wordsJSON(&written, append([]string{"plan", "write", "--version", "01.1", "--tag", "corpus/v0.1.0",
		"--items", "obj002-fast-scoring#R1.1"}, control...)...); err != nil {
		return err
	}
	end := time.Now().AddDate(0, 0, 21).Format(time.DateOnly)

	// The full push: Milestone → SprintIssue → SprintEnds → Events → Done.
	out, runErr := r.runPlanPushAgent(rel, system, clone, "01.1", end)
	c.expect(runErr == nil, "the push run failed: %v: %s", runErr, tail18(out))
	c.expect(strings.Contains(out, "final machine state: Done"), "the run must end in Done: %s", tail18(out))

	// A second machine run, and the one command after it, change nothing.
	out, runErr = r.runPlanPushAgent(rel, system, clone, "01.1", end)
	c.expect(runErr == nil && strings.Contains(out, "final machine state: Done"), "the second run must end in Done: %s", tail18(out))
	var pushed plan.PushResult
	if err := r.wordsJSON(&pushed, append([]string{"plan", "push", "--version", "01.1", "--end", end, "--group", r.group}, control...)...); err != nil {
		return err
	}
	c.expect(pushed.Milestone == "unchanged" && pushed.Sprint.Action == "unchanged", "the machine's push must satisfy the one command: %+v", pushed)
	for _, s := range pushed.SprintEnds {
		c.expect(s.Action == "unchanged", "sprint-end issue after the machine ran: %+v", s)
	}

	// The refusal: a road map without the release ends in Refused.
	out, runErr = r.runPlanPushAgent(rel, system, clone, "99.9", end)
	c.expect(runErr != nil, "a refused run must not report success: %s", tail18(out))
	c.expect(strings.Contains(out, "final machine state: Refused"), "the run must end in Refused: %s", tail18(out))
	c.expect(strings.Contains(out, "names no release"), "the Refused state must carry scrum-master's reason: %s", tail18(out))
	return c.err("planPushProfile")
}

// runPlanPushAgent runs the release's agent over the plan-push profile for
// one release, from the system repository's checkout, with the rig's
// tracker — the environment the profile's declarations name — and no scrum-master
// on PATH: the agent serves the words in process (GH-28).
func (r *rigEnv) runPlanPushAgent(rel runtime.Release, system, clone, version, end string) (string, error) {
	outDir, err := os.MkdirTemp(r.run.Dir, "planpush-agent-*")
	if err != nil {
		return "", err
	}
	cmd := exec.Command(rel.Agent,
		"--profile", filepath.Join(r.root, "plugin", "agents", "plan-push", "profile.yaml"),
		"--core-root", rel.CoreRoot(),
		"--directory", clone,
		"--output", outDir,
	)
	cmd.Dir = clone
	cmd.Env = append(os.Environ(),
		"PATH="+pathWithout("tpm", "scrum-master"),
		"SCRUM_MASTER_TRACKER="+filepath.Join(r.run.Dir, "tracker.git"),
		"SCRUM_MASTER_TRACKER_CLONE="+filepath.Join(r.run.Dir, "tracker-"+r.user),
		"SCRUM_MASTER_USER="+r.user,
		"SCRUM_MASTER_WEB_BASE="+r.webBase,
		"SCRUM_MASTER_SYSTEM="+system,
		"SCRUM_MASTER_VERSION="+version,
		"SCRUM_MASTER_END="+end,
		"SCRUM_MASTER_REF="+seedBranch,
	)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	return out.String(), err
}

// tail18 is the last part of a run's output, enough to carry its terminal
// state and reason into a finding.
func tail18(out string) string {
	const n = 800
	if len(out) <= n {
		return out
	}
	return "…" + out[len(out)-n:]
}
