// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
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
	Name       string         `yaml:"name"`
	Type       string         `yaml:"type"`
	Binary     string         `yaml:"binary"`
	Init       string         `yaml:"init"`
	Config     map[string]any `yaml:"config"`
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

	var declarations plannerDeclarations
	readPlannerYAML(t, "builtin.yaml", &declarations)
	var report plannerDeclaration
	for _, declaration := range declarations.Tools {
		if declaration.Name == "report_parse_error" {
			report = declaration
			break
		}
	}
	if report.Init != "report_parse_error" {
		t.Fatalf("report_parse_error init = %q, want report_parse_error", report.Init)
	}
	if got := report.Config["response_contract"]; got != "implementation_plan_yaml" {
		t.Fatalf("report_parse_error response_contract = %#v, want implementation_plan_yaml", got)
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

func TestPlannerVariantsComposeProfileOwnedPrompts(t *testing.T) {
	for _, file := range []string{"machine.yaml", "machine-plan-only.yaml"} {
		t.Run(file, func(t *testing.T) {
			var machine plannerMachine
			readPlannerYAML(t, file, &machine)
			requirePlannerTransition(t, machine, "Extracting", "TaskExtracted", "MarkingPlanning", "mark_nodes_planning", "")
			requirePlannerTransition(t, machine, "MarkingPlanning", "NodesMarkedPlanning", "InitializingFailureContext", "reset_failure_context", "failure_context")
			requirePlannerTransition(t, machine, "Resetting", "ToolDone", "ProjectingPromptContext", "project_planner_context", "planner_context")
			requirePlannerTransition(t, machine, "ProjectingPromptContext", "PlannerContextProjected", "PromptAssembly", "compose_planner_prompt", "")
			requirePlannerTransition(t, machine, "PromptAssembly", "PromptReady", "PlanInvoking", "invoke_llm", "")
		})
	}

	var full plannerMachine
	readPlannerYAML(t, "machine.yaml", &full)
	requirePlannerTransition(t, full, "Testing", "ToolFailed", "CapturingFailure", "capture_planner_failure", "failure_context")
	requirePlannerTransition(t, full, "CapturingFailure", "FailureCaptured", "IncrementingRetry", "increment_retry", "retry_count")
	requirePlannerTransition(t, full, "CheckingRetryLimit", "RetryAvailable", "Resetting", "reset_history", "")

	var declarations plannerDeclarations
	readPlannerYAML(t, "builtin.yaml", &declarations)
	var compose plannerDeclaration
	for _, declaration := range declarations.Tools {
		if declaration.Name == "compose_planner_prompt" {
			compose = declaration
			break
		}
	}
	if compose.Init != "compose" {
		t.Fatalf("compose_planner_prompt init = %q, want compose", compose.Init)
	}
	template, _ := compose.Config["template"].(string)
	if !strings.Contains(template, "## Retry Context") || !strings.Contains(template, "## Output Format") {
		t.Fatalf("profile prompt template omits retry or output sections: %q", template)
	}
	inputs, _ := compose.Config["inputs"].(map[string]any)
	if inputs["retry_context"] != "$from(failure_context).output" {
		t.Fatalf("retry context selector = %#v", inputs["retry_context"])
	}
}

func TestPlannerPromptLayersRequestCanonicalPlanDocument(t *testing.T) {
	var builtin plannerDeclarations
	readPlannerYAML(t, "builtin.yaml", &builtin)
	var llm plannerDeclarations
	readPlannerYAML(t, filepath.Join("llm", "default.yaml"), &llm)

	prompts := make(map[string]string)
	for _, declaration := range builtin.Tools {
		if declaration.Name == "compose_planner_prompt" {
			prompts["composed user prompt"], _ = declaration.Config["template"].(string)
		}
	}
	for _, declaration := range llm.Tools {
		if declaration.Name == "invoke_llm" {
			prompts["system prompt"], _ = declaration.Config["system_prompt"].(string)
			prompts["tool prompt"], _ = declaration.Config["tool_prompt"].(string)
		}
	}

	if len(prompts) != 3 {
		t.Fatalf("planner prompt layers = %d, want composed, system, and tool prompts", len(prompts))
	}
	for name, prompt := range prompts {
		t.Run(name, func(t *testing.T) {
			normalized := strings.Join(strings.Fields(prompt), " ")
			for _, required := range []string{
				"exactly one top-level YAML mapping",
				"exactly these six keys: title, summary, files, requirements, design_decisions, and acceptance_criteria",
				"root sequence/list",
				"multiple plans",
				"wrapper/envelope",
			} {
				if !strings.Contains(normalized, required) {
					t.Errorf("%s omits %q:\n%s", name, required, prompt)
				}
			}
			for _, incompatible := range []string{"- steps:", "- rationale:"} {
				if strings.Contains(prompt, incompatible) {
					t.Errorf("%s retains incompatible schema field %q:\n%s", name, incompatible, prompt)
				}
			}
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
