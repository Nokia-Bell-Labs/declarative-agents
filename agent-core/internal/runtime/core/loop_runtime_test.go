// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestLoop_RunsToCompletion(t *testing.T) {
	t.Parallel()
	tr := &loopRecorder{}
	params := simpleLoopParams(tr)

	rr, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, rr.Status)
	require.Equal(t, State("Finished"), rr.FinalState)
	require.Equal(t, 2, rr.Iterations)
}

func TestLoopMonitorSamplesIncludeWorkflowMetricLabels(t *testing.T) {
	t.Parallel()
	store := monitor.NewStore(monitor.Limits{Samples: 10})
	params := workflowMetricLoopParams(monitor.NewRecorder(store, nil))

	rr, err := Loop(params, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, rr.Status)
	snapshot := store.Snapshot()
	requireSampleLabels(t, snapshot.RecentSamples, "dispatch_duration", map[string]string{
		"use_case": "rel04.0-monitor",
		"phase":    "dispatch",
	})
	requireSampleEnvelope(t, snapshot.RecentSamples, "dispatch_duration", monitor.MetricSample{
		ToolName: "emit_metric",
		RunID:    "workflow-run",
		State:    "Working",
		Signal:   string(ToolDone),
		Status:   "success",
	})
	requireSampleLabels(t, snapshot.RecentSamples, "tool.bytes", map[string]string{
		"use_case": "rel04.0-monitor",
		"phase":    "dispatch",
	})
	requireSampleEnvelope(t, snapshot.RecentSamples, "tool.bytes", monitor.MetricSample{
		ToolName: "emit_metric",
		RunID:    "workflow-run",
		State:    "Working",
		Signal:   string(ToolDone),
		Status:   "success",
	})
}

func TestLoop_SuspendWithoutPersistenceIsExplicitNoop(t *testing.T) {
	t.Parallel()
	params := suspendLoopParams(&loopRecorder{}, &fakeBuilder{name: "suspend", signal: AwaitApproval})

	rr, err := Loop(params, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusSuspended, rr.Status)
	require.Equal(t, State("AwaitingApproval"), rr.FinalState)
	require.NoError(t, rr.LastError)
}

func TestLoop_SuspendCommandErrorFailsWhenToolRequiresCheckpoint(t *testing.T) {
	t.Parallel()
	params := suspendLoopParams(&loopRecorder{}, &staticBuilder{cmd: &errorCmd{name: "suspend", err: fmt.Errorf("suspend requires a persistent checkpoint backend")}})

	rr, err := Loop(params, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusFailed, rr.Status)
	require.Equal(t, State("Failed"), rr.FinalState)
	require.ErrorContains(t, rr.LastError, "suspend requires a persistent checkpoint backend")
}

func TestRunResultJSONOmitsHistoryWhenDisabled(t *testing.T) {
	t.Parallel()
	params := simpleLoopParams(&loopRecorder{})

	rr, err := Loop(params, context.Background())
	require.NoError(t, err)

	data, err := json.Marshal(rr)
	require.NoError(t, err)
	require.NotContains(t, string(data), `"history"`)
}

func TestLoop_BudgetExhausted(t *testing.T) {
	t.Parallel()
	tr := &loopRecorder{}
	params := simpleLoopParams(tr)
	params.Budget.MaxIterations = 1

	rr, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusBudgetExceeded, rr.Status)
	require.Equal(t, State("OverBudget"), rr.FinalState)
}

func TestLoop_ContextCancellation(t *testing.T) {
	t.Parallel()
	tr := &loopRecorder{}
	params := simpleLoopParams(tr)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rr, err := Loop(params, ctx)
	require.NoError(t, err)
	require.Equal(t, StatusCancelled, rr.Status)
}

func TestLoop_ActiveDispatchCancellation(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	finished := make(chan struct{})
	command := &dispatchContextBlockingCmd{started: started, finished: finished}
	registry := NewRegistry()
	registry.Register(ToolSpec{Name: command.Name(), Visibility: Internal}, activeCommandBuilder{command})
	params := LoopParams{
		InitialState: "Start",
		Registry:     registry,
		Table: TransitionTable{
			{State: "Start", Signal: Seed}: {
				NextState: "Working",
				Action:    func(Result) Command { return command },
			},
		},
		IsTerminal: func(State) bool { return false },
		Trace:      &loopRecorder{},
		Budget:     Budget{MaxIterations: 10},
	}
	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan RunResult, 1)
	go func() {
		result, _ := Loop(params, ctx)
		results <- result
	}()
	<-started
	cancel()

	select {
	case result := <-results:
		require.Equal(t, StatusCancelled, result.Status)
		require.Equal(t, State("Working"), result.FinalState)
		require.Len(t, result.Events, 1)
		require.Equal(t, CommandError, result.Events[0].Signal)
	case <-time.After(time.Second):
		t.Fatal("Loop remained blocked after active command cancellation")
	}
	select {
	case <-finished:
	default:
		t.Fatal("active context command was not joined before Loop returned")
	}
}

func TestLoop_ValidateParamsHook(t *testing.T) {
	t.Parallel()
	tr := &loopRecorder{}
	params := simpleLoopParams(tr)
	params.Hooks.ValidateParams = ValidateBuilders("nonexistent")

	_, err := Loop(params, context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "nonexistent")
}

func TestLoop_ValidateBuildersHelper(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	reg.Register(ToolSpec{Name: "a"}, &fakeBuilder{name: "a"})

	validate := ValidateBuilders("a")
	require.NoError(t, validate(reg))

	validate2 := ValidateBuilders("a", "b")
	require.Error(t, validate2(reg))
}

