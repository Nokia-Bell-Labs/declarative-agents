// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"path/filepath"
	"testing"
)

// Shipped-critic conformance for the sentence-tool migration. catalog
// owns the critic profile, so the assertion that its machines actually sequence
// the replacement words belongs here rather than in an agent-core package test
// reaching across the module boundary (srd034 R2.1, R2.2; GH-512).
//
// These read only catalog assets. The generic checks -- every selected
// word is declared, and every emit has a transition -- run over all profiles in
// `mage validate`; what is proven here is the critic-specific sequence.

// criticSessionWords are the words that replaced the single load_suite word.
var criticSessionWords = []string{
	"parse_suite_config",
	"discover_suite_samples",
	"expand_eval_grid",
	"init_eval_session",
	"report_suite_summary",
	"materialize_eval_points",
}

type criticMachineConfig struct {
	Transitions []criticTransition `yaml:"transitions"`
}

type criticTransition struct {
	State   string `yaml:"state"`
	Signal  string `yaml:"signal"`
	Next    string `yaml:"next"`
	Action  string `yaml:"action"`
	ForEach *struct {
		Items      string   `yaml:"items"`
		As         string   `yaml:"as"`
		Mode       string   `yaml:"mode"`
		Failure    string   `yaml:"failure"`
		ContinueOn []string `yaml:"continue_on"`
		Join       struct {
			Next string `yaml:"next"`
		} `yaml:"join"`
	} `yaml:"for_each"`
}

type criticToolSelectionFile struct {
	Tools []string `yaml:"tools"`
}

type criticToolDeclarations struct {
	Tools []criticToolDeclaration `yaml:"tools"`
}

type criticToolDeclaration struct {
	Name         string   `yaml:"name"`
	Type         string   `yaml:"type"`
	Binary       string   `yaml:"binary"`
	Args         []string `yaml:"args"`
	Requirements struct {
		Input []string `yaml:"input"`
	} `yaml:"requirements"`
	Config struct {
		PointToolDeclarations []string `yaml:"point_tool_declarations"`
	} `yaml:"config"`
}

// TestCriticSelectsSentenceWords proves the shipped session selection asks for
// the replacement words and no longer asks for the retired load_suite.
func TestCriticSelectsSentenceWords(t *testing.T) {
	selection := criticToolSelection(t, "tools.yaml")
	requireNotSelected(t, selection, "load_suite")
	requireNotSelected(t, selection, "next_point")
	for _, word := range criticSessionWords {
		requireSelected(t, selection, word)
	}
}

// TestCriticSessionSequence pins the session pipeline: each stage is reached by
// the previous stage's word, ending in the nested point advance. A reordering
// that still passes generic emits validation would change what the critic does.
func TestCriticSessionSequence(t *testing.T) {
	machine := criticMachine(t, "machine.yaml")
	assertTransition(t, machine, "Idle", "Seed", "ParsingSuiteConfig", "parse_suite_config")
	assertTransition(t, machine, "ParsingSuiteConfig", "SuiteConfigParsed", "DiscoveringSuiteSamples", "discover_suite_samples")
	assertTransition(t, machine, "DiscoveringSuiteSamples", "SuiteSamplesDiscovered", "ExpandingEvalGrid", "expand_eval_grid")
	assertTransition(t, machine, "ExpandingEvalGrid", "EvalGridExpanded", "InitializingEvalSession", "init_eval_session")
	assertTransition(t, machine, "InitializingEvalSession", "EvalSessionInitialized", "ReportingSuiteSummary", "report_suite_summary")
	assertTransition(t, machine, "ReportingSuiteSummary", "SuiteLoaded", "MaterializingPoints", "materialize_eval_points")
	assertTransition(t, machine, "RunningPoints", "PointsCompleted", "Reporting", "report_session")
	iteration := findTransition(machine, "MaterializingPoints", "EvalPointsMaterialized")
	if iteration == nil || iteration.ForEach == nil {
		t.Fatal("critic point dispatch does not declare for_each")
	}
	if iteration.Action != "run_point" || iteration.ForEach.Items != "$from(eval_points).points" ||
		iteration.ForEach.As != "point" || iteration.ForEach.Mode != "sequential" ||
		iteration.ForEach.Failure != "fail_fast" || iteration.ForEach.Join.Next != "RunningPoints" {
		t.Fatalf("unexpected critic point iterator: %#v", iteration)
	}
}

