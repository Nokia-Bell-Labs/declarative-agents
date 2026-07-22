// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry/genai"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

// Dispatch wraps Command.Execute with tracing, timeout, panic
// recovery, duration measurement, and CommandName assignment.
func Dispatch(cmd Command, tr tracing.Tracer, timeout time.Duration) Result {
	return dispatchWithMonitor(cmd, tr, timeout, nil, monitor.DispatchContext{})
}

func dispatchWithMonitor(
	cmd Command,
	tr tracing.Tracer,
	timeout time.Duration,
	rec monitor.RuntimeRecorder,
	dispatchCtx monitor.DispatchContext,
) Result {
	spanName := genai.ToolSpanName(cmd.Name())
	var spanAttrs []attribute.KeyValue
	spanAttrs = append(spanAttrs, genai.ToolAttrs(cmd.Name(), genai.ToolTypeFunction)...)

	if so, ok := cmd.(SpanOverride); ok {
		spanName = so.SpanName()
		spanAttrs = so.SpanCreationAttrs()
	}

	child, done := tr.Push(spanName, spanAttrs...)
	defer done()

	var toolMetrics *dispatchMetricRecorder
	if aware, ok := cmd.(MonitorRecorderAware); ok && rec != nil {
		toolMetrics = &dispatchMetricRecorder{
			rec: rec, dc: dispatchCtx, toolName: cmd.Name(),
		}
		aware.SetMonitorRecorder(toolMetrics)
	}
	if aware, ok := cmd.(TraceContextAware); ok {
		aware.SetTraceContext(oteltrace.SpanContextFromContext(child.Context()))
	}
	res := SafeExecute(cmd, timeout)

	res.CommandName = cmd.Name()
	if toolMetrics != nil {
		toolMetrics.Flush(child.Context(), res)
	}
	stampSpan(child, cmd.Name(), res)
	recordDispatchMetrics(child.Context(), rec, dispatchCtx, res)
	return res
}

// SafeExecute runs a command with panic recovery and optional timeout. Legacy
// commands that outlive a timeout detach, but their single buffered result send
// can always complete without racing or blocking on the returned timeout value.
func SafeExecute(cmd Command, timeout time.Duration) Result {
	if contextual, ok := cmd.(ContextCommand); ok {
		return safeExecuteContext(cmd, contextual, timeout)
	}
	return safeExecuteLegacy(cmd, timeout)
}

func safeExecuteLegacy(cmd Command, timeout time.Duration) Result {
	results := make(chan Result, 1)
	start := time.Now()

	go func() {
		results <- executeSafely(cmd, cmd.Execute)
	}()

	if timeout <= 0 {
		res := <-results
		FillDuration(&res, start)
		ForceErrorSignal(&res)
		return res
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case res := <-results:
		FillDuration(&res, start)
		ForceErrorSignal(&res)
		return res
	case <-timer.C:
		return Result{
			Signal: CommandError,
			Err:    fmt.Errorf("timeout executing %s after %s", cmd.Name(), timeout),
			Output: fmt.Sprintf("timeout: %s", cmd.Name()),
			Cost:   Cost{Duration: time.Since(start)},
		}
	}
}

func safeExecuteContext(cmd Command, contextual ContextCommand, timeout time.Duration) Result {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan Result, 1)
	start := time.Now()
	go func() {
		results <- executeSafely(cmd, func() Result { return contextual.ExecuteContext(ctx) })
	}()
	if timeout <= 0 {
		res := <-results
		FillDuration(&res, start)
		ForceErrorSignal(&res)
		return res
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case res := <-results:
		FillDuration(&res, start)
		ForceErrorSignal(&res)
		return res
	case <-timer.C:
		cancel()
		<-results
		return Result{
			Signal: CommandError,
			Err:    fmt.Errorf("timeout executing %s after %s", cmd.Name(), timeout),
			Output: fmt.Sprintf("timeout: %s", cmd.Name()),
			Cost:   Cost{Duration: time.Since(start)},
		}
	}
}

func executeSafely(cmd Command, execute func() Result) (result Result) {
	defer func() {
		if r := recover(); r != nil {
			result = Result{
				Signal: CommandError,
				Err:    fmt.Errorf("panic in %s: %v", cmd.Name(), r),
				Output: fmt.Sprintf("panic: %v", r),
			}
		}
	}()
	return execute()
}

func recordDispatchMetrics(ctx context.Context, rec monitor.RuntimeRecorder, dc monitor.DispatchContext, res Result) {
	if rec == nil {
		return
	}
	base := dispatchSample(dc, res)
	_ = rec.RecordMetric(ctx, base)
	count := base
	count.Name = "dispatch_count"
	count.Kind = monitor.InstrumentCounter
	count.Unit = "{dispatch}"
	count.Value = 1
	_ = rec.RecordMetric(ctx, count)
	_ = rec.RecordMetric(ctx, dispatchOutcomeSample(count, res))
}

