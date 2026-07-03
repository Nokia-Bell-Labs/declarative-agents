// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
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
	var res core.Result
	if sig == core.BudgetExhausted {
		res = core.Result{Signal: sig, Output: fmt.Sprintf("parse error retry limit reached: %s", r.errorText)}
	} else {
		res = core.Result{Signal: core.ToolDone, Output: parseFeedback(r.errorText)}
	}
	if r.hasSnapshot {
		res.Receipt = encodeRetryReceipt(r.prevRetries)
	}
	return res
}

func parseFeedback(errorText string) string {
	return fmt.Sprintf(
		"Your previous response was invalid. %s\n\n"+
			"Please respond with a single JSON object: {\"tool\": \"<tool_name>\", \"parameters\": {<params>}}",
		errorText,
	)
}

// Undo restores the parse-retry counter, preferring the tool-owned receipt on
// the prior Result and falling back to the in-memory snapshot on the live path
// (srd035-checkpoint-port R3; #44 R2, R3).
func (r *reportParseErrorCmd) Undo(prior core.Result) core.Result {
	if r.retry == nil {
		return core.NoopUndo(r.Name())
	}
	if retries, ok, err := decodeRetryReceipt(prior.Receipt); err != nil {
		e := fmt.Errorf("undo report_parse_error: decode receipt: %w", err)
		return core.Result{Signal: core.CommandError, CommandName: r.Name(), Output: e.Error(), Err: e}
	} else if ok {
		r.retry.Restore(retries)
		return core.Result{Signal: core.ToolDone, CommandName: r.Name(), Output: fmt.Sprintf("undo: restored parse retry counter to %d", retries)}
	}
	if !r.hasSnapshot {
		err := fmt.Errorf("undo report_parse_error: no retry counter snapshot recorded")
		return core.Result{Signal: core.CommandError, CommandName: r.Name(), Output: err.Error(), Err: err}
	}
	r.retry.Restore(r.prevRetries)
	return core.Result{Signal: core.ToolDone, CommandName: r.Name(), Output: fmt.Sprintf("undo: restored parse retry counter to %d", r.prevRetries)}
}

// ReportParseErrorBuilder constructs report_parse_error commands.
type ReportParseErrorBuilder struct {
	Tracer tracing.Tracer
	Retry  *ParseErrorRetryTracker
}

func (b *ReportParseErrorBuilder) Build(res core.Result) core.Command {
	return &reportParseErrorCmd{errorText: res.Output, tracer: b.Tracer, retry: b.Retry}
}