// TestCriticPointFailureSignals pins the point machine's failure routing. A
// harness failure, an oracle failure, and a version mismatch each have to keep
// the point moving toward a summary rather than stranding the session, so these
// transitions are what make a failed point still produce a result.
func TestCriticPointFailureSignals(t *testing.T) {
	machine := criticMachine(t, "point.yaml")
	// Every harness outcome first persists result.json through the shared write
	// word (GH-1378), then proceeds to the oracle check.
	assertTransition(t, machine, "RunningAgent", "HarnessFailed", "WritingResult", "write")
	assertTransition(t, machine, "RunningAgent", "HarnessTimedOut", "WritingResult", "write")
	assertTransition(t, machine, "WritingResult", "ToolDone", "RunningOracleCheck", "run_oracle_check")
	assertTransition(t, machine, "WritingResult", "ToolFailed", "RecordingFailure", "record_point_failure")
	assertTransition(t, machine, "WritingResult", "CommandError", "RecordingFailure", "record_point_failure")
	assertTransition(t, machine, "RunningOracleCheck", "ToolFailed", "RecordingOracleResult", "record_oracle_result")
	assertTransition(t, machine, "RecordingOracleResult", "OracleCheckFailed", "CollectingTraceTokens", "collect_trace_tokens")
	assertTransition(t, machine, "CollectingTraceTokens", "TraceTokensCollected", "CheckingAgentVersion", "check_agent_version")
	assertTransition(t, machine, "CheckingAgentVersion", "AgentVersionMismatch", "SummarizingPointResults", "summarize_point_results")
	assertTransition(t, machine, "SummarizingPointResults", "ResultsCollected", "CollectingMetrics", "collect_metrics")
	assertTransition(t, machine, "RunningAgent", "CommandError", "RecordingFailure", "record_point_failure")
	assertTransition(t, machine, "RecordingFailure", "PointFailureRecorded", "CollectingFailureMetrics", "collect_metrics")
	assertTransition(t, machine, "CollectingFailureMetrics", "MetricsCollected", "Failed", "")
}

func TestCriticPointSelectsSharedWorkspaceExecWords(t *testing.T) {
	selection := criticToolSelection(t, "tools-point.yaml")
	for _, word := range []string{"copy_dir", "git_init", "stage_all", "commit_workspace_baseline", "write"} {
		requireSelected(t, selection, word)
	}
	for _, retired := range []string{"copy_sample_workspace", "init_workspace_repo", "stage_workspace_baseline"} {
		requireNotSelected(t, selection, retired)
	}
}

func TestCriticPointWorkspaceSequenceUsesSharedExecSignals(t *testing.T) {
	machine := criticMachine(t, "point.yaml")
	assertTransition(t, machine, "CreatingPointDir", "PointDirCreated", "CopyingSampleWorkspace", "copy_dir")
	assertTransition(t, machine, "CopyingSampleWorkspace", "ToolDone", "CheckingSampleDocs", "sample_docs")
	assertTransition(t, machine, "CheckingSampleDocs", "SampleDocsPresent", "CopyingSampleDocs", "copy_dir")
	assertTransition(t, machine, "CheckingSampleDocs", "SampleDocsAbsent", "InitializingWorkspaceRepo", "git_init")
	assertTransition(t, machine, "CopyingSampleDocs", "ToolDone", "InitializingWorkspaceRepo", "git_init")
	assertTransition(t, machine, "InitializingWorkspaceRepo", "ToolDone", "StagingWorkspaceBaseline", "stage_all")
	assertTransition(t, machine, "StagingWorkspaceBaseline", "ToolDone", "CommittingWorkspaceBaseline", "commit_workspace_baseline")
	assertTransition(t, machine, "CommittingWorkspaceBaseline", "ToolDone", "ResolvingAgentCommit", "rev_parse")
	assertTransition(t, machine, "ResolvingAgentCommit", "ToolDone", "RecordingAgentCommit", "record_agent_commit")
	assertTransition(t, machine, "RecordingAgentCommit", "AgentCommitRecorded", "SnapshotConfig", "dump_config")
	assertTransition(t, machine, "SnapshotConfig", "ConfigDumped", "RunningAgent", "run_agent")
	assertTransition(t, machine, "RunningAgent", "HarnessFinished", "WritingResult", "write")
}

