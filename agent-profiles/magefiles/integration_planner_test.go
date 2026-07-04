// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReadPlannerGraphLoadsSingleTask(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graph.yaml")
	writeFile(t, path, `id: rel07-planner-generator
tasks:
  - id: task-greet
    srd_id: srd001-greet
    status: ready
    title: Implement greeting
    requirements: [R1]
    workspace: workspace
    child_profile: agents/generator/profile.yaml
`)

	graph, err := readPlannerGraph(path)
	if err != nil {
		t.Fatalf("readPlannerGraph: %v", err)
	}
	if graph.ID != "rel07-planner-generator" || len(graph.Tasks) != 1 {
		t.Fatalf("graph = %#v", graph)
	}
	if graph.Tasks[0].ChildProfile != "agents/generator/profile.yaml" {
		t.Fatalf("child profile = %q", graph.Tasks[0].ChildProfile)
	}
}

func TestWriteMaterializedTaskRecordsPlannerBoundary(t *testing.T) {
	runDir := t.TempDir()
	task := plannerTask{
		ID: "task-greet", SRDID: "srd001-greet", Status: "ready",
		Title: "Implement greeting", Requirements: []string{"R1"},
		Workspace: "workspace", ChildProfile: "agents/generator/profile.yaml",
	}

	if err := writeMaterializedTask(runDir, task); err != nil {
		t.Fatalf("writeMaterializedTask: %v", err)
	}
	data := readTestFile(t, filepath.Join(runDir, "materialized-task.yaml"))
	for _, want := range []string{"task-greet", "srd001-greet", "agents/generator/profile.yaml"} {
		if !strings.Contains(data, want) {
			t.Fatalf("materialized task missing %q:\n%s", want, data)
		}
	}
}

func TestAssertPlannerGeneratorStateRequiresTraceAndTerminalState(t *testing.T) {
	runDir := t.TempDir()
	state := plannerTerminalState{
		Graph: "rel07-planner-generator", TerminalState: "completed",
		MaterializedTaskCount: 1, ChildProfile: "agents/generator/profile.yaml",
		ChildRunOutcome: "succeeded", Validation: "passed",
	}
	if err := writePlannerTerminalState(runDir, state); err != nil {
		t.Fatalf("writePlannerTerminalState: %v", err)
	}
	writeFile(t, filepath.Join(runDir, "child-trace.ndjson"), `{"Name":"child generator shim","Attributes":[{"Key":"gen_ai.usage.input_tokens","Value":{"Type":"INT64","Value":1}}]}`+"\n")

	if err := assertPlannerGeneratorState(runDir, state); err != nil {
		t.Fatalf("assertPlannerGeneratorState: %v", err)
	}
}

func TestAssertPlannerGeneratorStateRejectsMismatch(t *testing.T) {
	runDir := t.TempDir()
	got := plannerTerminalState{
		Graph: "rel07-planner-generator", TerminalState: "stalled",
		MaterializedTaskCount: 1, ChildProfile: "agents/generator/profile.yaml",
		ChildRunOutcome: "failed", Validation: "failed",
	}
	want := plannerTerminalState{
		Graph: "rel07-planner-generator", TerminalState: "completed",
		MaterializedTaskCount: 1, ChildProfile: "agents/generator/profile.yaml",
		ChildRunOutcome: "succeeded", Validation: "passed",
	}
	if err := writePlannerTerminalState(runDir, got); err != nil {
		t.Fatalf("writePlannerTerminalState: %v", err)
	}
	writeFile(t, filepath.Join(runDir, "child-trace.ndjson"), `{"Name":"child generator shim","Attributes":[{"Key":"gen_ai.usage.input_tokens","Value":{"Type":"INT64","Value":1}}]}`+"\n")

	err := assertPlannerGeneratorState(runDir, want)
	if err == nil {
		t.Fatal("expected terminal state mismatch")
	}
	if !strings.Contains(err.Error(), "terminal state") {
		t.Fatalf("error = %q, want terminal state mismatch", err)
	}
}

func TestRequireProfilePathsRejectsMissingProfile(t *testing.T) {
	err := requireProfilePaths(t.TempDir(), "agents/planner/profile.yaml")
	if err == nil {
		t.Fatal("expected missing profile error")
	}
	if !strings.Contains(err.Error(), "required profile path") {
		t.Fatalf("unexpected error: %v", err)
	}
}
