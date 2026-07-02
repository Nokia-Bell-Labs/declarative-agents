// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	modelllm "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

type reportParseErrorCmd struct {
	errorText   string
	tracer      tracing.Tracer
	retry       *ParseErrorRetryTracker
	prevRetries int
	hasSnapshot bool
}

func (r *reportParseErrorCmd) Name() string { return "report_parse_error" }

func (r *reportParseErrorCmd) Execute() core.Result {
	if r.retry != nil {
		r.prevRetries = r.retry.Snapshot()
		r.hasSnapshot = true
	}
	sig := r.retry.ReportParseError()
	r.tracer.Event("parse_error_reported",
		attribute.String("error_class", modelllm.ClassifyParseError(r.errorText)),
		attribute.String("signal", string(sig)),
	)
	if sig == core.BudgetExhausted {
		return core.Result{Signal: sig, Output: fmt.Sprintf("parse error retry limit reached: %s", r.errorText)}
	}
	return core.Result{Signal: core.ToolDone, Output: parseFeedback(r.errorText)}
}

func parseFeedback(errorText string) string {
	return fmt.Sprintf(
		"Your previous response was invalid. %s\n\n"+
			"Please respond with a single JSON object: {\"tool\": \"<tool_name>\", \"parameters\": {<params>}}",
		errorText,
	)
}

func (r *reportParseErrorCmd) Undo() core.Result {
	if r.retry == nil {
		return core.NoopUndo(r.Name())
	}
	if !r.hasSnapshot {
		err := fmt.Errorf("undo report_parse_error: no retry counter snapshot recorded")
		return core.Result{Signal: core.CommandError, CommandName: r.Name(), Output: err.Error(), Err: err}
	}
	r.retry.Restore(r.prevRetries)
	return core.Result{Signal: core.ToolDone, CommandName: r.Name(), Output: fmt.Sprintf("undo: restored parse retry counter to %d", r.prevRetries)}
}

func (r *reportParseErrorCmd) UndoMemento() (core.UndoMemento, error) {
	if r.retry == nil {
		return core.NoopUndoMemento(r.Name()), nil
	}
	if !r.hasSnapshot {
		return core.UndoMemento{}, fmt.Errorf("%w: no retry counter snapshot recorded for %s", core.ErrUndoMementoMissing, r.Name())
	}
	return retryCounterMemento(r.Name(), r.prevRetries)
}

// ReportParseErrorBuilder constructs report_parse_error commands.
type ReportParseErrorBuilder struct {
	Tracer tracing.Tracer
	Retry  *ParseErrorRetryTracker
}

func (b *ReportParseErrorBuilder) Build(res core.Result) core.Command {
	return &reportParseErrorCmd{errorText: res.Output, tracer: b.Tracer, retry: b.Retry}
}
