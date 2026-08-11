// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
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
