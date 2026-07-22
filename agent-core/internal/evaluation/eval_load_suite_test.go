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

// These tests cover the two things agent-core owns in the sentence-tool
// migration: the core-owned evaluator builtin declarations under tools/builtin,
// and the reusable loader plus emits-validation contract that any session
// machine is checked against. Neither reads a shipped agent profile.
//
// The assertion that the shipped critic profile actually sequences these words
// belongs to the repository that owns the profile, and lives in
// agent-profiles/magefiles/critic_wiring_test.go (srd034 R2.1, R2.2; GH-512).

// sessionWords are the evaluator session words that replaced the single
// load_suite word. agent-core owns their declarations (srd034 R1.2).
var sessionWords = []string{
	"parse_suite_config",
	"discover_suite_samples",
	"expand_eval_grid",
	"init_eval_session",
	"report_suite_summary",
}

// pointWords are the evaluator point words whose failure paths the point
// machine routes on. agent-core owns their declarations too.
var pointWords = []string{
	"run_oracle_check",
	"collect_trace_tokens",
	"check_agent_version",
	"summarize_point_results",
	"collect_metrics",
}

// TestSessionWordsDeclareFullContract proves the core-owned session
// declarations carry the contract metadata a sentence tool must publish. A word
// with no declared emits or no reversibility cannot be reasoned about by a
// machine author or by emits validation.
func TestSessionWordsDeclareFullContract(t *testing.T) {
	defs := loadToolDefs(t, coreBuiltinPath(t, "critic-session"))
	requireFullToolContract(t, selectToolDefs(t, defs, sessionWords))
}

// TestPointWordsDeclareFullContract is the same contract check for the point
// words. It is separate because the point words live in their own core-owned
// declaration tree.
func TestPointWordsDeclareFullContract(t *testing.T) {
	defs := loadToolDefs(t, coreBuiltinPath(t, "critic-point"))
	requireFullToolContract(t, selectToolDefs(t, defs, pointWords))
}

// TestSessionWordsReplaceLoadSuite records that load_suite was retired rather
// than kept alongside its replacements, so a profile cannot select the old word
// from a core-owned declaration tree.
func TestSessionWordsReplaceLoadSuite(t *testing.T) {
	defs := loadToolDefs(t, coreBuiltinPath(t, "critic-session"))
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	require.NotContains(t, names, "load_suite")
	for _, word := range sessionWords {
		require.Contains(t, names, word)
	}
}

// TestSessionPipelineEmitsValidate proves the reusable contract on synthetic
// fixtures: a selection resolves against declarations, and a machine whose
// stages are chained by tool emits passes validation. This is the contract the
// shipped profiles are held to, proven without any profile path.
func TestSessionPipelineEmitsValidate(t *testing.T) {
	spec := loadMachineSpec(t, fixturePath("session_machine.yaml"))
	selection := loadToolSelection(t, fixturePath("session_tools.yaml"))
	defs := loadToolDefs(t, fixturePath("session_declarations.yaml"))
	selected := selectToolDefs(t, defs, selection)

	require.NoError(t, catalog.ValidateToolEmits(spec, selected))
	assertTransition(t, spec, "Idle", "Seed", "Loading", "load_input")
	assertTransition(t, spec, "Loading", "Loaded", "Preparing", "prepare_work")
	assertTransition(t, spec, "Preparing", "Prepared", "Reporting", "report_result")
}

// TestSessionPipelineFailureSignalsRoute proves every stage tool's failure emit
// has somewhere to go. A failure signal with no transition strands the machine,
// and emits validation is what catches it.
func TestSessionPipelineFailureSignalsRoute(t *testing.T) {
	spec := loadMachineSpec(t, fixturePath("session_machine.yaml"))
	for _, state := range []string{"Loading", "Preparing", "Reporting"} {
		assertTransition(t, spec, state, "StepFailed", "Failed", "")
	}
}

// TestUndeclaredEmitIsRejected is the negative half of the contract. Loading
// succeeds -- the declaration file is well formed -- so validation, not the
// loader, must reject a word that emits a signal the machine never declares.
func TestUndeclaredEmitIsRejected(t *testing.T) {
	spec := loadMachineSpec(t, fixturePath("session_machine.yaml"))
	selection := loadToolSelection(t, fixturePath("session_tools.yaml"))
	defs := loadToolDefs(t, fixturePath("session_declarations_undeclared_emit.yaml"))
	selected := selectToolDefs(t, defs, selection)

	err := catalog.ValidateToolEmits(spec, selected)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Summarized")
}

// TestSelectionRejectsUndeclaredWord covers the other loader failure a profile
// author hits: selecting a word no declaration tree provides.
func TestSelectionRejectsUndeclaredWord(t *testing.T) {
	defs := loadToolDefs(t, fixturePath("session_declarations.yaml"))
	_, err := catalog.SelectTools(defs, []string{"load_input", "no_such_word"})
	require.ErrorContains(t, err, "no_such_word")
}

func requireFullToolContract(t *testing.T, defs []catalog.ToolDef) {
	t.Helper()
	for _, def := range defs {
		require.NotEmpty(t, def.Description, "%s description", def.Name)
		require.NotEmpty(t, def.Emits, "%s emits", def.Name)
		require.NotEmpty(t, def.SideEffects.Items, "%s side effects", def.Name)
		require.NotEmpty(t, def.Reversibility.Classification, "%s reversibility", def.Name)
		require.NotEmpty(t, def.Undo.Strategy, "%s undo", def.Name)
	}
}

// agentCoreRoot is the agent-core module root, derived from this test file so
// it holds wherever the module is checked out. It never reaches outside the
// module (srd034 R1.2, R2.1).
func agentCoreRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func coreBuiltinPath(t *testing.T, family string) string {
	t.Helper()
	return filepath.Join(agentCoreRoot(t), "tools", "builtin", family, "all.yaml")
}

func fixturePath(name string) string {
	return filepath.Join("testdata", name)
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
