// Copyright (c) 2026 Nokia. All rights reserved.

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
	raw         string
	registry    *core.Registry
	parser      modelllm.ResponseParser
	tracer      tracing.Tracer
	verbose     bool
	retry       *ParseErrorRetryTracker
	prevRetries int
	hasSnapshot bool
}

func (p *parseResponseCmd) Name() string { return "parse_response" }

func (p *parseResponseCmd) Execute() core.Result {
	p.snapshotRetry()
	p.tracer.SetAttributes(attribute.Int("raw_response_bytes", len(p.raw)))
	if p.verbose {
		p.tracer.SetAttributes(attribute.String("llm.raw_output", p.raw))
	}
	toolReq, sig, errMsg := p.parse(p.tracer)
	p.retry.RecordParseResult(sig)
	if sig == core.ParseFailed {
		p.tracer.Event("parse_failed", attribute.String("reason", errMsg))
		return core.Result{Signal: core.ParseFailed, Output: errMsg}
	}
	return p.resultForToolRequest(toolReq, sig)
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
	spec, ok := p.registry.SpecByName(envelope.Tool)
	if !ok || spec.Visibility != core.External {
		return modelllm.ToolRequest{}, core.ParseFailed, fmt.Sprintf("unknown tool %q; available tools: [%s]", envelope.Tool, strings.Join(p.registry.ExternalToolNames(), ", "))
	}
	if missing := modelllm.CheckRequiredFields(spec.InputSchema, envelope.Params); len(missing) > 0 {
		span.Event("parse.missing_params", attribute.Int("missing_count", len(missing)))
		return modelllm.ToolRequest{}, core.ParseFailed, fmt.Sprintf("tool %q missing required parameters: [%s]", envelope.Tool, strings.Join(missing, ", "))
	}
	return tr, core.ToolDone, ""
}

// ParseResponseBuilder constructs parse_response commands.
type ParseResponseBuilder struct {
	Registry *core.Registry
	Parser   modelllm.ResponseParser
	Tracer   tracing.Tracer
	Verbose  bool
	Retry    *ParseErrorRetryTracker
}

func (b *ParseResponseBuilder) Build(res core.Result) core.Command {
	return &parseResponseCmd{
		raw: res.Output, registry: b.Registry, parser: b.Parser,
		tracer: b.Tracer, verbose: b.Verbose, retry: b.Retry,
	}
}
