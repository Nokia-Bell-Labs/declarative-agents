// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestSentenceToolMigrationCreatesReplacementWords(t *testing.T) {
	repo := repositoryRoot(t)
	sessionSelection := loadToolSelection(t, filepath.Join(repo, "agent-profiles", "agents", "evaluator", "tools.yaml"))
	require.NotContains(t, sessionSelection, "load_suite")

	replacementWords := []string{
		"parse_suite_config",
		"discover_suite_samples",
		"expand_eval_grid",
		"init_eval_session",
		"report_suite_summary",
	}
	for _, word := range replacementWords {
		require.Contains(t, sessionSelection, word)
	}

	defs := loadToolDefs(t, filepath.Join(repo, "agent-core", "tools", "builtin", "evaluator-session", "all.yaml"))
	selected := selectToolDefs(t, defs, replacementWords)
	for _, def := range selected {
		require.NotEmpty(t, def.Description, "%s description", def.Name)
		require.NotEmpty(t, def.Emits, "%s emits", def.Name)
		require.NotEmpty(t, def.SideEffects.Items, "%s side effects", def.Name)
		require.NotEmpty(t, def.Reversibility.Classification, "%s reversibility", def.Name)
		require.NotEmpty(t, def.Undo.Strategy, "%s undo", def.Name)
	}
}

func TestSentenceToolMigrationMachineSequence(t *testing.T) {
	repo := repositoryRoot(t)
	spec := loadMachineSpec(t, filepath.Join(repo, "agent-profiles", "agents", "evaluator", "machine.yaml"))
	selection := loadToolSelection(t, filepath.Join(repo, "agent-profiles", "agents", "evaluator", "tools.yaml"))
	defs := loadToolDefs(t, filepath.Join(repo, "agent-core", "tools", "builtin", "evaluator-session", "all.yaml"))
	selected := selectToolDefs(t, defs, selection)

	require.NoError(t, catalog.ValidateToolEmits(spec, selected))
	assertTransition(t, spec, "Idle", "Seed", "ParsingSuiteConfig", "parse_suite_config")
	assertTransition(t, spec, "ParsingSuiteConfig", "SuiteConfigParsed", "DiscoveringSuiteSamples", "discover_suite_samples")
	assertTransition(t, spec, "DiscoveringSuiteSamples", "SuiteSamplesDiscovered", "ExpandingEvalGrid", "expand_eval_grid")
	assertTransition(t, spec, "ExpandingEvalGrid", "EvalGridExpanded", "InitializingEvalSession", "init_eval_session")
	assertTransition(t, spec, "InitializingEvalSession", "EvalSessionInitialized", "ReportingSuiteSummary", "report_suite_summary")
	assertTransition(t, spec, "ReportingSuiteSummary", "SuiteLoaded", "AdvancingPoint", "next_point")
}

func TestSentenceToolMigrationPreservesFailureSignals(t *testing.T) {
	repo := repositoryRoot(t)
	spec := loadMachineSpec(t, filepath.Join(repo, "agent-profiles", "agents", "evaluator", "point.yaml"))
	selection := loadToolSelection(t, filepath.Join(repo, "agent-profiles", "agents", "evaluator", "tools-point.yaml"))
	defs := loadToolDefs(t, filepath.Join(repo, "agent-core", "tools", "builtin", "evaluator-point", "all.yaml"))
	selected := selectToolDefs(t, defs, selection)

	require.NoError(t, catalog.ValidateToolEmits(spec, selected))
	assertTransition(t, spec, "RunningAgent", "HarnessFailed", "RunningOracleCheck", "run_oracle_check")
	assertTransition(t, spec, "RunningAgent", "HarnessTimedOut", "RunningOracleCheck", "run_oracle_check")
	assertTransition(t, spec, "RunningOracleCheck", "OracleCheckFailed", "CollectingTraceTokens", "collect_trace_tokens")
	assertTransition(t, spec, "CollectingTraceTokens", "TraceTokensCollected", "CheckingAgentVersion", "check_agent_version")
	assertTransition(t, spec, "CheckingAgentVersion", "AgentVersionMismatch", "SummarizingPointResults", "summarize_point_results")
	assertTransition(t, spec, "SummarizingPointResults", "ResultsCollected", "CollectingMetrics", "collect_metrics")
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func loadMachineSpec(t *testing.T, path string) core.MachineSpec {
	t.Helper()
	spec, err := core.LoadMachineSpec(path)
	require.NoError(t, err)
	return spec
}

func loadToolSelection(t *testing.T, path string) []string {
	t.Helper()
	names, err := catalog.LoadToolSelection(path)
	require.NoError(t, err)
	return names
}

func loadToolDefs(t *testing.T, path string) []catalog.ToolDef {
	t.Helper()
	defs, err := catalog.LoadToolDeclarations([]string{path})
	require.NoError(t, err)
	return defs
}

func selectToolDefs(t *testing.T, defs []catalog.ToolDef, selection []string) []catalog.ToolDef {
	t.Helper()
	selected, err := catalog.SelectTools(defs, selection)
	require.NoError(t, err)
	return selected
}

func assertTransition(t *testing.T, spec core.MachineSpec, state, signal, next, action string) {
	t.Helper()
	for _, tr := range spec.Transitions {
		if tr.State == state && tr.Signal == signal {
			require.Equal(t, next, tr.Next)
			require.Equal(t, action, tr.Action)
			return
		}
	}
	t.Fatalf("missing transition %s + %s", state, signal)
}
