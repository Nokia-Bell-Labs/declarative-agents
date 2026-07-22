// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestValidate_ToolMetricConfig(t *testing.T) {
	corpus := &Corpus{
		ToolDeclarations: map[string]ToolDeclaration{
			"valid": {
				Name: "valid",
				Metrics: core.MetricConfig{
					Instruments: []core.MetricInstrument{{
						Name:        "rest.request_duration",
						Kind:        "histogram",
						Description: "Duration.",
						ValueSource: "dispatch_duration",
					}},
				},
			},
			"bad": {
				Name: "bad",
				Metrics: core.MetricConfig{
					Instruments: []core.MetricInstrument{{
						Name:        "bad",
						Kind:        "summary",
						Description: "Invalid.",
						ValueSource: "dispatch_count",
					}},
				},
			},
		},
	}

	findings := checkToolMetricConfig(corpus)

	require.Len(t, findings, 1)
	assert.Equal(t, "tool-metric-config-invalid", findings[0].Check)
	assert.Contains(t, findings[0].Message, "summary")
}

func TestValidate_ToolSelectionDeclared(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"agent": {
				Name: "agent", InitialState: "Idle",
				States: core.StateSpecs{{Name: "Idle"}}, Signals: core.SignalSpecs{{Name: "Seed"}},
				Transitions:    []core.TransitionSpec{{State: "Idle", Signal: "Seed", Next: "Idle"}},
				TerminalStates: []string{"Idle"},
			},
		},
		ToolSelections:   map[string][]string{"agent": {"exists", "missing"}},
		ToolDeclarations: map[string]ToolDeclaration{"exists": {Name: "exists"}},
		MachineOrder:     []string{"agent"},
	}

	findings := checkToolSelectionDeclared(corpus)
	require.Len(t, findings, 1)
	assert.Equal(t, "error", findings[0].Level)
	assert.Contains(t, findings[0].Message, "missing")
}

func TestValidate_SelectedToolContractCompletenessErrorsForActiveMigratedTool(t *testing.T) {
	corpus := &Corpus{
		ToolSelections: map[string][]string{
			"agent": {"sparse"},
		},
		ToolDeclarations: map[string]ToolDeclaration{
			"sparse": {
				Name:     "sparse",
				Contract: "migrated",
				Emits:    []string{"ToolDone"},
			},
		},
	}

	findings := checkSelectedToolContractCompleteness(corpus)

	require.Len(t, findings, 1)
	assert.Equal(t, "tool-contract-incomplete", findings[0].Check)
	assert.Equal(t, "error", findings[0].Level)
	assert.Contains(t, findings[0].Message, "sparse")
	assert.Contains(t, findings[0].Message, "problem")
	assert.Contains(t, findings[0].Message, "output.schema")
}

func TestValidate_SelectedToolContractCompletenessKeepsLegacyWarningOnly(t *testing.T) {
	corpus := &Corpus{
		ToolSelections: map[string][]string{
			"agent": {"legacy"},
		},
		ToolDeclarations: map[string]ToolDeclaration{
			"legacy": {
				Name:     "legacy",
				Contract: "legacy",
				Emits:    []string{"ToolDone"},
			},
		},
	}

	findings := checkSelectedToolContractCompleteness(corpus)

	require.Len(t, findings, 1)
	assert.Equal(t, "warning", findings[0].Level)
	assert.Contains(t, findings[0].Message, "legacy")
}

func TestValidate_SelectedToolContractCompletenessIgnoresUnselectedTool(t *testing.T) {
	corpus := &Corpus{
		ToolSelections: map[string][]string{
			"agent": {"complete"},
		},
		ToolDeclarations: map[string]ToolDeclaration{
			"complete": completeToolDeclaration("complete"),
			"sparse":   {Name: "sparse"},
		},
	}

	findings := checkSelectedToolContractCompleteness(corpus)

	require.Empty(t, findings)
}

func TestValidate_SelectedToolContractCompletenessPassesCompleteTool(t *testing.T) {
	corpus := &Corpus{
		ToolSelections: map[string][]string{
			"agent":       {"complete"},
			"agent:point": {"complete"},
		},
		ToolDeclarations: map[string]ToolDeclaration{
			"complete": completeToolDeclaration("complete"),
		},
	}

	findings := checkSelectedToolContractCompleteness(corpus)

	require.Empty(t, findings)
}

