// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestRuntimeStartupValidatesWiringNotFullContractCompleteness(t *testing.T) {
	machine := core.MachineSpec{
		Name:           "startup-boundary",
		InitialState:   "Idle",
		States:         core.StateSpecs{{Name: "Idle"}, {Name: "Working"}, {Name: "Done"}},
		TerminalStates: []string{"Done"},
		Signals:        core.SignalSpecsFromNames("Seed", "ToolDone"),
		Transitions: []core.TransitionSpec{
			{State: "Idle", Signal: "Seed", Next: "Working", Action: "read"},
			{State: "Working", Signal: "ToolDone", Next: "Done"},
		},
	}
	incomplete := catalog.ToolDef{
		Name: "read", Type: "builtin", Init: "file_read",
		Emits: []string{"ToolDone"},
	}

	require.NoError(t, validateRuntimeToolWiring(machine, []catalog.ToolDef{incomplete}, nil, nil, catalog.ExhaustivenessInputs{}),
		"ordinary startup accepts incomplete descriptive metadata when wiring is safe")
	badWiring := incomplete
	badWiring.Emits = []string{"UndeclaredSignal"}
	require.ErrorContains(t,
		validateRuntimeToolWiring(machine, []catalog.ToolDef{badWiring}, nil, nil, catalog.ExhaustivenessInputs{}),
		"tool emits validation",
		"ordinary startup rejects emitted signals the machine cannot route")
}

// A typo'd $from(label) used to reach the runtime and fail there as an
// UnresolvedLabelError. The startup boundary now rejects it (GH-1966).
func TestRuntimeStartupRejectsUnresolvedSelectorLabel(t *testing.T) {
	machine := core.MachineSpec{
		Name:           "selector-labels",
		InitialState:   "Idle",
		States:         core.StateSpecs{{Name: "Idle"}, {Name: "Working"}, {Name: "Done"}},
		TerminalStates: []string{"Done"},
		Signals:        core.SignalSpecsFromNames("Seed", "ToolDone"),
		Transitions: []core.TransitionSpec{
			{State: "Idle", Signal: "Seed", Next: "Working", Action: "read", Label: "fetched"},
			{State: "Working", Signal: "ToolDone", Next: "Done", Action: "report"},
		},
	}
	read := catalog.ToolDef{Name: "read", Type: "builtin", Init: "file_read", Emits: []string{"ToolDone"}}
	report := catalog.ToolDef{
		Name: "report", Type: "builtin", Init: "file_read", Emits: []string{"ToolDone"},
		Config: map[string]interface{}{"source": "$from(fetched).body"},
	}

	require.NoError(t, validateRuntimeToolWiring(machine, []catalog.ToolDef{read, report}, nil, nil, catalog.ExhaustivenessInputs{}),
		"a selector naming a published label loads")

	typo := report
	typo.Config = map[string]interface{}{"source": "$from(fetchedd).body"}
	err := validateRuntimeToolWiring(machine, []catalog.ToolDef{read, typo}, nil, nil, catalog.ExhaustivenessInputs{})
	require.ErrorContains(t, err, "unresolved selector labels")
	require.ErrorContains(t, err, `tool "report"`)
	require.ErrorContains(t, err, `$from(fetchedd).body`)
	require.ErrorContains(t, err, `closest declared label is "fetched"`)

	seeded := machine
	seeded.ExternalLabels = []core.ExternalLabel{{Name: "fetchedd"}}
	require.NoError(t, validateRuntimeToolWiring(seeded, []catalog.ToolDef{read, typo}, nil, nil, catalog.ExhaustivenessInputs{}),
		"declaring the label as runtime-seeded resolves the same selector")
}

// A transition waiting on a signal nothing entering its state emits can never
// fire. The startup boundary now rejects it (GH-2005). The converse direction
// is ValidateToolEmits', asserted here too so the two stay distinguishable.
func TestRuntimeStartupRejectsADeadTransition(t *testing.T) {
	machine := core.MachineSpec{
		Name: "liveness", InitialState: "Idle",
		States:         core.StateSpecs{{Name: "Idle"}, {Name: "Working"}, {Name: "Done"}},
		TerminalStates: []string{"Done"},
		Signals:        core.SignalSpecsFromNames("Seed", "Worked", "CommandError"),
		Transitions: []core.TransitionSpec{
			{State: "Idle", Signal: "Seed", Next: "Working", Action: "work"},
			{State: "Working", Signal: "Worked", Next: "Done", Action: "finish"},
			{State: "Working", Signal: "CommandError", Next: "Done", Action: "finish"},
		},
	}
	defs := []catalog.ToolDef{
		{Name: "work", Type: "builtin", Init: "file_read", Emits: []string{"Worked", "CommandError"}},
		{Name: "finish", Type: "builtin", Init: "file_read", Emits: []string{"Worked"}},
	}

	require.NoError(t,
		validateRuntimeToolWiring(machine, defs, nil, nil, catalog.ExhaustivenessInputs{}),
		"a machine whose every transition can fire loads")

	// This is the shape the conformance lifecycle machines carried: a route for
	// a signal the action behind it never emits.
	dead := machine
	dead.Signals = core.SignalSpecsFromNames("Seed", "Worked", "CommandError", "ToolFailed")
	dead.Transitions = append(append([]core.TransitionSpec{}, machine.Transitions...),
		core.TransitionSpec{State: "Working", Signal: "ToolFailed", Next: "Done"})

	err := validateRuntimeToolWiring(dead, defs, nil, nil, catalog.ExhaustivenessInputs{})
	require.ErrorContains(t, err, "machine is not exhaustive")
	require.ErrorContains(t, err, `waits on a signal nothing entering "Working" emits`)

	// A request signal source injecting it makes the same transition live.
	require.NoError(t, validateRuntimeToolWiring(
		dead, defs, nil, nil, catalog.ExhaustivenessInputs{External: []string{"ToolFailed"}}),
		"an externally injected signal is not a dead route")

	unhandled := machine
	unhandled.Transitions = machine.Transitions[:2]
	require.ErrorContains(t,
		validateRuntimeToolWiring(unhandled, defs, nil, nil, catalog.ExhaustivenessInputs{}),
		"tool emits validation",
		"an unhandled emitted signal is ValidateToolEmits' finding, not this one")
}
