// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

func TestDispatch_StampsSpanAttributesAndCommandName(t *testing.T) {
	t.Parallel()
	tr := &dispatchTestTracer{}
	cmd := dispatchResultCmd{
		name: "dispatch_ok",
		res:  Result{Signal: ToolDone, Cost: Cost{Duration: time.Millisecond, TokensIn: 3, TokensOut: 5}},
	}

	res := Dispatch(cmd, tr, 0)

	require.Equal(t, ToolDone, res.Signal)
	require.Equal(t, "dispatch_ok", res.CommandName)
	require.Equal(t, "execute_tool dispatch_ok", tr.pushName)
	require.Equal(t, "dispatch_ok", tr.pushAttrs["gen_ai.tool.name"].AsString())
	require.Equal(t, "dispatch_ok", tr.child.attrs["command.name"].AsString())
	require.Equal(t, "ToolDone", tr.child.attrs["command.signal"].AsString())
	require.Equal(t, int64(3), tr.child.attrs["gen_ai.usage.input_tokens"].AsInt64())
	require.Equal(t, int64(5), tr.child.attrs["gen_ai.usage.output_tokens"].AsInt64())
}

func TestDispatch_RecordsCommandErrorsOnSpan(t *testing.T) {
	t.Parallel()
	tr := &dispatchTestTracer{}
	cmd := dispatchResultCmd{name: "dispatch_err", res: Result{Signal: ToolDone, Err: fmt.Errorf("boom")}}

	res := Dispatch(cmd, tr, 0)

	require.Equal(t, CommandError, res.Signal)
	require.Equal(t, "dispatch_err", res.CommandName)
	require.Len(t, tr.child.errors, 1)
	require.Equal(t, "dispatch_err", tr.child.attrs["command.name"].AsString())
	require.NotEmpty(t, tr.child.attrs["error.type"].AsString())
}

func TestSafeExecute_RecoversPanicAndForcesCommandError(t *testing.T) {
	t.Parallel()

	res := SafeExecute(dispatchPanicCmd{name: "panic_cmd"}, 0)

	require.Equal(t, CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Err.Error(), "panic in panic_cmd")
	require.Equal(t, "panic: boom", res.Output)
	require.Greater(t, res.Cost.Duration, time.Duration(0))
}

func TestSafeExecute_TimeoutAndIndefiniteWait(t *testing.T) {
	t.Parallel()

	timeout := SafeExecute(dispatchSleepCmd{name: "slow_cmd", sleep: 30 * time.Millisecond}, time.Millisecond)
	require.Equal(t, CommandError, timeout.Signal)
	require.ErrorContains(t, timeout.Err, "timeout executing slow_cmd")
	require.Equal(t, "timeout: slow_cmd", timeout.Output)
	require.Greater(t, timeout.Cost.Duration, time.Duration(0))

	wait := SafeExecute(dispatchSleepCmd{name: "wait_cmd", sleep: time.Millisecond}, 0)
	require.Equal(t, ToolDone, wait.Signal)
	require.NoError(t, wait.Err)
	require.Equal(t, "slept", wait.Output)
	require.Greater(t, wait.Cost.Duration, time.Duration(0))
}

func TestSafeExecute_CompletionTimeoutRaceHasSingleResultOwner(t *testing.T) {
	t.Parallel()
	for range 500 {
		result := SafeExecute(dispatchResultCmd{name: "racy", res: Result{Signal: ToolDone}}, time.Nanosecond)
		if result.Signal == CommandError {
			require.ErrorContains(t, result.Err, "timeout executing racy")
			continue
		}
		require.Equal(t, ToolDone, result.Signal)
		require.NoError(t, result.Err)
	}
}

func TestSafeExecute_LegacyWorkerCanFinishAfterTimeout(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	cmd := &dispatchBlockingCmd{started: started, release: release, finished: finished}

	result := SafeExecute(cmd, time.Millisecond)
	require.Equal(t, CommandError, result.Signal)
	require.ErrorContains(t, result.Err, "timeout executing blocking")
	<-started
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("legacy worker remained blocked after command returned")
	}
}

func TestFillDurationAndForceErrorSignal(t *testing.T) {
	t.Parallel()
	start := time.Now().Add(-time.Millisecond)
	res := Result{Signal: ToolDone, Err: fmt.Errorf("failed")}

	FillDuration(&res, start)
	ForceErrorSignal(&res)

	require.Greater(t, res.Cost.Duration, time.Duration(0))
	require.Equal(t, CommandError, res.Signal)

	kept := Result{Cost: Cost{Duration: 10 * time.Millisecond}}
	FillDuration(&kept, start)
	require.Equal(t, 10*time.Millisecond, kept.Cost.Duration)
}

