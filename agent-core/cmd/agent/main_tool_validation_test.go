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

	require.NoError(t, validateRuntimeToolWiring(machine, []catalog.ToolDef{incomplete}),
		"ordinary startup accepts incomplete descriptive metadata when wiring is safe")
	require.NotEmpty(t,
		catalog.ValidateToolContracts([]catalog.ToolDef{incomplete},
			catalog.ContractValidationOptions{}),
		"authoring/audit validation still reports the incomplete contract")

	badWiring := incomplete
	badWiring.Emits = []string{"UndeclaredSignal"}
	require.ErrorContains(t,
		validateRuntimeToolWiring(machine, []catalog.ToolDef{badWiring}),
		"tool emits validation",
		"ordinary startup rejects emitted signals the machine cannot route")
}