func TestCriticPointOracleCommandIsProfileConfiguredExec(t *testing.T) {
	var declarations criticToolDeclarations
	if err := readYAML(criticProfilePath(t, "point-exec.yaml"), &declarations); err != nil {
		t.Fatalf("load critic point exec declarations: %v", err)
	}
	for _, declaration := range declarations.Tools {
		if declaration.Name == "run_oracle_check" {
			if declaration.Type != "exec" || declaration.Binary != "go" {
				t.Fatalf("run_oracle_check declaration = %#v, want configured go exec", declaration)
			}
			if len(declaration.Args) != 2 || declaration.Args[0] != "test" || declaration.Args[1] != "./..." {
				t.Fatalf("run_oracle_check args = %v, want [test ./...]", declaration.Args)
			}
			return
		}
	}
	t.Fatal("run_oracle_check exec declaration not found")
}

func TestCriticRunPointContractNamesToolDeclarations(t *testing.T) {
	var declarations criticToolDeclarations
	if err := readYAML(criticProfilePath(t, "builtin.yaml"), &declarations); err != nil {
		t.Fatalf("load critic builtin declarations: %v", err)
	}
	for _, declaration := range declarations.Tools {
		if declaration.Name != "run_point" {
			continue
		}
		if len(declaration.Config.PointToolDeclarations) == 0 {
			t.Fatal("run_point config has no point_tool_declarations")
		}
		for _, requirement := range declaration.Requirements.Input {
			if requirement == "must require point_machine, point_tools, and point_tool_declarations in config" {
				return
			}
		}
		t.Fatalf("run_point input requirements do not name point_tool_declarations: %v", declaration.Requirements.Input)
	}
	t.Fatal("run_point declaration not found")
}

func criticProfilePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(repoRootFromTest(t), "agents", "critic", name)
}

func criticMachine(t *testing.T, name string) criticMachineConfig {
	t.Helper()
	var machine criticMachineConfig
	if err := readYAML(criticProfilePath(t, name), &machine); err != nil {
		t.Fatalf("load critic %s: %v", name, err)
	}
	return machine
}

func criticToolSelection(t *testing.T, name string) []string {
	t.Helper()
	var selection criticToolSelectionFile
	if err := readYAML(criticProfilePath(t, name), &selection); err != nil {
		t.Fatalf("load critic %s: %v", name, err)
	}
	return selection.Tools
}

func requireSelected(t *testing.T, selection []string, word string) {
	t.Helper()
	for _, name := range selection {
		if name == word {
			return
		}
	}
	t.Fatalf("word %q is not selected", word)
}

func requireNotSelected(t *testing.T, selection []string, word string) {
	t.Helper()
	for _, name := range selection {
		if name == word {
			t.Fatalf("retired word %q is still selected", word)
		}
	}
}

func assertTransition(t *testing.T, machine criticMachineConfig, state, signal, next, action string) {
	t.Helper()
	for _, tr := range machine.Transitions {
		if tr.State == state && tr.Signal == signal {
			if tr.Next != next {
				t.Fatalf("%s + %s: next is %q, want %q", state, signal, tr.Next, next)
			}
			if tr.Action != action {
				t.Fatalf("%s + %s: action is %q, want %q", state, signal, tr.Action, action)
			}
			return
		}
	}
	t.Fatalf("missing transition %s + %s", state, signal)
}

func findTransition(machine criticMachineConfig, state, signal string) *criticTransition {
	for i := range machine.Transitions {
		tr := &machine.Transitions[i]
		if tr.State == state && tr.Signal == signal {
			return tr
		}
	}
	return nil
}
