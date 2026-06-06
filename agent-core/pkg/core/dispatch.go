// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/telemetry/genai"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
)

// Dispatch wraps Command.Execute with tracing, timeout, panic
// recovery, duration measurement, and CommandName assignment.
func Dispatch(cmd Command, tr tracing.Tracer, timeout time.Duration) Result {
	spanName := genai.ToolSpanName(cmd.Name())
	var spanAttrs []attribute.KeyValue
	spanAttrs = append(spanAttrs, genai.ToolAttrs(cmd.Name(), genai.ToolTypeFunction)...)

	if so, ok := cmd.(SpanOverride); ok {
		spanName = so.SpanName()
		spanAttrs = so.SpanCreationAttrs()
	}

	child, done := tr.Push(spanName, spanAttrs...)
	defer done()

	res := SafeExecute(cmd, timeout)

	res.CommandName = cmd.Name()
	stampSpan(child, cmd.Name(), res)
	return res
}

// SafeExecute runs a command with panic recovery and optional timeout.
func SafeExecute(cmd Command, timeout time.Duration) (res Result) {
	done := make(chan struct{})
	start := time.Now()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				res = Result{
					Signal: CommandError,
					Err:    fmt.Errorf("panic in %s: %v", cmd.Name(), r),
					Output: fmt.Sprintf("panic: %v", r),
				}
			}
			close(done)
		}()
		res = cmd.Execute()
	}()

	if timeout <= 0 {
		<-done
		FillDuration(&res, start)
		ForceErrorSignal(&res)
		return res
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
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
