// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

type plannerMachine struct {
	States      []plannerState      `yaml:"states"`
	Signals     []plannerSignal     `yaml:"signals"`
	Terminals   []string            `yaml:"terminal_states"`
	Transitions []plannerTransition `yaml:"transitions"`
}

type plannerState struct {
	Name string `yaml:"name"`
}

type plannerSignal struct {
	Name string `yaml:"name"`
}

type plannerTransition struct {
	State  string `yaml:"state"`
	Signal string `yaml:"signal"`
	Next   string `yaml:"next"`
	Action string `yaml:"action"`
	Label  string `yaml:"label"`
}

type plannerSelection struct {
	Tools []string `yaml:"tools"`
}

type plannerDeclarations struct {
	Tools []plannerDeclaration `yaml:"tools"`
}

type plannerDeclaration struct {
	Name       string `yaml:"name"`
	Type       string `yaml:"type"`
	Binary     string `yaml:"binary"`
	Parameters struct {
		Properties map[string]plannerParameter `yaml:"properties"`
	} `yaml:"parameters"`
}

type plannerParameter struct {
	Flag   string `yaml:"flag"`
	Source string `yaml:"source"`
}

func TestPlannerSelectsDeclarativeTrackerSentence(t *testing.T) {
	var selection plannerSelection
	readPlannerYAML(t, "tools.yaml", &selection)
	for _, word := range []string{"format_issue", "write", "create_tracker_issue", "record_tracker_issue"} {
		if !containsPlannerWord(selection.Tools, word) {
			t.Errorf("planner selection is missing %q", word)
		}
	}
	if containsPlannerWord(selection.Tools, "create_issue") {
		t.Error("planner selection still contains legacy create_issue")
	}
}

func TestPlannerVariantsRouteParseRetriesExplicitly(t *testing.T) {
	var selection plannerSelection
	readPlannerYAML(t, "tools.yaml", &selection)
	if !containsPlannerWord(selection.Tools, "report_parse_error") {
		t.Fatal(`planner selection is missing "report_parse_error"`)
	}

	for _, file := range []string{"machine.yaml", "machine-plan-only.yaml"} {
		t.Run(file, func(t *testing.T) {
			var machine plannerMachine
			readPlannerYAML(t, file, &machine)
			requirePlannerTransition(t, machine, "PlanParsing", "ParseFailed", "ReportingParseError", "report_parse_error", "")
			requirePlannerTransition(t, machine, "ReportingParseError", "ToolDone", "PlanInvoking", "invoke_llm", "")
			requirePlannerTransition(t, machine, "ReportingParseError", "BudgetExhausted", "Failed", "", "")
		})
	}
}

func TestPlannerVariantsClassifyEmptyExtractionThroughRemainingWork(t *testing.T) {
	for _, test := range []struct {
		file  string
		state string
	}{
		{file: "machine.yaml", state: "Extracting"},
		{file: "machine-plan-only.yaml", state: "Extracting"},
		{file: "machine-passthrough.yaml", state: "SelectingReady"},
	} {
		t.Run(test.file, func(t *testing.T) {
			var machine plannerMachine
			readPlannerYAML(t, test.file, &machine)
			requirePlannerTransition(t, machine, test.state, "NoTask", "QueryingRemainingWork", "remaining_work", "")
			requirePlannerTransition(t, machine, "QueryingRemainingWork", "AllDone", "Completed", "", "")
			requirePlannerTransition(t, machine, "QueryingRemainingWork", "Blocked", "Stalled", "", "")
			for _, transition := range machine.Transitions {
				if transition.State == test.state && (transition.Signal == "AllDone" || transition.Signal == "Blocked") {
					t.Errorf("extraction still classifies graph state directly: %+v", transition)
				}
			}
		})
	}
}

func TestPlannerPassThroughSplitsAggregateSelectionPlanAndGraphMutation(t *testing.T) {
	var machine plannerMachine
	readPlannerYAML(t, "machine-passthrough.yaml", &machine)
	requirePlannerTransition(t, machine, "Loading", "GraphLoaded", "SelectingReady", "select_all_ready", "")
	requirePlannerTransition(t, machine, "SelectingReady", "ReadySelected", "SeedingPassThroughPlan", "seed_passthrough_plan", "")
	requirePlannerTransition(t, machine, "SeedingPassThroughPlan", "PassThroughPlanSeeded", "MarkingPlanning", "mark_nodes_planning", "")
	requirePlannerTransition(t, machine, "MarkingPlanning", "NodesMarkedPlanning", "InitializingRetry", "reset_retry_count", "retry_count")
	for _, transition := range machine.Transitions {
		if transition.Action == "extract_all" {
			t.Errorf("pass-through machine still selects compound extract_all: %+v", transition)
		}
	}
}

