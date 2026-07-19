// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
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

func (r *loopRecorder) RecordError(_ error) {}

func (r *loopRecorder) Context() context.Context { return context.Background() }

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

func (f *fakeCmd) Name() string         { return f.name }
func (f *fakeCmd) Execute() Result      { return Result{Signal: f.signal, CommandName: f.name} }
func (f *fakeCmd) Undo(_ Result) Result { return NoopUndo(f.name) }

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

func workflowMetricLoopParams(rec monitor.RuntimeRecorder) LoopParams {
	spec := &MachineSpec{
		Name:           "workflow-metrics",
		InitialState:   "Start",
		MetricLabels:   MetricLabels{"use_case": "rel04.0-monitor", "phase": "setup"},
		States:         StateSpecsFromNames("Start", "Working", "Finished"),
		TerminalStates: []string{"Finished"},
		Signals:        SignalSpecsFromNames(string(Seed), string(ToolDone)),
		Transitions: []TransitionSpec{
			{State: "Start", Signal: string(Seed), Next: "Working", Action: "emit_metric", MetricLabels: MetricLabels{"phase": "dispatch"}},
			{State: "Working", Signal: string(ToolDone), Next: "Finished"},
		},
	}
	return LoopParams{
		MachineSpec:     spec,
		AgentName:       "workflow-run",
		Trace:           &loopRecorder{},
		Budget:          Budget{MaxIterations: 10},
		MonitorRecorder: rec,
		InitFunc: func(reg *Registry) error {
			reg.Register(ToolSpec{Name: "emit_metric", Visibility: Internal}, workflowMetricBuilder{})
			return nil
		},
		Hooks: LoopHooks{TerminalStatus: func(State) RunStatus { return StatusSucceeded }},
	}
}

type workflowMetricBuilder struct{}

func (workflowMetricBuilder) Build(Result) Command {
	return &workflowMetricCmd{}
}

type workflowMetricCmd struct {
	rec monitor.ToolMetricsRecorder
}

func (c *workflowMetricCmd) Name() string { return "emit_metric" }
func (c *workflowMetricCmd) Execute() Result {
	_ = c.rec.RecordMetric(context.Background(), monitor.MetricSample{
		Name: "tool.bytes", Kind: monitor.InstrumentHistogram, Unit: "By",
		Value: 7, ToolName: c.Name(),
	})
	return Result{Signal: ToolDone}
}
func (c *workflowMetricCmd) Undo(_ Result) Result { return NoopUndo(c.Name()) }
func (c *workflowMetricCmd) SetMonitorRecorder(rec monitor.ToolMetricsRecorder) {
	c.rec = rec
}

func requireSampleLabels(t *testing.T, samples []monitor.MetricSample, name string, labels map[string]string) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name != name {
			continue
		}
		for label, want := range labels {
			require.Equal(t, want, sample.Attributes[label], "sample %#v", sample)
		}
		return
	}
	t.Fatalf("missing sample %s in %#v", name, samples)
}

func requireSampleEnvelope(t *testing.T, samples []monitor.MetricSample, name string, want monitor.MetricSample) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name != name {
			continue
		}
		require.Equal(t, want.ToolName, sample.ToolName, "sample %#v", sample)
		require.Equal(t, want.RunID, sample.RunID, "sample %#v", sample)
		require.Equal(t, want.State, sample.State, "sample %#v", sample)
		require.Equal(t, want.Signal, sample.Signal, "sample %#v", sample)
		require.Equal(t, want.Status, sample.Status, "sample %#v", sample)
		return
	}
	t.Fatalf("missing sample %s in %#v", name, samples)
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

