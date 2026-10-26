// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/spectool"
)

// TestSpecToolConvertsAndFormats builds the tool against the sibling
// agent-core and runs it on a copy of the corpus, converting an objective
// document as an SRD (GH-190). It builds the tool, so it runs only when
// SCRUM_MASTER_SPEC_TOOL names anything.
func TestSpecToolConvertsAndFormats(t *testing.T) {
	if os.Getenv("SCRUM_MASTER_SPEC_TOOL") == "" {
		t.Skip("set SCRUM_MASTER_SPEC_TOOL to build and run the tool")
	}
	rel, err := repoRuntime()
	if err != nil {
		t.Fatal(err)
	}
	tool, err := spectool.Ensure(rel.Dir, rel.CoreRoot(), runStreaming)
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	run := func(name string, args ...string) {
		t.Helper()
		if err := runStreaming(work, name, args...); err != nil {
			t.Fatalf("%s %v: %v", name, args, err)
		}
	}
	run("cp", "-R", mustAbs(t, filepath.Join("..", "docs")), ".")
	run("mkdir", "-p", filepath.Join("docs", "specs", "objectives"))
	writeFile(t, filepath.Join(work, "docs", "specs", "objectives", "obj001-reliable-agents.yaml"), objectiveYAML)
	run("git", "init", "-q")
	run("git", "add", ".")
	run(tool, "convert", "all")
	if _, err := os.Stat(filepath.Join(work, "docs", "specs", "objectives", "obj001-reliable-agents.md")); err != nil {
		t.Fatal("convert left no objective document in Markdown")
	}
	run("git", "add", "-A")
	run(tool, "fmt", "all")
	run(tool, "check", "all")
}

// objectiveYAML is an objective document in YAML with its data in the
// declared fields, which the release converts to an SRD.
const objectiveYAML = `id: obj001-reliable-agents
title: Reliable agents
status: agreed
context:
  owner: technical program manager
  horizon: 2026-W43..2027-W03
problem: Nobody can show that an agent works before it is deployed.
goals:
  - "G1: Every deployed agent passed an evaluation."
requirements:
  R1:
    title: Evaluation of one agent
    applicability:
      due: "2026-11-06"
    items:
      - R1.1: A grader scores one agent's trajectories.
non_goals:
  - Choosing which agents to deploy.
acceptance_criteria:
  - id: AC1
    criterion: One agent is scored.
    traces: [R1.1]
    tests: [grader_scores_agent]
    applicability: dev
`

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
