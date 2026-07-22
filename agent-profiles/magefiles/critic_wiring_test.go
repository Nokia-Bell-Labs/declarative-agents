// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"path/filepath"
	"testing"
)

// Shipped-critic conformance for the sentence-tool migration. agent-profiles
// owns the critic profile, so the assertion that its machines actually sequence
// the replacement words belongs here rather than in an agent-core package test
// reaching across the module boundary (srd034 R2.1, R2.2; GH-512).
//
// These read only agent-profiles assets. The generic checks -- every selected
// word is declared, and every emit has a transition -- run over all profiles in
// `mage validate`; what is proven here is the critic-specific sequence.

// criticSessionWords are the words that replaced the single load_suite word.
var criticSessionWords = []string{
	"parse_suite_config",
	"discover_suite_samples",
	"expand_eval_grid",
	"init_eval_session",
	"report_suite_summary",
}

// TestCriticSelectsSentenceWords proves the shipped session selection asks for
// the replacement words and no longer asks for the retired load_suite.
func TestCriticSelectsSentenceWords(t *testing.T) {
	selection := criticToolSelection(t, "tools.yaml")
	requireNotSelected(t, selection, "load_suite")
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
	assertTransition(t, machine, "ReportingSuiteSummary", "SuiteLoaded", "AdvancingPoint", "next_point")
}

// TestCriticPointFailureSignals pins the point machine's failure routing. A
// harness failure, an oracle failure, and a version mismatch each have to keep
// the point moving toward a summary rather than stranding the session, so these
// transitions are what make a failed point still produce a result.
func TestCriticPointFailureSignals(t *testing.T) {
	machine := criticMachine(t, "point.yaml")
	assertTransition(t, machine, "RunningAgent", "HarnessFailed", "RunningOracleCheck", "run_oracle_check")
	assertTransition(t, machine, "RunningAgent", "HarnessTimedOut", "RunningOracleCheck", "run_oracle_check")
	assertTransition(t, machine, "RunningOracleCheck", "OracleCheckFailed", "CollectingTraceTokens", "collect_trace_tokens")
	assertTransition(t, machine, "CollectingTraceTokens", "TraceTokensCollected", "CheckingAgentVersion", "check_agent_version")
	assertTransition(t, machine, "CheckingAgentVersion", "AgentVersionMismatch", "SummarizingPointResults", "summarize_point_results")
	assertTransition(t, machine, "SummarizingPointResults", "ResultsCollected", "CollectingMetrics", "collect_metrics")
}

func criticProfilePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(repoRootFromTest(t), "agents", "critic", name)
}

func criticMachine(t *testing.T, name string) machineConfig {
	t.Helper()
	machine, err := loadMachine(criticProfilePath(t, name))
	if err != nil {
		t.Fatalf("load critic %s: %v", name, err)
	}
	return machine
}

func criticToolSelection(t *testing.T, name string) []string {
	t.Helper()
	var selection toolSelectionFile
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

func assertTransition(t *testing.T, machine machineConfig, state, signal, next, action string) {
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