func TestLoop_DeclarativeInit(t *testing.T) {
	t.Parallel()

	machineYAML := `
name: test-machine
initial_state: Start
states: [Start, Working, Finished, Failed]
terminal_states: [Finished, Failed]
signals: [Seed, Done, TaskCompleted, BudgetExhausted, CommandError]
transitions:
  - state: Start
    signal: Seed
    next: Working
    action: step_a
  - state: Working
    signal: Done
    next: Working
    action: step_b
  - state: Working
    signal: TaskCompleted
    next: Finished
  - state: Working
    signal: BudgetExhausted
    next: Failed
  - state: Working
    signal: CommandError
    next: Failed
`
	dir := t.TempDir()
	machineFile := dir + "/machine.yaml"
	if err := os.WriteFile(machineFile, []byte(machineYAML), 0644); err != nil {
		t.Fatal(err)
	}

	tr := &loopRecorder{}

	params := LoopParams{
		Prompt:      "test",
		MachineFile: machineFile,
		Trace:       tr,
		Budget:      Budget{MaxIterations: 100},
		InitFunc: func(reg *Registry) error {
			reg.Register(ToolSpec{Name: "step_a", Visibility: Internal}, &fakeBuilder{name: "step_a", signal: Signal("Done")})
			reg.Register(ToolSpec{Name: "step_b", Visibility: Internal}, &fakeBuilder{name: "step_b", signal: Signal("TaskCompleted")})
			return nil
		},
		Hooks: LoopHooks{
			TaskCompletedSignal: Signal("TaskCompleted"),
			TerminalStatus: func(s State) RunStatus {
				if s == "Finished" {
					return StatusSucceeded
				}
				return StatusFailed
			},
		},
	}

	rr, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, rr.Status)
	require.Equal(t, State("Finished"), rr.FinalState)
	require.Equal(t, 2, rr.Iterations)
}

func TestLoop_DeclarativeSuspendAction(t *testing.T) {
	t.Parallel()

	machineYAML := `
name: suspend-machine
initial_state: Start
states: [Start, AwaitingApproval, Failed]
terminal_states: [Failed]
signals: [Seed, AwaitApproval]
transitions:
  - state: Start
    signal: Seed
    next: AwaitingApproval
    action: suspend
`
	dir := t.TempDir()
	machineFile := dir + "/machine.yaml"
	require.NoError(t, os.WriteFile(machineFile, []byte(machineYAML), 0o644))

	params := LoopParams{
		Prompt:      "test",
		MachineFile: machineFile,
		Trace:       &loopRecorder{},
		Budget:      Budget{MaxIterations: 10},
		InitFunc: func(reg *Registry) error {
			reg.Register(ToolSpec{Name: "suspend", Visibility: Internal}, &fakeBuilder{name: "suspend", signal: AwaitApproval})
			return nil
		},
	}

	rr, err := Loop(params, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusSuspended, rr.Status)
	require.Equal(t, State("AwaitingApproval"), rr.FinalState)
}

func TestLoop_DeclarativeInit_MissingTool(t *testing.T) {
	t.Parallel()

	machineYAML := `
name: test
initial_state: S
states: [S, F]
terminal_states: [F]
signals: [Seed]
transitions:
  - state: S
    signal: Seed
    next: F
    action: nonexistent
`
	dir := t.TempDir()
	machineFile := dir + "/machine.yaml"
	if err := os.WriteFile(machineFile, []byte(machineYAML), 0644); err != nil {
		t.Fatal(err)
	}

	params := LoopParams{
		Prompt:      "test",
		MachineFile: machineFile,
		Trace:       &loopRecorder{},
		Budget:      Budget{MaxIterations: 10},
	}

	_, err := Loop(params, context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "nonexistent")
}

func TestLoop_DeclarativeInit_UsesPreloadedMachineSpec(t *testing.T) {
	t.Parallel()

	spec := MachineSpec{
		Name:           "test",
		InitialState:   "S",
		States:         StateSpecsFromNames("S", "F"),
		TerminalStates: []string{"F"},
		Signals:        SignalSpecsFromNames("Seed", "Done"),
		Transitions: []TransitionSpec{
			{State: "S", Signal: "Seed", Next: "F", Action: "step"},
		},
	}
	params := LoopParams{
		Prompt:      "test",
		MachineFile: "/definitely/not/read/machine.yaml",
		MachineSpec: &spec,
		Trace:       &loopRecorder{},
		Budget:      Budget{MaxIterations: 10},
		InitFunc: func(reg *Registry) error {
			reg.Register(ToolSpec{Name: "step", Visibility: Internal}, &fakeBuilder{name: "step", signal: Signal("Done")})
			return nil
		},
		Hooks: LoopHooks{
			TerminalStatus: func(s State) RunStatus {
				if s == "F" {
					return StatusSucceeded
				}
				return StatusFailed
			},
		},
	}

	rr, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, State("F"), rr.FinalState)
	require.Equal(t, StatusSucceeded, rr.Status)
}

