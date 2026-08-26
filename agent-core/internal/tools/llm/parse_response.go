// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const doneToolName = "done"

type parseResponseCmd struct {
	toolName     string
	raw          string
	registry     *core.Registry
	parser       modelllm.ResponseParser
	tracer       tracing.Tracer
	state        core.State
	captureLevel CaptureLevel
	retry        *ParseErrorRetryTracker
	prevRetries  int
	hasSnapshot  bool
}

func (p *parseResponseCmd) Name() string {
	if p.toolName != "" {
		return p.toolName
	}
	return "parse_response"
}

// SetTracer receives the active dispatch span so parsing attributes and events
// remain isolated to the turn that produced the response.
func (p *parseResponseCmd) SetTracer(tracer tracing.Tracer) {
	p.tracer = tracerOrNoop(tracer)
}

var _ core.TracerAware = (*parseResponseCmd)(nil)

func (p *parseResponseCmd) Execute() core.Result {
	p.snapshotRetry()
	p.tracer.SetAttributes(attribute.Int("raw_response_bytes", len(p.raw)))
	if p.captureLevel.CapturesFullContent() {
		p.tracer.SetAttributes(attribute.String("llm.raw_output", p.raw))
	}
	toolReq, sig, errMsg := p.parse(p.tracer)
	p.retry.RecordParseResult(sig)
	var res core.Result
	if sig == core.ParseFailed {
		p.tracer.Event("parse_failed", attribute.String("reason", errMsg))
		res = core.Result{Signal: core.ParseFailed, Output: errMsg}
	} else {
		res = p.resultForToolRequest(toolReq, sig)
	}
	res.CommandName = p.Name()
	if p.hasSnapshot {
		res.Receipt = encodeRetryReceipt(p.prevRetries)
	}
	return res
}

func (p *parseResponseCmd) snapshotRetry() {
	if p.retry != nil {
		p.prevRetries = p.retry.Snapshot()
		p.hasSnapshot = true
	}
}

func (p *parseResponseCmd) resultForToolRequest(toolReq modelllm.ToolRequest, sig core.Signal) core.Result {
	isDone := toolReq.ToolName == doneToolName
	p.tracer.SetAttributes(attribute.String("tool_name", toolReq.ToolName), attribute.Bool("is_done_tool", isDone))
	if isDone {
		summary := modelllm.ExtractDoneSummary(toolReq.Params)
		p.tracer.SetAttributes(attribute.String("done.summary", summary))
		return core.Result{Signal: sig, Output: summary, CommandName: p.Name()}
	}
	out, err := json.Marshal(toolReq)
	if err != nil {
		return core.Result{Signal: core.ParseFailed, Output: fmt.Sprintf("failed to serialize ToolRequest: %v", err)}
	}
	return core.Result{Signal: sig, Output: string(out), CommandName: p.Name()}
}

func (p *parseResponseCmd) parse(span tracing.Tracer) (modelllm.ToolRequest, core.Signal, string) {
	parser := p.parser
	if parser == nil {
		parser = modelllm.DefaultProfile()
	}
	cleaned := p.cleanRaw(parser, span)
	envelope, ok, errMsg := decodeEnvelope(cleaned, span)
	if !ok {
		return modelllm.ToolRequest{}, core.ParseFailed, errMsg
	}
	return p.validateEnvelope(cleaned, envelope, span)
}

func (p *parseResponseCmd) cleanRaw(parser modelllm.ResponseParser, span tracing.Tracer) string {
	cleaned := parser.ExtractToolCall(p.raw)
	if cleaned != strings.TrimSpace(p.raw) {
		span.Event("parse.correction", attribute.String("type", "envelope_extraction"))
	}
	if n := modelllm.CountToolCallBlocks(p.raw); n > 1 {
		span.Event("parse.correction", attribute.String("type", "multi_tool_call_dropped"), attribute.Int("total_blocks", n))
	}
	return cleaned
}

type responseEnvelope struct {
	Tool   string          `json:"tool"`
	Params json.RawMessage `json:"parameters"`
}

func decodeEnvelope(cleaned string, span tracing.Tracer) (responseEnvelope, bool, string) {
	var envelope responseEnvelope
	if err := json.Unmarshal([]byte(cleaned), &envelope); err != nil {
		fixed := modelllm.FixNewlinesInStrings(cleaned)
		if err2 := json.Unmarshal([]byte(fixed), &envelope); err2 != nil {
			return responseEnvelope{}, false, fmt.Sprintf("malformed JSON: %v", err)
		}
		span.Event("parse.correction", attribute.String("type", "fix_newlines_in_strings"))
	}
	if envelope.Tool == "" {
		return responseEnvelope{}, false, `missing required field "tool"`
	}
	return envelope, true, ""
}

func (p *parseResponseCmd) validateEnvelope(cleaned string, envelope responseEnvelope, span tracing.Tracer) (modelllm.ToolRequest, core.Signal, string) {
	if envelope.Params == nil {
		envelope.Params = modelllm.ExtractFlatParams(cleaned, envelope.Tool)
	}
	tr := modelllm.ToolRequest{ToolName: envelope.Tool, Params: envelope.Params}
	if envelope.Tool == doneToolName {
		return tr, core.TaskCompleted, ""
	}
	spec, _, availability := p.registry.ResolveExternalTool(envelope.Tool, p.state)
	switch availability {
	case core.ExternalToolUnknown, core.ExternalToolInternal:
		return modelllm.ToolRequest{}, core.ParseFailed, fmt.Sprintf("unknown tool %q; available tools: [%s]", envelope.Tool, strings.Join(p.registry.ExternalToolNames(), ", "))
	case core.ExternalToolUnavailableInState:
		return modelllm.ToolRequest{}, core.ParseFailed, fmt.Sprintf("tool %q is not available in state %q; available tools: [%s]", envelope.Tool, p.state, strings.Join(p.availableToolNames(), ", "))
	}
	if missing := modelllm.CheckRequiredFields(spec.InputSchema, envelope.Params); len(missing) > 0 {
		span.Event("parse.missing_params", attribute.Int("missing_count", len(missing)))
		return modelllm.ToolRequest{}, core.ParseFailed, fmt.Sprintf("tool %q missing required parameters: [%s]", envelope.Tool, strings.Join(missing, ", "))
	}
	return tr, core.ToolDone, ""
}

func (p *parseResponseCmd) availableToolNames() []string {
	return p.registry.AvailableExternalToolNames(p.state)
}

// ParseResponseBuilder constructs parse_response commands.
type ParseResponseBuilder struct {
	ToolName     string
	Registry     *core.Registry
	Parser       modelllm.ResponseParser
	Tracer       tracing.Tracer
	State        core.State
	CaptureLevel CaptureLevel
	Retry        *ParseErrorRetryTracker
}

func (b *ParseResponseBuilder) Build(res core.Result) core.Command {
	state := b.manifestState(res)
	return &parseResponseCmd{
		toolName: b.ToolName,
		raw:      res.Output, registry: b.Registry, parser: b.Parser,
		tracer: tracerOrNoop(b.Tracer), state: state, captureLevel: b.CaptureLevel, retry: b.Retry,
	}
}

// BuildReverser constructs the receipt-only parse command used after restart.
func (b *ParseResponseBuilder) BuildReverser() core.Command {
	return &parseResponseCmd{
		toolName: b.ToolName,
		retry:    b.Retry,
	}
}

var _ core.Reverser = (*ParseResponseBuilder)(nil)

func (b *ParseResponseBuilder) manifestState(res core.Result) core.State {
	if b.State != "" {
		return b.State
	}
	return res.State
}
