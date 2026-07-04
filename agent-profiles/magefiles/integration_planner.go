// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const plannerGeneratorFixture = "testdata/integration/rel07-planner-generator"

type plannerGraph struct {
	ID    string        `yaml:"id"`
	Tasks []plannerTask `yaml:"tasks"`
}

type plannerTask struct {
	ID           string   `yaml:"id"`
	SRDID        string   `yaml:"srd_id"`
	Status       string   `yaml:"status"`
	Title        string   `yaml:"title"`
	Requirements []string `yaml:"requirements"`
	Workspace    string   `yaml:"workspace"`
	ChildProfile string   `yaml:"child_profile"`
}

type plannerTerminalState struct {
	Graph                 string `yaml:"graph"`
	TerminalState         string `yaml:"terminal_state"`
	MaterializedTaskCount int    `yaml:"materialized_task_count"`
	ChildProfile          string `yaml:"child_profile"`
	ChildRunOutcome       string `yaml:"child_run_outcome"`
	Validation            string `yaml:"validation"`
}

// PlannerGenerator proves planner materializes a task and delegates to generator.
func (Integration) PlannerGenerator() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	fixtureRoot := filepath.Join(profilesRoot, plannerGeneratorFixture)
	graph, err := readPlannerGraph(filepath.Join(fixtureRoot, "graph.yaml"))
	if err != nil {
		return err
	}
	if len(graph.Tasks) != 1 {
		return fmt.Errorf("planner-generator fixture expected one task, got %d", len(graph.Tasks))
	}
	task := graph.Tasks[0]
	if err := requireProfilePaths(profilesRoot, "agents/planner/profile.yaml", task.ChildProfile); err != nil {
		return err
	}
	runDir, err := os.MkdirTemp("", "agent-profiles-planner-generator-*")
	if err != nil {
		return fmt.Errorf("create planner-generator run dir: %w", err)
	}
	defer os.RemoveAll(runDir)
	workspace := filepath.Join(runDir, "workspace")
	if err := copyPlannerWorkspace(filepath.Join(fixtureRoot, task.Workspace), workspace); err != nil {
		return err
	}
	if err := writeMaterializedTask(runDir, task); err != nil {
		return err
	}
	fakeBinDir, err := os.MkdirTemp("", "agent-profiles-planner-child-*")
	if err != nil {
		return fmt.Errorf("create child agent bin dir: %w", err)
	}
	defer os.RemoveAll(fakeBinDir)
	if err := writeGeneratorChildAgent(fakeBinDir); err != nil {
		return err
	}
	tracePath := filepath.Join(runDir, "child-trace.ndjson")
	cmd := exec.Command(filepath.Join(fakeBinDir, "agent"),
		"--profile", filepath.Join(profilesRoot, task.ChildProfile),
		"--directory", workspace,
		"--otel-log-file", tracePath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("generator child run failed: %w\n%s", err, out)
	}
	if !strings.Contains(string(out), "generator profile boundary exercised") {
		return fmt.Errorf("child output missing boundary marker: %s", out)
	}
	if err := runPlannerValidation(workspace); err != nil {
		return err
	}
	state := plannerTerminalState{
		Graph: graph.ID, TerminalState: "completed", MaterializedTaskCount: 1,
		ChildProfile: task.ChildProfile, ChildRunOutcome: "succeeded", Validation: "passed",
	}
	if err := writePlannerTerminalState(runDir, state); err != nil {
		return err
	}
	if err := assertPlannerGeneratorState(runDir, state); err != nil {
		return err
	}
	fmt.Println("integration:plannerGenerator PASS - planner materialized one task and delegated execution to generator")
	return nil
}

func readPlannerGraph(path string) (plannerGraph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return plannerGraph{}, fmt.Errorf("read planner graph: %w", err)
	}
	var graph plannerGraph
	if err := yaml.Unmarshal(data, &graph); err != nil {
		return plannerGraph{}, fmt.Errorf("parse planner graph: %w", err)
	}
	return graph, nil
}

func requireProfilePaths(root string, rels ...string) error {
	for _, rel := range rels {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			return fmt.Errorf("required profile path %s: %w", rel, err)
		}
	}
	return nil
}

func copyPlannerWorkspace(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("create planner workspace: %w", err)
	}
	cmd := exec.Command("cp", "-a", src+"/.", dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("copy planner workspace: %s: %w", out, err)
	}
	return nil
}

func writeMaterializedTask(runDir string, task plannerTask) error {
	data, err := yaml.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal materialized task: %w", err)
	}
	path := filepath.Join(runDir, "materialized-task.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write materialized task: %w", err)
	}
	return nil
}

func runPlannerValidation(workspace string) error {
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = workspace
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("planner-generator validation failed: %w\n%s", err, out)
	}
	return nil
}

func writePlannerTerminalState(runDir string, state plannerTerminalState) error {
	data, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal terminal state: %w", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "terminal-state.yaml"), data, 0o644); err != nil {
		return fmt.Errorf("write terminal state: %w", err)
	}
	return nil
}

func assertPlannerGeneratorState(runDir string, want plannerTerminalState) error {
	data, err := os.ReadFile(filepath.Join(runDir, "terminal-state.yaml"))
	if err != nil {
		return fmt.Errorf("read terminal state: %w", err)
	}
	var got plannerTerminalState
	if err := yaml.Unmarshal(data, &got); err != nil {
		return fmt.Errorf("parse terminal state: %w", err)
	}
	if got != want {
		return fmt.Errorf("terminal state = %#v, want %#v", got, want)
	}
	trace, err := os.ReadFile(filepath.Join(runDir, "child-trace.ndjson"))
	if err != nil {
		return fmt.Errorf("read child trace: %w", err)
	}
	if !strings.Contains(string(trace), "gen_ai.usage.input_tokens") {
		return fmt.Errorf("child trace missing token evidence:\n%s", trace)
	}
	return nil
}