func TestLoop_DeclarativeInit_MachineNameDoesNotChangeEngineBehavior(t *testing.T) {
	t.Parallel()

	run := func(t *testing.T, machineName string) RunResult {
		t.Helper()
		spec := MachineSpec{
			Name:           machineName,
			InitialState:   "Start",
			States:         StateSpecsFromNames("Start", "Working", "Finished"),
			TerminalStates: []string{"Finished"},
			Signals:        SignalSpecsFromNames("Seed", "Done", "TaskCompleted"),
			Transitions: []TransitionSpec{
				{State: "Start", Signal: "Seed", Next: "Working", Action: "step_a"},
				{State: "Working", Signal: "Done", Next: "Working", Action: "step_b"},
				{State: "Working", Signal: "TaskCompleted", Next: "Finished"},
			},
		}
		params := LoopParams{
			MachineSpec: &spec,
			Trace:       &loopRecorder{},
			Budget:      Budget{MaxIterations: 10},
			InitFunc: func(reg *Registry) error {
				reg.Register(ToolSpec{Name: "step_a", Visibility: Internal}, &fakeBuilder{name: "step_a", signal: Signal("Done")})
				reg.Register(ToolSpec{Name: "step_b", Visibility: Internal}, &fakeBuilder{name: "step_b", signal: Signal("TaskCompleted")})
				return nil
			},
			Hooks: LoopHooks{
				TaskCompletedSignal: Signal("TaskCompleted"),
				TerminalStatus: func(s State) RunStatus {
					if s == "Finished" {
						return StatusSucceeded
					}
					return StatusFailed
				},
			},
		}
		rr, err := Loop(params, context.Background())
		require.NoError(t, err)
		return rr
	}

	generatorRun := run(t, "executor")
	evaluatorRun := run(t, "critic")

	require.Equal(t, generatorRun.Status, evaluatorRun.Status)
	require.Equal(t, generatorRun.FinalState, evaluatorRun.FinalState)
	require.Equal(t, generatorRun.Iterations, evaluatorRun.Iterations)
	require.Equal(t, len(generatorRun.Events), len(evaluatorRun.Events))
	for i := range generatorRun.Events {
		require.Equal(t, generatorRun.Events[i].CommandName, evaluatorRun.Events[i].CommandName)
		require.Equal(t, generatorRun.Events[i].FromState, evaluatorRun.Events[i].FromState)
		require.Equal(t, generatorRun.Events[i].ToState, evaluatorRun.Events[i].ToState)
		require.Equal(t, generatorRun.Events[i].Signal, evaluatorRun.Events[i].Signal)
	}
}

func TestLoop_DeclarativeInit_InitFuncError(t *testing.T) {
	t.Parallel()

	machineYAML := `
name: test
initial_state: S
states: [S, F]
terminal_states: [F]
signals: [Seed]
transitions:
  - state: S
    signal: Seed
    next: F
`
	dir := t.TempDir()
	machineFile := dir + "/machine.yaml"
	if err := os.WriteFile(machineFile, []byte(machineYAML), 0644); err != nil {
		t.Fatal(err)
	}

	params := LoopParams{
		Prompt:      "test",
		MachineFile: machineFile,
		Trace:       &loopRecorder{},
		Budget:      Budget{MaxIterations: 10},
		InitFunc: func(reg *Registry) error {
			return fmt.Errorf("init failed: bad config")
		},
	}

	_, err := Loop(params, context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "init failed: bad config")
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
func (f *tokenFakeCmd) Undo(_ Result) Result { return NoopUndo(f.Name()) }

type staticBuilder struct {
	cmd Command
}

func (s *staticBuilder) Build(_ Result) Command { return s.cmd }

func suspendLoopParams(tr tracing.Tracer, builder Builder) LoopParams {
	reg := NewRegistry()
	reg.Register(ToolSpec{Name: "suspend", Visibility: Internal}, builder)
	b, _ := reg.Resolve("suspend")
	return LoopParams{
		InitialState: "Start",
		Registry:     reg,
		Table: TransitionTable{
			{State: "Start", Signal: Seed}: {
				NextState: "AwaitingApproval",
				Action:    func(r Result) Command { return b.Build(r) },
			},
			{State: "AwaitingApproval", Signal: CommandError}: {
				NextState: "Failed",
			},
		},
		IsTerminal: func(s State) bool { return s == "Failed" },
		Trace:      tr,
		Budget:     Budget{MaxIterations: 10},
	}
}

type errorCmd struct {
	name string
	err  error
}

func (e *errorCmd) Name() string { return e.name }
func (e *errorCmd) Execute() Result {
	return Result{Signal: ToolDone, CommandName: e.name, Err: e.err, Output: e.err.Error()}
}
func (e *errorCmd) Undo(_ Result) Result { return NoopUndo(e.name) }
