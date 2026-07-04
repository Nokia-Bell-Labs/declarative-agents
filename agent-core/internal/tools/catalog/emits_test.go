// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestDeriveDynamicToolPhasesScopesExternalToolsByRoutableSignals(t *testing.T) {
	t.Parallel()
	spec := dynamicPhaseSpec()
	defs := []ToolDef{
		{Name: "read", Type: "builtin", Init: "file_read", Emits: []string{"ToolDone", "ToolFailed"}},
		{Name: "edit", Type: "builtin", Init: "file_edit", Emits: []string{"EditDone", "ToolFailed"}},
		{Name: "broken", Type: "builtin", Init: "broken", Emits: []string{"MissingRoute"}},
		{Name: "silent", Type: "builtin", Init: "silent"},
		{Name: "parse_response", Type: "builtin", Init: "parse_response", Visibility: "internal", Emits: []string{"ToolDone"}},
	}

	phases, scoped := DeriveDynamicToolPhases(spec, defs)

	require.True(t, scoped)
	require.Equal(t, []core.State{"Composing"}, phases["read"])
	require.Equal(t, []core.State{"Composing"}, phases["edit"])
	require.NotContains(t, phases, "broken")
	require.NotContains(t, phases, "silent")
	require.NotContains(t, phases, "parse_response")
}

func TestApplyDynamicToolPhasesMakesMissingEmitToolsUnavailable(t *testing.T) {
	t.Parallel()
	defs := ApplyDynamicToolPhases(dynamicPhaseSpec(), []ToolDef{
		{Name: "read", Type: "builtin", Init: "file_read", Emits: []string{"ToolDone", "ToolFailed"}},
		{Name: "silent", Type: "builtin", Init: "silent"},
	})

	read := defs[0].ToToolSpec()
	silent := defs[1].ToToolSpec()

	require.True(t, read.AvailableIn("Composing"))
	require.False(t, read.AvailableIn("Parsing"))
	require.True(t, silent.PhaseScoped)
	require.False(t, silent.AvailableIn("Composing"))
}

func TestApplyDynamicToolPhasesFiltersRegistryManifest(t *testing.T) {
	t.Parallel()
	defs := ApplyDynamicToolPhases(dynamicPhaseSpec(), []ToolDef{
		{Name: "read", Type: "builtin", Init: "file_read", Emits: []string{"ToolDone", "ToolFailed"}},
		{Name: "broken", Type: "builtin", Init: "broken", Emits: []string{"MissingRoute"}},
		{Name: "parse_response", Type: "builtin", Init: "parse_response", Visibility: "internal", Emits: []string{"ToolDone"}},
	})
	reg := core.NewRegistry()
	for _, def := range defs {
		reg.Register(def.ToToolSpec(), noopBuilder{})
	}

	requireManifestNames(t, reg.Manifest("Composing"), []string{"read"})
	requireManifestNames(t, reg.Manifest("Parsing"), []string{})
}

func TestDeriveDynamicToolPhasesRespectsExplicitPhaseNarrowing(t *testing.T) {
	t.Parallel()
	spec := dynamicPhaseSpec()
	defs := []ToolDef{
		{Name: "read", Type: "builtin", Init: "file_read", Emits: []string{"ToolDone", "ToolFailed"}, Phases: []string{"Reviewing"}},
		{Name: "edit", Type: "builtin", Init: "file_edit", Emits: []string{"EditDone", "ToolFailed"}, Phases: []string{"Composing"}},
	}

	phases, scoped := DeriveDynamicToolPhases(spec, defs)

	require.True(t, scoped)
	require.NotContains(t, phases, "read")
	require.Equal(t, []core.State{"Composing"}, phases["edit"])
}

func TestDeriveDynamicToolPhasesAllowsTerminalTargets(t *testing.T) {
	t.Parallel()
	spec := core.MachineSpec{
		Name:           "terminal-dynamic",
		States:         core.StateSpecsFromNames("Parsing", "Done"),
		TerminalStates: []string{"Done"},
		Signals:        core.SignalSpecsFromNames("ToolReady"),
		Transitions: []core.TransitionSpec{
			{State: "Parsing", Signal: "ToolReady", Next: "Done", Action: "$tool"},
		},
	}
	defs := []ToolDef{{Name: "finish", Type: "builtin", Init: "finish"}}

	phases, scoped := DeriveDynamicToolPhases(spec, defs)

	require.True(t, scoped)
	require.Equal(t, []core.State{"Done"}, phases["finish"])
}

type noopBuilder struct{}

func (noopBuilder) Build(core.Result) core.Command { return noopCmd{} }

type noopCmd struct{}

func (noopCmd) Name() string                   { return "noop" }
func (noopCmd) Execute() core.Result           { return core.Result{Signal: core.ToolDone} }
func (noopCmd) Undo(_ core.Result) core.Result { return core.NoopUndo("noop") }

func requireManifestNames(t *testing.T, specs []core.ToolSpec, want []string) {
	t.Helper()
	got := make([]string, 0, len(specs))
	for _, spec := range specs {
		got = append(got, spec.Name)
	}
	require.Equal(t, want, got)
}

func dynamicPhaseSpec() core.MachineSpec {
	return core.MachineSpec{
		Name:           "dynamic",
		States:         core.StateSpecsFromNames("Parsing", "Composing", "Done", "Failed"),
		TerminalStates: []string{"Done", "Failed"},
		Signals:        core.SignalSpecsFromNames("ToolReady", "ToolDone", "ToolFailed", "EditDone", "MissingRoute"),
		Transitions: []core.TransitionSpec{
			{State: "Parsing", Signal: "ToolReady", Next: "Composing", Action: "$tool"},
			{State: "Composing", Signal: "ToolDone", Next: "Composing"},
			{State: "Composing", Signal: "ToolFailed", Next: "Failed"},
			{State: "Composing", Signal: "EditDone", Next: "Composing"},
		},
	}
}
