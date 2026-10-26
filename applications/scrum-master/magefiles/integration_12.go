// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/scaffold"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.Backlog)
}

// Backlog builds scrum-master, inits a system repository, lays a theme, initiatives, an epic, a feature, stories, and an engineering document into it, and proves scrum-master backlog list names them all with their kinds and a story's points and rank, scrum-master backlog check passes them and fails once a story loses its points, neither writes to the repository, and AGENTS.md lists the draft skill (test-rel04.2-draft). It needs no rig and no network.
func (Integration) Backlog() error {
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	c := &check{}
	tmp, err := os.MkdirTemp("", "scrum-master-backlog-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if tmp, err = filepath.EvalSymlinks(tmp); err != nil {
		return err
	}
	bin, repo := filepath.Join(tmp, "scrum-master"), filepath.Join(tmp, "system")
	if err := runStreaming(root, "go", "build", "-o", bin, "./magefiles/words"); err != nil {
		return fmt.Errorf("build the words: %w", err)
	}
	run := func(args ...string) (string, error) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		return err
	}
	if out, err := exec.Command("git", "-C", repo, "init", "-q", "-b", "main").CombinedOutput(); err != nil {
		return fmt.Errorf("git init: %w: %s", err, out)
	}
	if out, err := run("init", "--dir", repo); err != nil {
		return fmt.Errorf("scrum-master init: %w: %s", err, out)
	}
	if err := copyTree(filepath.Join(root, "testdata", "hierarchy", "valid"), repo); err != nil {
		return err
	}
	// An engineering document beside the hierarchy: the gate checks its
	// format, and the folder README init wrote is not read as one (#181).
	writeFileTo(filepath.Join(repo, "docs", "engineering", "eng01-scoring-runtime.md"), `---
id: eng01-scoring-runtime
title: How the scoring subsystem runs
---

<!-- field: introduction -->
## Introduction

How the scoring subsystem is structured, run, and deployed.

<!-- section: processes -->
## Processes

Ingest reads trajectories from the queue, and grading scores each one.

<!-- field: references -->
## References

- docs/ARCHITECTURE.md
`)
	// The gate reads only what git tracks and names what it does not.
	if out, err := exec.Command("git", "-C", repo, "add", "-A").CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %w: %s", err, out)
	}

	out, err := run("backlog", "list", "--json")
	var entries []struct {
		ID, Kind, Points, Rank string
	}
	c.expect(err == nil && json.Unmarshal([]byte(out), &entries) == nil, "backlog list: %v: %s", err, out)
	kinds := map[string]int{}
	for _, e := range entries {
		kinds[e.Kind]++
		if e.Kind == "story" {
			c.expect(e.Points != "" && e.Rank != "", "story %s listed without points or rank", e.ID)
		}
	}
	c.expect(kinds["theme"] == 1 && kinds["initiative"] == 2 && kinds["epic"] == 1 && kinds["feature"] == 1 && kinds["story"] == 4, "backlog list kinds %v", kinds)

	before, err := tree(repo)
	if err != nil {
		return err
	}
	out, err = run("backlog", "check")
	// Nine hierarchy documents, the repository's SRD (#149), and VISION and
	// ARCHITECTURE, which init left as placeholders the gate names (GH-22).
	c.expect(err == nil && strings.HasPrefix(out, "hierarchy gate: pass, 12 documents") && strings.Contains(out, "unfilled: docs/ARCHITECTURE.md"),
		"check on a valid hierarchy: %v: %s", err, out)
	after, err := tree(repo)
	if err != nil {
		return err
	}
	c.expect(before == after, "list or check wrote to the repository")

	story := filepath.Join(repo, "docs", "specs", "stories", "st002-store-scores.md")
	b, err := os.ReadFile(story)
	if err != nil {
		return err
	}
	if err := os.WriteFile(story, []byte(strings.Replace(string(b), "- points: 3\n", "", 1)), 0o644); err != nil {
		return err
	}
	out, err = run("backlog", "check")
	c.expect(err != nil && strings.Contains(out, "st002-store-scores.md: points:"), "check on a story without points: %v: %s", err, out)

	agents, err := os.ReadFile(filepath.Join(repo, scaffold.AgentsFile))
	c.expect(err == nil && strings.Contains(string(agents), "\n- draft: "), "AGENTS.md does not list the draft skill")
	return c.err("backlog")
}

// tree is every file under dir with its contents, as one string, so two
// calls can be compared.
func tree(dir string) (string, error) {
	var b strings.Builder
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.Contains(p, string(filepath.Separator)+".git"+string(filepath.Separator)) {
			return err
		}
		data, err := os.ReadFile(p)
		fmt.Fprintf(&b, "%s\n%s\n", p, data)
		return err
	})
	return b.String(), err
}
