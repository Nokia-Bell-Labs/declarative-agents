// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

func TestValidateToolEmitsValid(t *testing.T) {
	spec := core.MachineSpec{
		Name:           "test",
		States:         []string{"Idle", "Running", "Done", "Failed"},
		TerminalStates: []string{"Done", "Failed"},
		Signals:        []string{"Seed", "Ready", "CommandError"},
		Transitions: []core.TransitionSpec{
			{State: "Idle", Signal: "Seed", Next: "Running", Action: "start"},
			{State: "Running", Signal: "Ready", Next: "Done"},
			{State: "Running", Signal: "CommandError", Next: "Failed"},
		},
	}
	defs := []ToolDef{
		{Name: "start", Type: "builtin", Init: "start", Emits: []string{"Ready", "CommandError"}},
	}

	require.NoError(t, ValidateToolEmits(spec, defs))
}

func TestValidateToolEmitsUnknownSignal(t *testing.T) {
	spec := core.MachineSpec{
		Name:           "test",
		States:         []string{"Idle", "Running", "Done"},
		TerminalStates: []string{"Done"},
		Signals:        []string{"Seed", "Ready"},
		Transitions: []core.TransitionSpec{
			{State: "Idle", Signal: "Seed", Next: "Running", Action: "start"},
			{State: "Running", Signal: "Ready", Next: "Done"},
		},
	}
	defs := []ToolDef{
		{Name: "start", Type: "builtin", Init: "start", Emits: []string{"MissingSignal"}},
	}

	err := ValidateToolEmits(spec, defs)
	require.Error(t, err)
	require.Contains(t, err.Error(), `tool "start" emits signal "MissingSignal"`)
}

func TestValidateToolEmitsMissingFollowupTransition(t *testing.T) {
	spec := core.MachineSpec{
		Name:           "test",
		States:         []string{"Idle", "Running", "Done"},
		TerminalStates: []string{"Done"},
		Signals:        []string{"Seed", "Ready", "CommandError"},
		Transitions: []core.TransitionSpec{
			{State: "Idle", Signal: "Seed", Next: "Running", Action: "start"},
			{State: "Running", Signal: "Ready", Next: "Done"},
		},
	}
	defs := []ToolDef{
		{Name: "start", Type: "builtin", Init: "start", Emits: []string{"Ready", "CommandError"}},
	}

	err := ValidateToolEmits(spec, defs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "has no transition for Running/CommandError")
}

func TestValidateToolEmitsTerminalTargetSkipsFollowup(t *testing.T) {
	spec := core.MachineSpec{
		Name:           "test",
		States:         []string{"Idle", "Done"},
		TerminalStates: []string{"Done"},
		Signals:        []string{"Seed", "Ignored"},
		Transitions: []core.TransitionSpec{
			{State: "Idle", Signal: "Seed", Next: "Done", Action: "finish"},
		},
	}
	defs := []ToolDef{
		{Name: "finish", Type: "builtin", Init: "finish", Emits: []string{"Ignored"}},
	}

	require.NoError(t, ValidateToolEmits(spec, defs))
}
