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

	require.NoError(t, validateRuntimeToolWiring(machine, []catalog.ToolDef{incomplete}, nil),
		"ordinary startup accepts incomplete descriptive metadata when wiring is safe")
	require.NotEmpty(t,
		catalog.ValidateToolContracts([]catalog.ToolDef{incomplete},
			catalog.ContractValidationOptions{}),
		"authoring/audit validation still reports the incomplete contract")

	badWiring := incomplete
	badWiring.Emits = []string{"UndeclaredSignal"}
	require.ErrorContains(t,
		validateRuntimeToolWiring(machine, []catalog.ToolDef{badWiring}, nil),
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

	require.NoError(t, validateRuntimeToolWiring(machine, []catalog.ToolDef{read, report}, nil),
		"a selector naming a published label loads")

	typo := report
	typo.Config = map[string]interface{}{"source": "$from(fetchedd).body"}
	err := validateRuntimeToolWiring(machine, []catalog.ToolDef{read, typo}, nil)
	require.ErrorContains(t, err, "unresolved selector labels")
	require.ErrorContains(t, err, `tool "report"`)
	require.ErrorContains(t, err, `$from(fetchedd).body`)
	require.ErrorContains(t, err, `closest declared label is "fetched"`)

	seeded := machine
	seeded.ExternalLabels = []core.ExternalLabel{{Name: "fetchedd"}}
	require.NoError(t, validateRuntimeToolWiring(seeded, []catalog.ToolDef{read, typo}, nil),
		"declaring the label as runtime-seeded resolves the same selector")
}