func dispatchSample(dc monitor.DispatchContext, res Result) monitor.MetricSample {
	return monitor.MetricSample{
		Name:       "dispatch_duration",
		Kind:       monitor.InstrumentHistogram,
		Unit:       "ms",
		Value:      float64(res.Cost.Duration.Milliseconds()),
		ToolName:   res.CommandName,
		RunID:      dc.RunID,
		State:      dc.State,
		Signal:     string(res.Signal),
		Status:     dispatchStatus(res),
		Attributes: dispatchAttributes(dc),
	}
}

func dispatchAttributes(dc monitor.DispatchContext) map[string]string {
	attrs := cloneMetricLabels(dc.MetricLabels)
	attrs["agent.name"] = dc.AgentName
	return attrs
}

func dispatchOutcomeSample(base monitor.MetricSample, res Result) monitor.MetricSample {
	if dispatchStatus(res) == "success" {
		base.Name = "dispatch_success"
		return base
	}
	base.Name = "dispatch_failure"
	return base
}

func dispatchStatus(res Result) string {
	if res.Err != nil || res.Signal == CommandError || res.Signal == ToolFailed {
		return "failure"
	}
	return "success"
}

type dispatchMetricRecorder struct {
	rec      monitor.RuntimeRecorder
	dc       monitor.DispatchContext
	toolName string
	mu       sync.Mutex
	samples  []monitor.MetricSample
}

func (r *dispatchMetricRecorder) RecordMetric(_ context.Context, sample monitor.MetricSample) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.samples = append(r.samples, sample)
	return nil
}

func (r *dispatchMetricRecorder) RecordDiagnostic(ctx context.Context, diagnostic monitor.Diagnostic) error {
	if diag, ok := r.rec.(monitor.DiagnosticRecorder); ok {
		return diag.RecordDiagnostic(ctx, diagnostic)
	}
	return nil
}

func (r *dispatchMetricRecorder) Flush(ctx context.Context, res Result) {
	r.mu.Lock()
	samples := append([]monitor.MetricSample(nil), r.samples...)
	r.samples = nil
	r.mu.Unlock()
	for _, sample := range samples {
		_ = r.rec.RecordMetric(ctx, r.envelope(sample, res))
	}
}

func (r *dispatchMetricRecorder) envelope(sample monitor.MetricSample, res Result) monitor.MetricSample {
	if sample.ToolName == "" {
		sample.ToolName = r.toolName
	}
	if sample.RunID == "" {
		sample.RunID = r.dc.RunID
	}
	if sample.State == "" {
		sample.State = r.dc.State
	}
	if sample.Signal == "" {
		sample.Signal = string(res.Signal)
	}
	if sample.Status == "" {
		sample.Status = dispatchStatus(res)
	}
	sample.Attributes = mergeMetricAttributes(sample.Attributes, r.dc.MetricLabels)
	return sample
}

func mergeMetricAttributes(base map[string]string, labels map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(labels))
	for name, value := range base {
		out[name] = value
	}
	for name, value := range labels {
		out[name] = value
	}
	return out
}

func cloneMetricLabels(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels))
	for name, value := range labels {
		out[name] = value
	}
	return out
}

// FillDuration sets the result's duration from wall clock if not already set.
func FillDuration(res *Result, start time.Time) {
	if res.Cost.Duration == 0 {
		res.Cost.Duration = time.Since(start)
	}
}

// ForceErrorSignal sets the signal to CommandError if Err is non-nil.
func ForceErrorSignal(res *Result) {
	if res.Err != nil {
		res.Signal = CommandError
	}
}

func stampSpan(tr tracing.Tracer, name string, res Result) {
	tr.SetAttributes(
		attribute.String("command.name", name),
		attribute.String("command.signal", string(res.Signal)),
		attribute.Int64("command.duration_ms", res.Cost.Duration.Milliseconds()),
		genai.AttrUsageInputTokens.Int(res.Cost.TokensIn),
		genai.AttrUsageOutputTokens.Int(res.Cost.TokensOut),
	)
	if res.Err != nil {
		tr.SetAttributes(genai.ErrorAttrs(res.Err.Error())...)
		tr.RecordError(res.Err)
	}
	if res.Metrics != nil {
		tr.SetAttributes(
			attribute.Int("tool.metrics.total", res.Metrics.Total),
			attribute.Int("tool.metrics.passed", res.Metrics.Passed),
			attribute.Int("tool.metrics.failed", res.Metrics.Failed),
		)
	}
}
