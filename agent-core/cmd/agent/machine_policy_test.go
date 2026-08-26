// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestLoopParamsUsesMachineCommandTimeout(t *testing.T) {
	t.Parallel()
	machine := core.MachineSpec{
		BudgetSpec: &core.BudgetSpec{CommandTimeout: "7s"},
	}
	params := loopParams(runtimeConfig{}, loopParamDeps{
		Machine: machine, State: &agentState{}, Registry: core.NewRegistry(),
	})
	require.Equal(t, 7*time.Second, params.CommandTimeout)

	machine.BudgetSpec = nil
	params = loopParams(runtimeConfig{}, loopParamDeps{
		Machine: machine, State: &agentState{}, Registry: core.NewRegistry(),
	})
	require.Zero(t, params.CommandTimeout)
}

func TestLoopParamsRunBudgetIgnoresRegisteredInvokeLimits(t *testing.T) {
	t.Parallel()
	type invokeLimits struct {
		name      string
		maxTime   int
		maxTokens int
	}
	orders := []struct {
		name   string
		limits []invokeLimits
	}{
		{name: "fast then deep", limits: []invokeLimits{
			{name: "fast", maxTime: 1, maxTokens: 100},
			{name: "deep", maxTime: 600, maxTokens: 9000},
		}},
		{name: "deep then fast", limits: []invokeLimits{
			{name: "deep", maxTime: 600, maxTokens: 9000},
			{name: "fast", maxTime: 1, maxTokens: 100},
		}},
	}
	for _, order := range orders {
		order := order
		t.Run(order.name, func(t *testing.T) {
			t.Parallel()
			state := &agentState{registry: core.NewRegistry(), tracer: tracing.NoopTracer{}}
			builtins := toolregistry.NewBuiltinRegistry()
			registerLLMFactories(state)(builtins)
			factory, ok := builtins.Resolve(toollm.InitInvokeLLM)
			require.True(t, ok)
			for _, limits := range order.limits {
				_, err := factory(catalog.ToolDef{
					Name: limits.name, Type: "builtin", Init: toollm.InitInvokeLLM,
					Config: map[string]interface{}{
						"model": "test-model", "provider_url": "http://127.0.0.1:11434",
						"manifest_state": "Calling", "max_time": limits.maxTime,
						"max_tokens": limits.maxTokens,
					},
				}, nil)
				require.NoError(t, err)
			}

			machine := core.MachineSpec{BudgetSpec: &core.BudgetSpec{
				MaxIterations: 7, MaxTokens: 321, MaxDuration: "2h",
			}}
			params := loopParams(runtimeConfig{}, loopParamDeps{
				Machine: machine, State: state, Registry: state.registry,
			})
			require.Equal(t, core.Budget{
				MaxIterations: 7, MaxTokens: 321, MaxDuration: 2 * time.Hour,
			}, params.Budget)

			unbounded := loopParams(runtimeConfig{}, loopParamDeps{
				Machine: core.MachineSpec{}, State: state, Registry: state.registry,
			})
			require.Zero(t, unbounded.Budget.MaxDuration)
			require.Zero(t, unbounded.Budget.MaxTokens)
		})
	}
}

func TestMachineCommandTimeoutRoutesRecoveryAndContinues(t *testing.T) {
	t.Parallel()
	machine := core.MachineSpec{
		Name: "timeout", InitialState: "Idle",
		States: core.StateSpecs{
			{Name: "Idle"}, {Name: "Slow"}, {Name: "Recovered"},
			{Name: "Done", RunStatus: core.StatusSucceeded},
		},
		TerminalStates: []string{"Done"},
		Signals:        core.SignalSpecsFromNames("Seed", "CommandError", "ToolDone"),
		Transitions: []core.TransitionSpec{
			{State: "Idle", Signal: "Seed", Next: "Slow", Action: "slow"},
			{State: "Slow", Signal: "CommandError", Next: "Recovered", Action: "recover"},
			{State: "Recovered", Signal: "ToolDone", Next: "Done"},
		},
		BudgetSpec: &core.BudgetSpec{MaxIterations: 5, CommandTimeout: "1ms"},
	}
	registry := core.NewRegistry()
	registry.Register(core.ToolSpec{Name: "slow"}, timeoutBuilder{})
	registry.Register(core.ToolSpec{Name: "recover"},
		staticSignalBuilder{name: "recover", signal: core.ToolDone})
	params := loopParams(runtimeConfig{}, loopParamDeps{
		Machine: machine, State: &agentState{}, Registry: registry,
		Tracer: tracing.NoopTracer{},
	})
	result, err := core.Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, core.StatusSucceeded, result.Status)
	require.Len(t, result.Events, 2)
	require.Equal(t, core.CommandError, result.Events[0].Signal)
	require.Equal(t, core.ToolDone, result.Events[1].Signal)
}