func TestDiscoverToolDeclarationsIncludesProfileFilesAndDirs(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "agents", "executor", "llm"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "tools", "builtin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "executor", "profile.yaml"), []byte(`
name: generator
machine: machine.yaml
tools:
  - tools.yaml
tool_config_dirs:
  - ../../tools/builtin
tool_declarations:
  - llm/default.yaml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "executor", "llm", "default.yaml"), []byte(`
tools:
  - name: invoke_llm
    type: builtin
    init: invoke_llm
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tools", "builtin", "read.yaml"), []byte(`
tools:
  - name: read
    type: builtin
    init: file_read
`), 0o644))

	decls, err := discoverAndParseToolDeclarations(root)

	require.NoError(t, err)
	require.Contains(t, decls, "invoke_llm")
	require.Contains(t, decls, "read")
}

func TestDiscoverToolDeclarationsAcceptsRESTSideEffectKinds(t *testing.T) {
	root := t.TempDir()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	restDecls := filepath.Join(repoRoot, "tools", "builtin", "rest", "all.yaml")

	require.NoError(t, os.MkdirAll(filepath.Join(root, "agents", "rest"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "rest", "profile.yaml"), []byte(`
name: rest
tool_declarations:
  - `+restDecls+`
`), 0o644))

	decls, err := discoverAndParseToolDeclarations(root)

	require.NoError(t, err)
	require.Contains(t, decls, "rest_server_launch")
	require.Contains(t, decls, "rest_server_stop")
	require.Contains(t, decls, "rest_await_event")
	corpus := &Corpus{ToolDeclarations: decls}
	assert.Empty(t, checkToolSideEffectVocab(corpus))
	assert.Empty(t, checkToolBoundaryCategory(corpus))
}

func TestDiscoverToolDeclarationsKeepsAgentLocalOverrides(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "agents", "bench"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "tools", "builtin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "bench", "profile.yaml"), []byte(`
name: bench
machine: machine.yaml
tools:
  - tools.yaml
tool_config_dirs:
  - ../../tools/builtin
tool_declarations:
  - builtin.yaml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "bench", "builtin.yaml"), []byte(`
tools:
  - name: serve_ui
    type: builtin
    init: serve_ui
    problem: local override
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tools", "builtin", "serve-ui.yaml"), []byte(`
tools:
  - name: serve_ui
    type: builtin
    init: serve_ui
    problem: shared compatibility declaration
`), 0o644))

	decls, err := discoverAndParseToolDeclarations(root)

	require.NoError(t, err)
	require.Contains(t, decls, "serve_ui")
	assert.Equal(t, "agents/bench/builtin.yaml", decls["serve_ui"].SourceFile)
	assert.Equal(t, "local override", decls["serve_ui"].Problem)
}

func TestValidate_ToolEmitsSignalSet(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"agent": {
				Name: "agent", InitialState: "Idle",
				States:         core.StateSpecs{{Name: "Idle"}},
				Signals:        core.SignalSpecs{{Name: "Seed"}, {Name: "ToolDone"}},
				Transitions:    []core.TransitionSpec{{State: "Idle", Signal: "Seed", Next: "Idle", Action: "work"}},
				TerminalStates: []string{"Idle"},
			},
		},
		ToolSelections: map[string][]string{"agent": {"work"}},
		ToolDeclarations: map[string]ToolDeclaration{
			"work": {Name: "work", Emits: []string{"ToolDone", "UnknownSignal"}},
		},
		MachineOrder: []string{"agent"},
	}

	findings := checkToolEmitsSignalSet(corpus)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "UnknownSignal")
}

// TestValidate_ToolEmitsSignalSetSkipsUndispatchedTool checks that a tool the
// profile selects but the machine never dispatches — the REST or
// machine_request binding case — is not validated against the machine's signal
// set, since it routes its signals in a request-scoped sentence.
func TestValidate_ToolEmitsSignalSetSkipsUndispatchedTool(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"agent": {
				Name: "agent", InitialState: "Idle",
				States:         core.StateSpecs{{Name: "Idle"}},
				Signals:        core.SignalSpecs{{Name: "Seed"}},
				Transitions:    []core.TransitionSpec{{State: "Idle", Signal: "Seed", Next: "Idle"}},
				TerminalStates: []string{"Idle"},
			},
		},
		ToolSelections: map[string][]string{"agent": {"rest_bound"}},
		ToolDeclarations: map[string]ToolDeclaration{
			"rest_bound": {Name: "rest_bound", Emits: []string{"RESTResponded"}},
		},
		MachineOrder: []string{"agent"},
	}

	assert.Empty(t, checkToolEmitsSignalSet(corpus))
}

// TestValidate_ToolEmitsSignalSetDynamicDispatch checks that a machine using
// $tool dynamic dispatch validates every selected tool, since it can invoke any
// of them.
func TestValidate_ToolEmitsSignalSetDynamicDispatch(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"agent": {
				Name: "agent", InitialState: "Idle",
				States:         core.StateSpecs{{Name: "Idle"}},
				Signals:        core.SignalSpecs{{Name: "Seed"}, {Name: "ToolDone"}},
				Transitions:    []core.TransitionSpec{{State: "Idle", Signal: "Seed", Next: "Idle", Action: "$tool"}},
				TerminalStates: []string{"Idle"},
			},
		},
		ToolSelections: map[string][]string{"agent": {"work"}},
		ToolDeclarations: map[string]ToolDeclaration{
			"work": {Name: "work", Emits: []string{"ToolDone", "UnknownSignal"}},
		},
		MachineOrder: []string{"agent"},
	}

	findings := checkToolEmitsSignalSet(corpus)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "UnknownSignal")
}

func TestValidate_ToolUndoConsistency(t *testing.T) {
	decls := map[string]ToolDeclaration{
		"invalid-irreversible": {
			Name:          "invalid-irreversible",
			Reversibility: ToolDeclReversibility{Classification: "irreversible"},
			Undo:          ToolDeclUndo{Strategy: "compensating_action"},
		},
		"payload-no-captures": {
			Name: "payload-no-captures",
			Undo: ToolDeclUndo{Strategy: "compensating_action", Payload: "boundary_compensation"},
		},
	}
	for _, strategy := range []string{
		"noop",
		"workspace_restore",
		"session_state_restore",
		"conversation_truncate",
		"conversation_restore",
		"parse_retry_counter_restore",
		"pipeline_state_restore",
		"evaluator_session_restore",
		"point_context_restore",
		"validation_state_restore",
	} {
		decls["reversible-"+strategy] = ToolDeclaration{
			Name:          "reversible-" + strategy,
			Reversibility: ToolDeclReversibility{Classification: "reversible"},
			Undo:          ToolDeclUndo{Strategy: strategy},
		}
	}
	for _, strategy := range []string{
		"compensating_action",
		"child_command_undo",
		"workspace_restore",
		"child_agent_workspace_restore",
		"nested_machine_rollback",
		"point_workspace_restore_and_child_process_compensation",
		"child_eval_artifact_compensation",
		"server_shutdown_or_user_action_compensation",
	} {
		decls["compensatable-"+strategy] = ToolDeclaration{
			Name:          "compensatable-" + strategy,
			Reversibility: ToolDeclReversibility{Classification: "compensatable"},
			Undo:          ToolDeclUndo{Strategy: strategy},
		}
	}

	corpus := &Corpus{ToolDeclarations: decls}

	findings := checkToolUndoConsistency(corpus)
	var checks []string
	for _, f := range findings {
		checks = append(checks, f.Check)
	}
	assert.Equal(t, 1, countFindings(checks, "tool-undo-mismatch"))
	assert.Contains(t, checks, "tool-undo-payload-no-captures")
}

func TestValidate_ToolSideEffectVocab(t *testing.T) {
	corpus := &Corpus{
		ToolDeclarations: map[string]ToolDeclaration{
			"good": {
				Name:        "good",
				SideEffects: ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "filesystem_read"}}},
			},
			"rest_launch": {
				Name:        "rest_launch",
				SideEffects: ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "network_listen"}}},
			},
			"rest_stop": {
				Name:        "rest_stop",
				SideEffects: ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "network_listener_shutdown"}}},
			},
			"bad": {
				Name:        "bad",
				SideEffects: ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "invented_kind"}}},
			},
		},
	}

	findings := checkToolSideEffectVocab(corpus)
	require.Len(t, findings, 1)
	assert.Equal(t, "error", findings[0].Level)
	assert.Contains(t, findings[0].Message, "invented_kind")
}

func TestValidate_ToolBoundaryCategory(t *testing.T) {
	corpus := &Corpus{
		ToolDeclarations: map[string]ToolDeclaration{
			"correct": {
				Name:        "correct",
				Category:    "boundary",
				SideEffects: ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "child_agent_execution"}}},
			},
			"wrong": {
				Name:        "wrong",
				Category:    "word",
				SideEffects: ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "external_api"}}},
			},
			"listener": {
				Name:        "listener",
				Category:    "word",
				SideEffects: ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "network_listen"}}},
			},
		},
	}

	findings := checkToolBoundaryCategory(corpus)
	require.Len(t, findings, 2)
	messages := []string{findings[0].Message, findings[1].Message}
	assert.Contains(t, messages, `tool "wrong" has boundary side effects but category is "word", expected "boundary"`)
	assert.Contains(t, messages, `tool "listener" has boundary side effects but category is "word", expected "boundary"`)
}

func TestValidate_ToolDeclNodesInGraph(t *testing.T) {
	g, c := loadTestGraphAndCorpus(t)

	require.Contains(t, c.ToolDeclarations, "do_work")

	declNodes := g.NodesByKind(KindToolDecl)
	require.NotEmpty(t, declNodes)

	var found bool
	for _, n := range declNodes {
		if n.ID == "tool-decl:do_work" {
			found = true
			break
		}
	}
	assert.True(t, found, "graph should contain tool-decl:do_work node")
}
