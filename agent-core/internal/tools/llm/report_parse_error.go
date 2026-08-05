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
	errorText        string
	responseContract ParseErrorResponseContract
	tracer           tracing.Tracer
	retry            *ParseErrorRetryTracker
	prevRetries      int
	hasSnapshot      bool
}

// ParseErrorResponseContract selects the response shape requested after a
// parsing failure. The zero value preserves the historical tool-call JSON
// correction for existing profiles.
type ParseErrorResponseContract string

const (
	ParseErrorResponseContractToolCallJSON           ParseErrorResponseContract = "tool_call_json"
	ParseErrorResponseContractImplementationPlanYAML ParseErrorResponseContract = "implementation_plan_yaml"
)

// ParseErrorResponseContractValue validates a declared response contract.
func ParseErrorResponseContractValue(value string) (ParseErrorResponseContract, error) {
	switch contract := ParseErrorResponseContract(value); contract {
	case "", ParseErrorResponseContractToolCallJSON:
		return ParseErrorResponseContractToolCallJSON, nil
	case ParseErrorResponseContractImplementationPlanYAML:
		return contract, nil
	default:
		return "", fmt.Errorf(
			"unknown response_contract %q (want %q or %q)",
			value,
			ParseErrorResponseContractToolCallJSON,
			ParseErrorResponseContractImplementationPlanYAML,
		)
	}
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
		res = core.Result{Signal: core.ToolDone, Output: parseFeedback(r.errorText, r.responseContract)}
	}
	if r.hasSnapshot {
		res.Receipt = encodeRetryReceipt(r.prevRetries)
	}
	return res
}

func parseFeedback(errorText string, contract ParseErrorResponseContract) string {
	prefix := fmt.Sprintf("Your previous response was invalid. %s\n\n", errorText)
	if contract == ParseErrorResponseContractImplementationPlanYAML {
		return prefix +
			"Please respond with exactly one top-level YAML mapping and no other document content. " +
			"The mapping must contain exactly these six keys: title, summary, files, requirements, " +
			"design_decisions, and acceptance_criteria. Do not return a root sequence/list, multiple " +
			"plans, a wrapper/envelope key, Markdown fences, prose, or any keys outside this mapping."
	}
	return prefix +
		"Please respond with a single JSON object: {\"tool\": \"<tool_name>\", \"parameters\": {<params>}}"
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
	Tracer           tracing.Tracer
	Retry            *ParseErrorRetryTracker
	ResponseContract ParseErrorResponseContract
}

func (b *ReportParseErrorBuilder) Build(res core.Result) core.Command {
	return &reportParseErrorCmd{
		errorText:        res.Output,
		responseContract: b.ResponseContract,
		tracer:           b.Tracer,
		retry:            b.Retry,
	}
}