type dispatchBlockingCmd struct {
	started  chan struct{}
	release  chan struct{}
	finished chan struct{}
}

func (c *dispatchBlockingCmd) Name() string { return "blocking" }
func (c *dispatchBlockingCmd) Execute() Result {
	close(c.started)
	<-c.release
	close(c.finished)
	return Result{Signal: ToolDone}
}
func (c *dispatchBlockingCmd) Undo(Result) Result { return NoopUndo(c.Name()) }

func TestDispatchWithMonitor_EmitsDispatchMetrics(t *testing.T) {
	t.Parallel()
	rec := &dispatchRuntimeRecorder{}
	ctx := monitor.DispatchContext{
		RunID:        "run-1",
		AgentName:    "agent-a",
		State:        "Working",
		MetricLabels: map[string]string{"phase": "dispatch"},
	}

	res := dispatchWithMonitor(
		dispatchResultCmd{name: "metric_cmd", res: Result{Signal: ToolDone}},
		&dispatchTestTracer{},
		0,
		rec,
		ctx,
	)

	require.Equal(t, ToolDone, res.Signal)
	requireDispatchSample(t, rec.samples, "dispatch_duration", monitor.MetricSample{
		Kind: monitor.InstrumentHistogram, Unit: "ms", Value: float64(res.Cost.Duration.Milliseconds()), ToolName: "metric_cmd",
		RunID: "run-1", State: "Working", Signal: "ToolDone", Status: "success",
		Attributes: map[string]string{"agent.name": "agent-a", "phase": "dispatch"},
	})
	requireDispatchSample(t, rec.samples, "dispatch_count", monitor.MetricSample{
		Kind: monitor.InstrumentCounter, Unit: "{dispatch}", Value: 1,
		ToolName: "metric_cmd", RunID: "run-1", State: "Working", Signal: "ToolDone", Status: "success",
	})
	requireDispatchSample(t, rec.samples, "dispatch_success", monitor.MetricSample{
		Kind: monitor.InstrumentCounter, Unit: "{dispatch}", Value: 1,
		ToolName: "metric_cmd", RunID: "run-1", State: "Working", Signal: "ToolDone", Status: "success",
	})
}

func TestDispatchWithMonitor_EnvelopesToolMetricSamples(t *testing.T) {
	t.Parallel()
	rec := &dispatchRuntimeRecorder{}
	ctx := monitor.DispatchContext{
		RunID:        "run-2",
		State:        "Running",
		MetricLabels: map[string]string{"workflow": "rel00"},
	}

	res := dispatchWithMonitor(&dispatchMetricCmd{name: "tool_metric"}, &dispatchTestTracer{}, 0, rec, ctx)

	require.Equal(t, ToolDone, res.Signal)
	requireDispatchSample(t, rec.samples, "tool.bytes", monitor.MetricSample{
		Kind: monitor.InstrumentHistogram, Unit: "By", Value: 7,
		ToolName: "tool_metric", RunID: "run-2", State: "Running",
		Signal: "ToolDone", Status: "success", Attributes: map[string]string{"workflow": "rel00"},
	})
}

func requireDispatchSample(t *testing.T, samples []monitor.MetricSample, name string, want monitor.MetricSample) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name != name {
			continue
		}
		require.Equal(t, want.Kind, sample.Kind)
		require.Equal(t, want.Unit, sample.Unit)
		require.Equal(t, want.Value, sample.Value)
		require.Equal(t, want.ToolName, sample.ToolName)
		require.Equal(t, want.RunID, sample.RunID)
		require.Equal(t, want.State, sample.State)
		require.Equal(t, want.Signal, sample.Signal)
		require.Equal(t, want.Status, sample.Status)
		for key, value := range want.Attributes {
			require.Equal(t, value, sample.Attributes[key], "sample %#v", sample)
		}
		return
	}
	t.Fatalf("missing sample %s in %#v", name, samples)
}

