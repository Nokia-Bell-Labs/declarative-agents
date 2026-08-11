// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMachinePolicySignalsOverrideLegacyDefaults(t *testing.T) {
	t.Parallel()
	machine := &MachineSpec{
		SummarySignal: "ResponseReady", ResumeSignal: "ResumeRequested",
	}
	params := LoopParams{MachineSpec: machine}
	require.Equal(t, Signal("ResponseReady"), taskCompletedSignal(params))
	require.Equal(t, Signal("ResumeRequested"), resumeSignal(machine))

	params.Hooks.TaskCompletedSignal = "HookSummary"
	require.Equal(t, Signal("HookSummary"), taskCompletedSignal(params))
	require.Equal(t, TaskCompleted, taskCompletedSignal(LoopParams{}))
	require.Equal(t, Approved, resumeSignal(nil))
}

func TestMachinePolicyDiagnosticsNameEveryImplicitDefault(t *testing.T) {
	t.Parallel()
	spec := MachineSpec{
		InitialState: "Idle",
		States: StateSpecs{
			{Name: "Idle"}, {Name: "Done"},
		},
		TerminalStates: []string{"Done"},
		Signals:        SignalSpecsFromNames("Seed"),
		Transitions: []TransitionSpec{{
			State: "Idle", Signal: "Seed", Next: "Done",
		}},
	}
	diagnostics := DiagnoseMachineSpec(spec)
	codes := make(map[string]bool)
	for _, diagnostic := range diagnostics {
		codes[diagnostic.Code] = true
	}
	for _, code := range []string{
		DiagnosticImplicitSummarySignal, DiagnosticImplicitResumeSignal,
		DiagnosticImplicitCommandTimeout, DiagnosticImplicitMaxIterations,
		DiagnosticMissingTerminalStatus,
	} {
		require.True(t, codes[code], code)
	}
}
