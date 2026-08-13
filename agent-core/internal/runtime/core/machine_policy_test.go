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
	require.Empty(t, resumeSignal(nil))
}

func TestValidateMachinePolicyRequiresResumeSignalAndIterationBudget(t *testing.T) {
	t.Parallel()
	err := ValidateRequiredMachinePolicy(MachineSpec{
		Signals: SignalSpecsFromNames(string(AwaitApproval)),
	})
	require.ErrorContains(t, err, "resume_signal")
	require.ErrorContains(t, err, "budget.max_iterations")

	require.NoError(t, ValidateRequiredMachinePolicy(MachineSpec{
		ResumeSignal: "Continue",
		BudgetSpec:   &BudgetSpec{MaxIterations: 10},
		Signals:      SignalSpecsFromNames(string(AwaitApproval), "Continue"),
	}))
}

func TestMachinePolicyDiagnosticsNameRemainingImplicitDefaults(t *testing.T) {
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
		DiagnosticImplicitSummarySignal, DiagnosticImplicitCommandTimeout,
		DiagnosticMissingTerminalStatus,
	} {
		require.True(t, codes[code], code)
	}
	require.False(t, codes[DiagnosticImplicitResumeSignal])
	require.False(t, codes[DiagnosticImplicitMaxIterations])
}

func TestTransitionReportOutputDecoratesAnyCommandResult(t *testing.T) {
	t.Parallel()
	spec := MachineSpec{
		Name: "report", InitialState: "Idle",
		States: StateSpecs{
			{Name: "Idle"}, {Name: "Reporting"}, {Name: "Later"},
			{Name: "Done", RunStatus: StatusSucceeded},
		},
		TerminalStates: []string{"Done"},
		Signals:        SignalSpecsFromNames("Seed", "Published", "LaterDone"),
		Transitions: []TransitionSpec{
			{
				State: "Idle", Signal: "Seed", Next: "Reporting",
				Action: "publish_endpoint", Label: "custom",
				ReportOutput: "$.endpoint", Summary: true,
			},
			{State: "Reporting", Signal: "Published", Next: "Later", Action: "later"},
			{State: "Later", Signal: "LaterDone", Next: "Done"},
		},
	}
	registry := NewRegistry()
	registry.Register(ToolSpec{Name: "publish_endpoint"}, reportBuilder{})
	registry.Register(ToolSpec{Name: "later"}, staticOutputBuilder{
		name: "later", signal: "LaterDone", output: `{"later":true}`,
	})
	var reported *OperatorReport
	params := LoopParams{
		MachineSpec: &spec, Registry: registry, Trace: &loopRecorder{},
		Budget: Budget{MaxIterations: 3},
		Hooks: LoopHooks{OnResult: func(rr RunResult, result Result) RunResult {
			if result.OperatorReport != nil {
				reported = result.OperatorReport
			}
			return rr
		}},
	}
	result, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, result.Status)
	require.JSONEq(t, `{"endpoint":"127.0.0.1:9000"}`, result.Summary)
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

type staticOutputBuilder struct {
	name   string
	signal Signal
	output string
}

func (builder staticOutputBuilder) Build(Result) Command {
	return staticOutputCommand(builder)
}

type staticOutputCommand staticOutputBuilder

func (command staticOutputCommand) Name() string { return command.name }
func (command staticOutputCommand) Execute() Result {
	return Result{
		CommandName: command.name, Signal: command.signal, Output: command.output,
	}
}
func (command staticOutputCommand) Undo(Result) Result {
	return NoopUndo(command.name)
}