func TestOperatorReportIsGenericAndPreservesMonitorLine(t *testing.T) {
	output, err := captureStderr(t, func() error {
		reportOperatorOutput(core.Result{
			CommandName: "differently_named_monitor_word",
			OperatorReport: &core.OperatorReport{
				Label: "monitor", Field: "address", Value: "127.0.0.1:9090",
			},
		})
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, "monitor address: 127.0.0.1:9090\n", output)
}

func TestOperatorReportOmissionPrintsNothing(t *testing.T) {
	output, err := captureStderr(t, func() error {
		reportOperatorOutput(core.Result{CommandName: "silent"})
		return nil
	})
	require.NoError(t, err)
	require.Empty(t, output)
}

func TestDeclaredSummaryIsNotOverwrittenByLaterOutput(t *testing.T) {
	t.Parallel()
	machine := &core.MachineSpec{Transitions: []core.TransitionSpec{{
		State: "A", Signal: "Ready", Next: "B", Action: "answer", Summary: true,
	}}}
	reporter := cliResultReporterForMachine(machine)
	result := reporter(
		core.RunResult{Summary: "declared answer"},
		core.Result{CommandName: "cleanup", Output: "later cleanup output"},
	)
	require.Equal(t, "declared answer", result.Summary)
}

func TestValidateConfigDiagnosticsNameUndeclaredTerminalStatus(t *testing.T) {
	machine := core.MachineSpec{
		InitialState: "Idle",
		States: core.StateSpecs{
			{Name: "Idle"}, {Name: "InsufficientGrounding"},
		},
		TerminalStates: []string{"InsufficientGrounding"},
		Signals:        core.SignalSpecsFromNames("Seed"),
		Transitions: []core.TransitionSpec{{
			State: "Idle", Signal: "Seed", Next: "InsufficientGrounding",
		}},
		BudgetSpec:    &core.BudgetSpec{MaxIterations: 3, CommandTimeout: "1s"},
		SummarySignal: "Seed", ResumeSignal: "Seed",
	}
	output, err := captureStderr(t, func() error {
		reportMachineDiagnostics(machine)
		return nil
	})
	require.NoError(t, err)
	require.Contains(t, output, "machine-diagnostic-undeclared_terminal_status")
	require.Contains(t, output, "InsufficientGrounding")
}

func TestParseRetryDomainSnapshotPreservesRemainingBudget(t *testing.T) {
	t.Parallel()
	origin := &agentState{parseRetries: &toollm.ParseErrorRetryTracker{
		MaxConsecutive: 5,
	}}
	for range 3 {
		require.Equal(t, core.ToolDone, origin.parseRetries.ReportParseError())
	}
	snapshot, err := origin.snapshotDomain()
	require.NoError(t, err)
	resumed := &agentState{parseRetries: &toollm.ParseErrorRetryTracker{
		MaxConsecutive: 5,
	}}
	require.NoError(t, resumed.restoreDomain(snapshot))
	require.Equal(t, core.ToolDone, resumed.parseRetries.ReportParseError())
	require.Equal(t, core.BudgetExhausted, resumed.parseRetries.ReportParseError())
}

type timeoutBuilder struct{}

func (timeoutBuilder) Build(core.Result) core.Command { return timeoutCommand{} }

type timeoutCommand struct{}

func (timeoutCommand) Name() string { return "slow" }
func (timeoutCommand) Execute() core.Result {
	panic("timeout command must use ExecuteContext")
}
func (timeoutCommand) ExecuteContext(ctx context.Context) core.Result {
	<-ctx.Done()
	return core.Result{CommandName: "slow", Signal: core.CommandError, Err: ctx.Err()}
}
func (timeoutCommand) Undo(core.Result) core.Result { return core.NoopUndo("slow") }
