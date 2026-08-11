// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
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

func TestTransitionReportOutputDecoratesAnyCommandResult(t *testing.T) {
	t.Parallel()
	spec := MachineSpec{
		Name: "report", InitialState: "Idle",
		States: StateSpecs{
			{Name: "Idle"}, {Name: "Reporting"},
			{Name: "Done", RunStatus: StatusSucceeded},
		},
		TerminalStates: []string{"Done"},
		Signals:        SignalSpecsFromNames("Seed", "Published"),
		Transitions: []TransitionSpec{
			{
				State: "Idle", Signal: "Seed", Next: "Reporting",
				Action: "publish_endpoint", Label: "custom",
				ReportOutput: "$.endpoint",
			},
			{State: "Reporting", Signal: "Published", Next: "Done"},
		},
	}
	registry := NewRegistry()
	registry.Register(ToolSpec{Name: "publish_endpoint"}, reportBuilder{})
	var reported *OperatorReport
	params := LoopParams{
		MachineSpec: &spec, Registry: registry, Trace: &loopRecorder{},
		Budget: Budget{MaxIterations: 3},
		Hooks: LoopHooks{OnResult: func(rr RunResult, result Result) RunResult {
			reported = result.OperatorReport
			return rr
		}},
	}
	result, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, result.Status)
	require.Equal(t, &OperatorReport{
		Label: "custom", Field: "endpoint", Value: "127.0.0.1:9000",
	}, reported)
}

type reportBuilder struct{}

func (reportBuilder) Build(Result) Command { return reportCommand{} }

type reportCommand struct{}

func (reportCommand) Name() string { return "publish_endpoint" }
func (reportCommand) Execute() Result {
	return Result{
		CommandName: "publish_endpoint", Signal: "Published",
		Output: `{"endpoint":"127.0.0.1:9000"}`,
	}
}
func (reportCommand) Undo(Result) Result { return NoopUndo("publish_endpoint") }
