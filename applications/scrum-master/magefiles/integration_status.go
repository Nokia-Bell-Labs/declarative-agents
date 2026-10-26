// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.StatusInPlace)
}

// StatusInPlace proves status reports in place (GH-2725, srd018 R12 to
// R17): the init profile lays down a system repository beside a team's,
// with a real tracker; with the roster in control/reports.md and the team's
// subsystem SRD, status-issues files the week's status issues where each
// person reports and the team and program rollup issues, and nothing on a
// second run; a report merged in the team's repository is collected, through
// the reports-collect profile, with the team repository's commit; the
// aggregate names the system's missing report with its open status issue;
// and previous reads the report back the next week (test: status reports
// in place; environment: local). It needs no rig: every repository is a
// bare one on local disk.
func (Integration) StatusInPlace() error {
	rel, err := repoRuntime()
	if err != nil {
		return err
	}
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "scrum-master-status-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if tmp, err = filepath.EvalSymlinks(tmp); err != nil {
		return err
	}
	words := filepath.Join(tmp, "scrum-master")
	if err := runStreaming(root, "go", "build", "-o", words, "./magefiles/words"); err != nil {
		return fmt.Errorf("build the words: %w", err)
	}
	const user, group, system, team = "operator", "prog", "prog/system", "prog/team-a"
	trackerRepo := filepath.Join(tmp, "tracker.git")
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "SCRUM_MASTER_") && !strings.HasPrefix(kv, "TPM_") {
			env = append(env, kv)
		}
	}
	env = append(env, "PATH="+pathWithout("tpm", "scrum-master"), install.SourceEnv+"="+rel.Src,
		"SCRUM_MASTER_USER="+user, "SCRUM_MASTER_TRACKER_CLONE="+filepath.Join(tmp, "tracker-"+user),
		"GIT_AUTHOR_NAME="+user, "GIT_AUTHOR_EMAIL="+user+"@example.invalid",
		"GIT_COMMITTER_NAME="+user, "GIT_COMMITTER_EMAIL="+user+"@example.invalid")
	sh := func(dir, name string, args ...string) (string, error) {
		cmd := exec.Command(name, args...)
		cmd.Dir, cmd.Env = dir, env
		out, err := cmd.CombinedOutput()
		if err != nil {
			return string(out), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out)), nil
	}
	sm := func(dir string, out any, args ...string) error {
		got, err := sh(dir, words, append(args, "--json")...)
		if err != nil {
			return err
		}
		return json.Unmarshal([]byte(got), out)
	}
	sysDir, teamDir := filepath.Join(tmp, "system"), filepath.Join(tmp, "team-a")
	for _, step := range [][]string{
		{tmp, "git", "init", "-q", "--bare", "-b", "main", trackerRepo},
		{tmp, "git", "init", "-q", "--bare", "-b", "main", filepath.Join(tmp, "system.git")},
		{tmp, "git", "init", "-q", "--bare", "-b", "main", filepath.Join(tmp, "team-a.git")},
		{tmp, "git", "clone", "-q", filepath.Join(tmp, "system.git"), sysDir},
		{tmp, "git", "clone", "-q", filepath.Join(tmp, "team-a.git"), teamDir},
		{teamDir, "git", "commit", "-q", "--allow-empty", "-m", "The team's first commit"},
		{teamDir, "git", "push", "-q", "-u", "origin", "main"},
	} {
		if _, err := sh(step[0], step[1], step[2:]...); err != nil {
			return err
		}
	}
	c := &check{}

	// The init profile lays the system down with the tracker and an empty
	// roster.
	req, _ := json.Marshal(map[string]any{"parameters": map[string]any{"agent": "claude", "tracker": trackerRepo, "group": group}})
	reqPath := filepath.Join(tmp, "init-request.json")
	if err := os.WriteFile(reqPath, req, 0o644); err != nil {
		return err
	}
	cmd := exec.Command(rel.Agent, "--profile", filepath.Join(root, "plugin", "agents", "init", "profile.yaml"),
		"--core-root", rel.CoreRoot(), "--directory", sysDir, "--output", filepath.Join(tmp, "init-run"), "--request", reqPath)
	cmd.Dir, cmd.Env = sysDir, env
	var b bytes.Buffer
	cmd.Stdout, cmd.Stderr = &b, &b
	runErr := cmd.Run()
	c.expect(runErr == nil && strings.Contains(b.String(), "final machine state: Done"), "the init run must end in Done: %v: %s", runErr, tail17(b.String()))
	_, err = os.Stat(filepath.Join(sysDir, reports.ControlFile))
	c.expect(err == nil, "init wrote no %s", reports.ControlFile)
	if len(c.failures) > 0 {
		return c.err("statusInPlace")
	}

	// The roster: alice reports in the team's repository, the program
	// manager in the system's; the team's subsystem SRD names its owner.
	roster, err := reports.RosterDocument(reports.Config{Team: []reports.Member{{Username: "alice", Project: team}, {Username: user, Project: system}}, ProgramLead: user})
	if err != nil {
		return err
	}
	writeFileTo(filepath.Join(sysDir, reports.ControlFile), string(roster))
	writeFileTo(filepath.Join(sysDir, "docs", "specs", "software-requirements", "srd010-team-a.md"), statusSubsystemSRD)
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "Lay down the program and its roster"}, {"push", "-q", "-u", "origin", "main"}} {
		if _, err := sh(sysDir, "git", args...); err != nil {
			return err
		}
	}

	// status-issues files where each person reports, then the rollups.
	var filed []reports.StatusResult
	if err := sm(sysDir, &filed, "reports", "status-issues", "--control", system, "--date", "2026-10-12"); err != nil {
		return err
	}
	var got []string
	for _, r := range filed {
		got = append(got, fmt.Sprintf("%s %s %s %s", r.Action, r.Kind, r.User, r.Project))
	}
	want := []string{"filed status alice " + team, "filed status " + user + " " + system, "filed team-rollup alice " + team, "filed program-rollup " + user + " " + system}
	c.expect(slices.Equal(got, want), "status-issues filed %v, want %v", got, want)
	var again []reports.StatusResult
	if err := sm(sysDir, &again, "reports", "status-issues", "--control", system, "--date", "2026-10-12"); err != nil {
		return err
	}
	c.expect(len(again) == len(filed) && !slices.ContainsFunc(again, func(r reports.StatusResult) bool { return r.Action != "exists" }),
		"a second status-issues run filed again: %+v", again)

	// alice's report merges in the team's repository.
	week, _ := reports.ParseWeek("2026-W42")
	path := reports.InPlaceReportPath(week, "alice")
	writeFileTo(filepath.Join(teamDir, path), reports.RenderWith("alice", "2026-W42", nil,
		[]reports.Entry{{"blocker": "no dev cluster", "component": "team-a", "since": "2026-10-12", "waiting-on": "infrastructure"}},
		nil, []string{"route the first model through the gateway"}))
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "Weekly status 2026-W42"}, {"push", "-q", "origin", "main"}} {
		if _, err := sh(teamDir, "git", args...); err != nil {
			return err
		}
	}
	commit, _ := sh(teamDir, "git", "rev-parse", "HEAD")

	// The collect profile reads it in place, with the team's commit.
	creq, _ := json.Marshal(map[string]any{"parameters": map[string]any{"control": system, "level": "program", "from": "2026-W42"}})
	creqPath := filepath.Join(tmp, "collect-request.json")
	if err := os.WriteFile(creqPath, creq, 0o644); err != nil {
		return err
	}
	cmd = exec.Command(rel.Agent, "--profile", filepath.Join(root, "plugin", "agents", "reports-collect", "profile.yaml"),
		"--core-root", rel.CoreRoot(), "--directory", sysDir, "--output", filepath.Join(tmp, "collect-run"), "--request", creqPath)
	cmd.Dir, cmd.Env = sysDir, env
	b.Reset()
	cmd.Stdout, cmd.Stderr = &b, &b
	runErr = cmd.Run()
	c.expect(runErr == nil && strings.Contains(b.String(), "final machine state: Done"), "the reports-collect run must end in Done: %v: %s", runErr, tail17(b.String()))
	var col reports.Collection
	if err := sm(sysDir, &col, "reports", "collect", "--control", system, "--level", "program", "--from", "2026-W42"); err != nil {
		return err
	}
	c.expect(len(col.Reports) == 1 && col.Reports[0].Repository == team && col.Reports[0].Path == path && col.Reports[0].Commit == commit,
		"collect read %+v, want alice's report in %s at %s", col.Reports, team, commit)
	c.expect(len(col.Unread) == 0, "collect could not read %+v", col.Unread)

	// The aggregate names the program manager's missing report and its
	// open status issue.
	var agg reports.Aggregation
	if err := sm(sysDir, &agg, "reports", "aggregate", "--control", system, "--level", "program", "--from", "2026-W42"); err != nil {
		return err
	}
	missing := slices.ContainsFunc(agg.Signals, func(s reports.Signal) bool {
		return s.Kind == reports.SignalMissingReport && s.Subject == system && strings.Contains(s.Detail, "its status issue "+system+"#")
	})
	c.expect(missing && agg.Reports == 1, "the aggregate read %d reports and signals %+v, want the system's missing report named with its issue", agg.Reports, agg.Signals)

	// previous reads it back the next week.
	var prev reports.Previous
	if err := sm(sysDir, &prev, "reports", "previous", "--project", team, "--user", "alice", "--date", "2026-10-19"); err != nil {
		return err
	}
	c.expect(prev.Found && prev.Week == "2026-W42" && prev.Commit == commit && slices.Equal(prev.Next, []string{"route the first model through the gateway"}),
		"previous = %+v", prev)
	return c.err("statusInPlace")
}

// statusSubsystemSRD is team-a's subsystem SRD: alice owns it, and its one
// component lives in prog/team-a.
const statusSubsystemSRD = `---
id: srd010-team-a
title: "Subsystem: team-a"
---

<!-- field: context | form: "tree" -->
## Context

- owner: alice

<!-- req: R1 -->
## R1 - Work

<!-- req: R1.1 -->
### R1.1

Work arrives.

<!-- field: implementation_targets -->
## Implementation targets

| name | repository | path |
| --- | --- | --- |
| team-a | https://gitlab.example.com/prog/team-a | / |
`