func TestDispatch_MetricExportFailurePreservesCommandResult(t *testing.T) {
	t.Parallel()
	exportErr := fmt.Errorf("metric collector unavailable")
	recorder := &dispatchRuntimeRecorder{err: exportErr}
	want := Result{
		CommandName: "build", Signal: ToolDone, Output: `{"built":true}`,
		Cost: Cost{Duration: 17 * time.Millisecond, TokensIn: 2, TokensOut: 3, Dollars: 0.04},
	}
	got := dispatchWithMonitor(
		dispatchResultCmd{name: "build", res: want},
		&dispatchTestTracer{},
		0,
		recorder,
		monitor.DispatchContext{RunID: "run-1", State: "Working", Iteration: 2},
	)

	require.Equal(t, want, got)
	require.NotEmpty(t, recorder.samples)
	require.Equal(t, "ToolDone", recorder.samples[0].Signal)
	require.Equal(t, "success", recorder.samples[0].Status)
}

type dispatchResultCmd struct {
	name string
	res  Result
}

func (c dispatchResultCmd) Name() string         { return c.name }
func (c dispatchResultCmd) Execute() Result      { return c.res }
func (c dispatchResultCmd) Undo(_ Result) Result { return NoopUndo(c.name) }

type dispatchPanicCmd struct {
	name string
}

func (c dispatchPanicCmd) Name() string         { return c.name }
func (c dispatchPanicCmd) Execute() Result      { panic("boom") }
func (c dispatchPanicCmd) Undo(_ Result) Result { return NoopUndo(c.name) }

type dispatchSleepCmd struct {
	name  string
	sleep time.Duration
}

func (c dispatchSleepCmd) Name() string { return c.name }
func (c dispatchSleepCmd) Execute() Result {
	time.Sleep(c.sleep)
	return Result{Signal: ToolDone, Output: "slept"}
}
func (c dispatchSleepCmd) Undo(_ Result) Result { return NoopUndo(c.name) }

type dispatchMetricCmd struct {
	name string
	rec  monitor.ToolMetricsRecorder
}

func (c *dispatchMetricCmd) Name() string { return c.name }
func (c *dispatchMetricCmd) Execute() Result {
	_ = c.rec.RecordMetric(context.Background(), monitor.MetricSample{
		Name: "tool.bytes", Kind: monitor.InstrumentHistogram, Unit: "By", Value: 7,
	})
	return Result{Signal: ToolDone}
}
func (c *dispatchMetricCmd) Undo(_ Result) Result { return NoopUndo(c.name) }
func (c *dispatchMetricCmd) SetMonitorRecorder(rec monitor.ToolMetricsRecorder) {
	c.rec = rec
}

type dispatchTestTracer struct {
	pushName  string
	pushAttrs map[string]attribute.Value
	attrs     map[string]attribute.Value
	errors    []error
	child     *dispatchTestTracer
}

func (r *dispatchTestTracer) Push(name string, attrs ...attribute.KeyValue) (tracing.Tracer, func()) {
	r.pushName = name
	r.pushAttrs = dispatchAttrs(attrs)
	r.child = &dispatchTestTracer{}
	return r.child, func() {}
}

func (r *dispatchTestTracer) Event(string, ...attribute.KeyValue) {}
func (r *dispatchTestTracer) SetAttributes(attrs ...attribute.KeyValue) {
	if r.attrs == nil {
		r.attrs = make(map[string]attribute.Value, len(attrs))
	}
	for _, attr := range attrs {
		r.attrs[string(attr.Key)] = attr.Value
	}
}
func (r *dispatchTestTracer) RecordError(err error) {
	r.errors = append(r.errors, err)
}
func (r *dispatchTestTracer) Context() context.Context { return context.Background() }

func dispatchAttrs(attrs []attribute.KeyValue) map[string]attribute.Value {
	out := make(map[string]attribute.Value, len(attrs))
	for _, attr := range attrs {
		out[string(attr.Key)] = attr.Value
	}
	return out
}

type dispatchRuntimeRecorder struct {
	samples []monitor.MetricSample
	err     error
}

func (r *dispatchRuntimeRecorder) RecordMetric(_ context.Context, sample monitor.MetricSample) error {
	r.samples = append(r.samples, sample)
	return r.err
}
func (r *dispatchRuntimeRecorder) RecordEvent(context.Context, monitor.RunEvent) error { return nil }
func (r *dispatchRuntimeRecorder) RecordRun(context.Context, monitor.RunSnapshot) error {
	return nil
}

var _ MonitorRecorderAware = (*dispatchMetricCmd)(nil)
var _ monitor.RuntimeRecorder = (*dispatchRuntimeRecorder)(nil)
var _ tracing.Tracer = (*dispatchTestTracer)(nil)
