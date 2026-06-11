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
		States:         core.StateSpecsFromNames("Idle", "Running", "Done", "Failed"),
		TerminalStates: []string{"Done", "Failed"},
		Signals:        core.SignalSpecsFromNames("Seed", "Ready", "CommandError"),
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
		States:         core.StateSpecsFromNames("Idle", "Running", "Done"),
		TerminalStates: []string{"Done"},
		Signals:        core.SignalSpecsFromNames("Seed", "Ready"),
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
		States:         core.StateSpecsFromNames("Idle", "Running", "Done"),
		TerminalStates: []string{"Done"},
		Signals:        core.SignalSpecsFromNames("Seed", "Ready", "CommandError"),
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
		States:         core.StateSpecsFromNames("Idle", "Done"),
		TerminalStates: []string{"Done"},
		Signals:        core.SignalSpecsFromNames("Seed", "Ignored"),
		Transitions: []core.TransitionSpec{
			{State: "Idle", Signal: "Seed", Next: "Done", Action: "finish"},
		},
	}
	defs := []ToolDef{
		{Name: "finish", Type: "builtin", Init: "finish", Emits: []string{"Ignored"}},
	}

	require.NoError(t, ValidateToolEmits(spec, defs))
}

func TestValidateToolEmitsDynamicToolMissingFollowupTransition(t *testing.T) {
	spec := core.MachineSpec{
		Name:           "grammar",
		States:         core.StateSpecsFromNames("Parsing", "Composing", "Done"),
		TerminalStates: []string{"Done"},
		Signals:        core.SignalSpecsFromNames("ToolReady", "ToolDone", "ToolFailed", "InternalOnly"),
		Transitions: []core.TransitionSpec{
			{State: "Parsing", Signal: "ToolReady", Next: "Composing", Action: "$tool"},
			{State: "Composing", Signal: "ToolDone", Next: "Done"},
		},
	}
	defs := []ToolDef{
		{Name: "write", Type: "builtin", Init: "file_write", Emits: []string{"ToolDone", "ToolFailed"}},
		{Name: "parse_response", Type: "builtin", Init: "parse_response", Visibility: "internal", Emits: []string{"InternalOnly"}},
	}

	err := ValidateToolEmits(spec, defs)

	require.Error(t, err)
	require.Contains(t, err.Error(), `dynamic $tool may dispatch tool "write" which emits "ToolFailed"`)
	require.Contains(t, err.Error(), "has no transition for Composing/ToolFailed")
	require.NotContains(t, err.Error(), "InternalOnly")
}

func TestValidateToolEmitsDynamicToolHandlesExternalVocabulary(t *testing.T) {
	spec := core.MachineSpec{
		Name:           "grammar",
		States:         core.StateSpecsFromNames("Parsing", "Composing", "Done", "Failed"),
		TerminalStates: []string{"Done", "Failed"},
		Signals:        core.SignalSpecsFromNames("ToolReady", "ToolDone", "ToolFailed"),
		Transitions: []core.TransitionSpec{
			{State: "Parsing", Signal: "ToolReady", Next: "Composing", Action: "$tool"},
			{State: "Composing", Signal: "ToolDone", Next: "Done"},
			{State: "Composing", Signal: "ToolFailed", Next: "Failed"},
		},
	}
	defs := []ToolDef{
		{Name: "read", Type: "builtin", Init: "file_read", Emits: []string{"ToolDone", "ToolFailed"}},
		{Name: "write", Type: "builtin", Init: "file_write", Emits: []string{"ToolDone", "ToolFailed"}},
	}

	require.NoError(t, ValidateToolEmits(spec, defs))
}
