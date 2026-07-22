// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"sync"
	"testing"
	"time"
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

func (f *fakeCmd) Name() string { return f.name }

func (f *fakeCmd) Execute() Result { return Result{Signal: f.signal, CommandName: f.name} }

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

type activeCommandBuilder struct{ command Command }

func (b activeCommandBuilder) Build(Result) Command { return b.command }

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