func TestLoop_OnResultHook(t *testing.T) {
	t.Parallel()
	tr := &loopRecorder{}
	params := simpleLoopParams(tr)

	callCount := 0
	params.Hooks.OnResult = func(rr RunResult, res Result) RunResult {
		callCount++
		return rr
	}

	_, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, callCount)
}

func TestLoop_CustomBudgetHook(t *testing.T) {
	t.Parallel()
	tr := &loopRecorder{}
	params := simpleLoopParams(tr)
	params.Budget.MaxIterations = 100

	params.Hooks.BudgetExceeded = func(b Budget, rr RunResult, iterations int) bool {
		return iterations >= 1
	}

	rr, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusBudgetExceeded, rr.Status)
}

func TestLoop_AccumulatesTokens(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	tokenCmd := &tokenFakeCmd{tokens: 50}
	reg.Register(ToolSpec{Name: "token_cmd", Visibility: Internal}, &staticBuilder{cmd: tokenCmd})
	reg.Register(ToolSpec{Name: "done_cmd", Visibility: Internal}, &fakeBuilder{name: "done_cmd", signal: Signal("TaskCompleted")})

	bToken, _ := reg.Resolve("token_cmd")
	bDone, _ := reg.Resolve("done_cmd")

	table := TransitionTable{
		{State: "S", Signal: Seed}: {
			NextState: "W",
			Action:    func(r Result) Command { return bToken.Build(r) },
		},
		{State: "W", Signal: Signal("Done")}: {
			NextState: "W",
			Action:    func(r Result) Command { return bDone.Build(r) },
		},
		{State: "W", Signal: Signal("TaskCompleted")}: {
			NextState: "F",
		},
	}

	params := LoopParams{
		InitialState: "S",
		Prompt:       "test",
		Registry:     reg,
		Table:        table,
		IsTerminal:   func(s State) bool { return s == "F" },
		Trace:        &loopRecorder{},
		Budget:       Budget{MaxIterations: 100},
		Hooks: LoopHooks{
			TaskCompletedSignal: Signal("TaskCompleted"),
		},
	}

	rr, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, 50, rr.TokensIn)
	require.Equal(t, 50, rr.TokensOut)
}

func TestLoop_EmitsRegistryFrozenEvent(t *testing.T) {
	t.Parallel()
	tr := &loopRecorder{}
	params := simpleLoopParams(tr)

	_, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.True(t, tr.hasEvent("init.registry_frozen"))
}

func TestLoop_DurationTracked(t *testing.T) {
	t.Parallel()
	tr := &loopRecorder{}
	params := simpleLoopParams(tr)

	rr, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.True(t, rr.Duration > 0, "duration should be positive")
}

func TestLoop_DefaultTerminalStatus(t *testing.T) {
	t.Parallel()
	require.Equal(t, StatusSucceeded, defaultTerminalStatus(State("Succeeded")))
	require.Equal(t, StatusBudgetExceeded, defaultTerminalStatus(State("BudgetExceeded")))
	require.Equal(t, StatusFailed, defaultTerminalStatus(State("Anything")))
}

func TestCoreBudgetExceeded(t *testing.T) {
	t.Parallel()

	require.False(t, coreBudgetExceeded(Budget{}, RunResult{}, 999))

	require.True(t, coreBudgetExceeded(Budget{MaxIterations: 5}, RunResult{}, 5))
	require.False(t, coreBudgetExceeded(Budget{MaxIterations: 5}, RunResult{}, 4))

	require.True(t, coreBudgetExceeded(Budget{MaxTokens: 100}, RunResult{TokensIn: 50, TokensOut: 50}, 0))
	require.False(t, coreBudgetExceeded(Budget{MaxTokens: 100}, RunResult{TokensIn: 50, TokensOut: 49}, 0))
}

func TestLoop_TokenBudgetExhausted(t *testing.T) {
	t.Parallel()
	tr := &loopRecorder{}

	reg := NewRegistry()
	tokenCmd := &tokenFakeCmd{tokens: 50}
	reg.Register(ToolSpec{Name: "step_a", Visibility: Internal}, &staticBuilder{cmd: tokenCmd})
	reg.Register(ToolSpec{Name: "step_b", Visibility: Internal}, &fakeBuilder{name: "step_b", signal: Signal("TaskCompleted")})

	bA, _ := reg.Resolve("step_a")
	bB, _ := reg.Resolve("step_b")

	table := TransitionTable{
		{State: "S", Signal: Seed}: {
			NextState: "W",
			Action:    func(r Result) Command { return bA.Build(r) },
		},
		{State: "W", Signal: Signal("Done")}: {
			NextState: "W",
			Action:    func(r Result) Command { return bB.Build(r) },
		},
		{State: "W", Signal: Signal("TaskCompleted")}: {
			NextState: "F",
		},
		{State: "W", Signal: BudgetExhausted}: {
			NextState: "OB",
		},
	}

	params := LoopParams{
		InitialState: "S",
		Prompt:       "test",
		Registry:     reg,
		Table:        table,
		IsTerminal:   func(s State) bool { return s == "F" || s == "OB" },
		Trace:        tr,
		Budget:       Budget{MaxTokens: 1},
		Hooks: LoopHooks{
			TaskCompletedSignal: Signal("TaskCompleted"),
			TerminalStatus: func(s State) RunStatus {
				if s == "OB" {
					return StatusBudgetExceeded
				}
				return StatusSucceeded
			},
		},
	}

	rr, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusBudgetExceeded, rr.Status)
}