func TestPlannerVariantsDoNotDeclareUnreachableBatchPause(t *testing.T) {
	for _, file := range []string{"machine.yaml", "machine-plan-only.yaml"} {
		t.Run(file, func(t *testing.T) {
			var machine plannerMachine
			readPlannerYAML(t, file, &machine)
			for _, state := range machine.States {
				if state.Name == "Paused" {
					t.Error("planner still declares unreachable Paused state")
				}
			}
			for _, signal := range machine.Signals {
				if signal.Name == "BatchLimitReached" {
					t.Error("planner still declares unreachable BatchLimitReached signal")
				}
			}
			for _, terminal := range machine.Terminals {
				if terminal == "Paused" {
					t.Error("planner still declares unreachable Paused terminal")
				}
			}
		})
	}
}

func TestPlannerVariantsSequenceTrackerSentence(t *testing.T) {
	for _, test := range []struct {
		file      string
		afterNext string
	}{
		{file: "machine.yaml", afterNext: "MarkingExecuting"},
		{file: "machine-plan-only.yaml", afterNext: "Extracting"},
	} {
		t.Run(test.file, func(t *testing.T) {
			var machine plannerMachine
			readPlannerYAML(t, test.file, &machine)
			requirePlannerTransition(t, machine, "PlanParsing", "PlanReady", "IssueFormatting", "format_issue", "issue_input")
			requirePlannerTransition(t, machine, "IssueFormatting", "IssueFormatted", "IssueBodyWriting", "write", "")
			requirePlannerTransition(t, machine, "IssueBodyWriting", "ToolDone", "IssueCreating", "create_tracker_issue", "")
			requirePlannerTransition(t, machine, "IssueCreating", "ToolDone", "IssueRecording", "record_tracker_issue", "")
			requirePlannerTransition(t, machine, "IssueRecording", "Materialized", test.afterNext, "", "")
			requirePlannerTransition(t, machine, "IssueBodyWriting", "ToolFailed", "Failed", "", "")
			requirePlannerTransition(t, machine, "IssueCreating", "ToolFailed", "Failed", "", "")
		})
	}
}

func TestPlannerExecutionSeparatesGraphWriteAndChildBoundary(t *testing.T) {
	for _, file := range []string{"machine.yaml", "machine-passthrough.yaml"} {
		t.Run(file, func(t *testing.T) {
			var machine plannerMachine
			readPlannerYAML(t, file, &machine)
			requirePlannerTransition(t, machine, "MarkingExecuting", "NodesMarkedExecuting", "FormattingTask", "format_task_file", "task_file")
			requirePlannerTransition(t, machine, "FormattingTask", "TaskFileFormatted", "MaterializingTask", "write", "")
			requirePlannerTransition(t, machine, "MaterializingTask", "ToolDone", "InvokingExecutor", "invoke_executor", "")
			requirePlannerTransition(t, machine, "InvokingExecutor", "ToolDone", "Vetting", "vet", "")
			for _, transition := range machine.Transitions {
				if transition.Action == "execute_task" {
					t.Errorf("planner still selects compound execute_task: %+v", transition)
				}
			}
		})
	}
}

func TestPlannerTrackerCommandIsProfileConfiguredExec(t *testing.T) {
	var declarations plannerDeclarations
	readPlannerYAML(t, "tracker-exec.yaml", &declarations)
	if len(declarations.Tools) != 1 {
		t.Fatalf("tracker declarations = %d, want 1", len(declarations.Tools))
	}
	tool := declarations.Tools[0]
	if tool.Name != "create_tracker_issue" || tool.Type != "exec" || tool.Binary != "bd" {
		t.Fatalf("tracker declaration = %+v", tool)
	}
	for name, want := range map[string]plannerParameter{
		"title":     {Flag: "--title", Source: "$from(issue_input).parameters.title"},
		"body_file": {Flag: "--body-file", Source: "$from(issue_input).parameters.body_file"},
		"directory": {Flag: "-C", Source: "$from(issue_input).parameters.directory"},
		"deps":      {Flag: "--deps", Source: "$from(issue_input).parameters.deps"},
	} {
		if got := tool.Parameters.Properties[name]; got != want {
			t.Errorf("parameter %q = %+v, want %+v", name, got, want)
		}
	}
}

func readPlannerYAML(t *testing.T, name string, target any) {
	t.Helper()
	path := filepath.Join("..", "agents", "planner", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func containsPlannerWord(words []string, want string) bool {
	for _, word := range words {
		if word == want {
			return true
		}
	}
	return false
}

func requirePlannerTransition(t *testing.T, machine plannerMachine, state, signal, next, action, label string) {
	t.Helper()
	for _, transition := range machine.Transitions {
		if transition.State == state && transition.Signal == signal {
			if transition.Next != next || (action != "" && transition.Action != action) || transition.Label != label {
				t.Fatalf("%s/%s = %+v, want next=%s action=%s label=%s", state, signal, transition, next, action, label)
			}
			return
		}
	}
	t.Fatalf("missing transition %s/%s", state, signal)
}
