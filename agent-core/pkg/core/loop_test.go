// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agentic-loop/agent-core/pkg/tracing"
)

type loopRecorder struct {
	mu     sync.Mutex
	events []string
	spans  []string
}

func (r *loopRecorder) Push(name string, _ ...attribute.KeyValue) (tracing.Tracer, func()) {
	r.mu.Lock()
	r.spans = append(r.spans, name)
	r.mu.Unlock()
	child := &loopRecorder{}
	return child, func() {}
}

func (r *loopRecorder) Event(name string, _ ...attribute.KeyValue) {
	r.mu.Lock()
	r.events = append(r.events, name)
	r.mu.Unlock()
}

func (r *loopRecorder) SetAttributes(_ ...attribute.KeyValue) {}

func (r *loopRecorder) hasEvent(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		if e == name {
			return true
		}
	}
	return false
}

var _ tracing.Tracer = (*loopRecorder)(nil)

type fakeCmd struct {
	name   string
	signal Signal
}

func (f *fakeCmd) Name() string    { return f.name }
func (f *fakeCmd) Execute() Result { return Result{Signal: f.signal, CommandName: f.name} }

type fakeBuilder struct {
	name   string
	signal Signal
}

func (f *fakeBuilder) Build(_ Result) Command { return &fakeCmd{name: f.name, signal: f.signal} }

func simpleLoopParams(tr tracing.Tracer) LoopParams {
	reg := NewRegistry()
	reg.Register(ToolSpec{Name: "step_a", Visibility: Internal}, &fakeBuilder{name: "step_a", signal: Signal("Done")})
	reg.Register(ToolSpec{Name: "step_b", Visibility: Internal}, &fakeBuilder{name: "step_b", signal: Signal("TaskCompleted")})

	bA, _ := reg.Resolve("step_a")
	bB, _ := reg.Resolve("step_b")

	table := TransitionTable{
		{State: "Start", Signal: Seed}: {
			NextState: "Working",
			Action:    func(r Result) Command { return bA.Build(r) },
		},
		{State: "Working", Signal: Signal("Done")}: {
			NextState: "Working",
			Action:    func(r Result) Command { return bB.Build(r) },
		},
		{State: "Working", Signal: Signal("TaskCompleted")}: {
			NextState: "Finished",
		},
		{State: "Working", Signal: BudgetExhausted}: {
			NextState: "OverBudget",
		},
		{State: "Working", Signal: CommandError}: {
			NextState: "Broken",
		},
	}

	terminal := func(s State) bool {
		return s == "Finished" || s == "OverBudget" || s == "Broken"
	}

	return LoopParams{
		InitialState: "Start",
		Prompt:       "test",
		Registry:     reg,
		Table:        table,
		IsTerminal:   terminal,
		Trace:        tr,
		Budget:       Budget{MaxIterations: 100},
		Hooks: LoopHooks{
			TerminalStatus: func(s State) RunStatus {
				switch s {
				case "Finished":
					return StatusSucceeded
				case "OverBudget":
					return StatusBudgetExceeded
				default:
					return StatusFailed
				}
			},
			TaskCompletedSignal: Signal("TaskCompleted"),
		},
	}
}

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

// --- test helpers ---

type tokenFakeCmd struct {
	tokens int
}

func (f *tokenFakeCmd) Name() string { return "token_cmd" }
func (f *tokenFakeCmd) Execute() Result {
	return Result{
		Signal:      Signal("Done"),
		CommandName: "token_cmd",
		Cost:        Cost{TokensIn: f.tokens, TokensOut: f.tokens, Duration: time.Millisecond},
	}
}

type staticBuilder struct {
	cmd Command
}

func (s *staticBuilder) Build(_ Result) Command { return s.cmd }
