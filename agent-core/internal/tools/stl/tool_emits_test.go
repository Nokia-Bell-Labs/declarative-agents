// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
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

func TestValidateToolEmitsDynamicToolIgnoresInternalMachineActions(t *testing.T) {
	spec := core.MachineSpec{
		Name:           "monitored-grammar",
		States:         core.StateSpecsFromNames("Idle", "StartingMonitor", "Parsing", "Composing", "AwaitingMonitorExit", "StoppingMonitor", "Done", "Failed"),
		TerminalStates: []string{"Done", "Failed"},
		Signals: core.SignalSpecsFromNames(
			"Seed", "ServerLaunched", "ToolDone", "RESTResponded", "RESTDomainFailed",
			"TaskCompleted", "ExitRequested", "AwaitTimedOut", "ServerStopped", "CommandError",
		),
		Transitions: []core.TransitionSpec{
			{State: "Idle", Signal: "Seed", Next: "StartingMonitor", Action: "launch_monitor_rest"},
			{State: "StartingMonitor", Signal: "ServerLaunched", Next: "Composing"},
			{State: "Parsing", Signal: "ToolDone", Next: "Composing", Action: "$tool"},
			{State: "Parsing", Signal: "TaskCompleted", Next: "AwaitingMonitorExit", Action: "await_monitor_control"},
			{State: "Composing", Signal: "RESTResponded", Next: "Composing"},
			{State: "Composing", Signal: "RESTDomainFailed", Next: "Composing"},
			{State: "AwaitingMonitorExit", Signal: "ExitRequested", Next: "StoppingMonitor", Action: "stop_monitor_rest"},
			{State: "AwaitingMonitorExit", Signal: "AwaitTimedOut", Next: "Failed"},
			{State: "AwaitingMonitorExit", Signal: "ServerStopped", Next: "Failed"},
			{State: "StoppingMonitor", Signal: "ServerStopped", Next: "Done"},
			{State: "StartingMonitor", Signal: "CommandError", Next: "Failed"},
			{State: "Composing", Signal: "CommandError", Next: "Failed"},
			{State: "AwaitingMonitorExit", Signal: "CommandError", Next: "Failed"},
			{State: "StoppingMonitor", Signal: "CommandError", Next: "Failed"},
		},
	}
	defs := []ToolDef{
		{Name: "launch_monitor_rest", Type: "builtin", Init: "rest_server_launch", Visibility: "internal", Emits: []string{"ServerLaunched", "CommandError"}},
		{Name: "await_monitor_control", Type: "builtin", Init: "rest_await_event", Visibility: "internal", Emits: []string{"ExitRequested", "AwaitTimedOut", "ServerStopped", "CommandError"}},
		{Name: "stop_monitor_rest", Type: "builtin", Init: "rest_server_stop", Visibility: "internal", Emits: []string{"ServerStopped", "CommandError"}},
		{Name: "ollama_list_models", Type: "builtin", Init: "rest_client_invoke", Emits: []string{"RESTResponded", "RESTDomainFailed", "CommandError"}},
	}

	require.NoError(t, ValidateToolEmits(spec, defs))
}
