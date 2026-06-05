// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
)

const doneToolName = "done"

// --- invoke_llm command ---

type invokeLLMCmd struct {
	client       llm.Client
	history      *llm.Conversation
	registry     *core.Registry
	assembler    llm.PromptAssembler
	state        core.State
	model        string
	userMessage  string
	tracer       tracing.Tracer
	contextLimit int
	verbose      bool
	ctx          context.Context
}

func (c *invokeLLMCmd) Name() string { return "invoke_llm" }

func (c *invokeLLMCmd) Execute() core.Result {
	if c.ctx == nil {
		c.ctx = context.Background()
	}
	child, done := c.tracer.Push(c.Name())
	defer done()

	c.history.Append(llm.Message{Role: llm.User, Content: c.userMessage})
	child.Event("history.user_appended",
		attribute.Int("history_len", c.history.Len()),
	)

	messages := c.assembler.AssembleMessages(c.history, c.registry, c.state)

	child.Event("prompt.assembled",
		attribute.Int("message_count", len(messages)),
		attribute.Int("history_messages", c.history.Len()),
	)

	if c.contextLimit > 0 {
		est := llm.EstimateTokens(messages)
		child.SetAttributes(
			attribute.Int("context.estimated_tokens", est),
			attribute.Int("context.limit", c.contextLimit),
		)
		if est > c.contextLimit {
			child.Event("context.overflow",
				attribute.Int("estimated", est),
				attribute.Int("limit", c.contextLimit),
			)
			return core.Result{
				Signal: core.CommandError,
				Err:    fmt.Errorf("context window exhaustion: estimated %d tokens exceeds limit %d", est, c.contextLimit),
				Output: fmt.Sprintf("assembled prompt (%d estimated tokens) does not fit in context window (%d)", est, c.contextLimit),
			}
		}
	}

	opts := llm.ChatOptions{
		Model:       c.model,
		Temperature: 0,
		Seed:        42,
	}

	if c.verbose {
		if inputJSON, err := json.Marshal(messages); err == nil {
			child.SetAttributes(attribute.String("llm.input", string(inputJSON)))
		}
	}

	child.Event("chat.request_start")
	start := time.Now()
	chatResp, err := c.client.Chat(c.ctx, messages, opts)
	duration := time.Since(start)

	if err != nil {
		child.Event("chat.request_error", attribute.String("error.message", err.Error()))
		return core.Result{
			Signal: core.CommandError,
			Err:    err,
			Output: err.Error(),
			Cost:   core.Cost{Duration: duration},
		}
	}

	child.Event("chat.request_done",
		attribute.Int("response_content_len", len(chatResp.Content)),
	)

	c.history.Append(llm.Message{Role: llm.Assistant, Content: chatResp.Content})
	child.Event("history.assistant_appended",
		attribute.Int("history_len", c.history.Len()),
	)

	cost := core.Cost{
		Duration:  duration,
		TokensIn:  chatResp.TokensIn,
		TokensOut: chatResp.TokensOut,
		Dollars:   0,
	}

	child.SetAttributes(
		attribute.String("model", c.model),
		attribute.Int("tokens_in", cost.TokensIn),
		attribute.Int("tokens_out", cost.TokensOut),
		attribute.Int64("duration_ms", cost.Duration.Milliseconds()),
	)

	if c.verbose {
		child.SetAttributes(attribute.String("llm.output", chatResp.Content))
	}

	return core.Result{
		Signal: core.LLMResponded,
		Output: chatResp.Content,
		Cost:   cost,
	}
}

// InvokeLLMBuilder constructs invoke_llm commands. All behavior is
// configured through interfaces: Client for the provider, Assembler
// for prompt structure, and the Registry for tool manifests.
type InvokeLLMBuilder struct {
	Client       llm.Client
	History      *llm.Conversation
	Registry     *core.Registry
	Assembler    llm.PromptAssembler
	State        core.State
	Model        string
	Tracer       tracing.Tracer
	ContextLimit int
	Verbose      bool
	Ctx          context.Context
}

func (b *InvokeLLMBuilder) Build(res core.Result) core.Command {
	ctx := b.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return &invokeLLMCmd{
		client:       b.Client,
		history:      b.History,
		registry:     b.Registry,
		assembler:    b.Assembler,
		state:        b.State,
		model:        b.Model,
		userMessage:  res.Output,
		tracer:       b.Tracer,
		contextLimit: b.ContextLimit,
		verbose:      b.Verbose,
		ctx:          ctx,
	}
}

// --- parse_response command ---

type parseResponseCmd struct {
	raw      string
	registry *core.Registry
	parser   llm.ResponseParser
	tracer   tracing.Tracer
	verbose  bool
}

func (p *parseResponseCmd) Name() string { return "parse_response" }

func (p *parseResponseCmd) Execute() core.Result {
	child, done := p.tracer.Push(p.Name())
	defer done()

	child.SetAttributes(attribute.Int("raw_response_bytes", len(p.raw)))
	if p.verbose {
		child.SetAttributes(attribute.String("llm.raw_output", p.raw))
	}

	tr, sig, errMsg := p.parse(child)
	if sig == core.ParseFailed {
		child.Event("parse_failed", attribute.String("reason", errMsg))
		child.SetAttributes(attribute.String("outcome", "failed"))
		return core.Result{Signal: core.ParseFailed, Output: errMsg}
	}

	isDone := tr.ToolName == doneToolName
	child.SetAttributes(
		attribute.String("tool_name", tr.ToolName),
		attribute.String("outcome", string(sig)),
		attribute.Bool("is_done_tool", isDone),
	)
	if p.verbose {
		child.SetAttributes(attribute.String("tool.params", string(tr.Params)))
	}

	if isDone {
		summary := llm.ExtractDoneSummary(tr.Params)
		child.SetAttributes(attribute.String("done.summary", summary))
		return core.Result{Signal: sig, Output: summary, CommandName: p.Name()}
	}

	out, err := json.Marshal(tr)
	if err != nil {
		return core.Result{Signal: core.ParseFailed, Output: fmt.Sprintf("failed to serialize ToolRequest: %v", err)}
	}

	return core.Result{Signal: sig, Output: string(out), CommandName: p.Name()}
}

func (p *parseResponseCmd) parse(span tracing.Tracer) (llm.ToolRequest, core.Signal, string) {
	parser := p.parser
	if parser == nil {
		parser = llm.DefaultProfile()
	}

	cleaned := parser.ExtractToolCall(p.raw)
	if cleaned != strings.TrimSpace(p.raw) {
		span.Event("parse.correction",
			attribute.String("type", "envelope_extraction"),
			attribute.String("from", llm.Truncate(p.raw, 200)),
			attribute.String("to", llm.Truncate(cleaned, 200)),
		)
	}

	if n := llm.CountToolCallBlocks(p.raw); n > 1 {
		span.Event("parse.correction",
			attribute.String("type", "multi_tool_call_dropped"),
			attribute.Int("total_blocks", n),
			attribute.Int("executed_blocks", 1),
		)
	}

	var envelope struct {
		Tool   string          `json:"tool"`
		Params json.RawMessage `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(cleaned), &envelope); err != nil {
		fixed := llm.FixNewlinesInStrings(cleaned)
		if err2 := json.Unmarshal([]byte(fixed), &envelope); err2 != nil {
			return llm.ToolRequest{}, core.ParseFailed, fmt.Sprintf("malformed JSON: %v", err)
		}
		span.Event("parse.correction",
			attribute.String("type", "fix_newlines_in_strings"),
			attribute.String("from", llm.Truncate(cleaned, 200)),
			attribute.String("to", llm.Truncate(fixed, 200)),
		)
		cleaned = fixed
	}

	if envelope.Tool == "" {
		return llm.ToolRequest{}, core.ParseFailed, `missing required field "tool"`
	}

	if envelope.Params == nil {
		rewrapped := llm.ExtractFlatParams(cleaned, envelope.Tool)
		envelope.Params = rewrapped
		if string(rewrapped) != "{}" {
			span.Event("parse.correction",
				attribute.String("type", "flat_params_rewrap"),
				attribute.String("from", llm.Truncate(cleaned, 200)),
				attribute.String("to_params", llm.Truncate(string(rewrapped), 200)),
			)
		}
	}

	tr := llm.ToolRequest{ToolName: envelope.Tool, Params: envelope.Params}

	if envelope.Tool == doneToolName {
		return tr, core.TaskCompleted, ""
	}

	spec, ok := p.registry.SpecByName(envelope.Tool)
	if !ok || spec.Visibility != core.External {
		return llm.ToolRequest{}, core.ParseFailed, fmt.Sprintf(
			"unknown tool %q; available tools: [%s]",
			envelope.Tool,
			strings.Join(p.registry.ExternalToolNames(), ", "),
		)
	}

	if missing := llm.CheckRequiredFields(spec.InputSchema, envelope.Params); len(missing) > 0 {
		span.Event("parse.missing_params",
			attribute.Int("missing_count", len(missing)),
			attribute.String("missing_names", strings.Join(missing, ",")),
		)
		return llm.ToolRequest{}, core.ParseFailed, fmt.Sprintf(
			"tool %q missing required parameters: [%s]",
			envelope.Tool,
			strings.Join(missing, ", "),
		)
	}

	return tr, core.ToolDone, ""
}

// ParseResponseBuilder constructs parse_response commands. The Parser
// handles model-specific response extraction; the Registry validates
// tool names and required parameters.
type ParseResponseBuilder struct {
	Registry *core.Registry
	Parser   llm.ResponseParser
	Tracer   tracing.Tracer
	Verbose  bool
}

func (b *ParseResponseBuilder) Build(res core.Result) core.Command {
	return &parseResponseCmd{
		raw:      res.Output,
		registry: b.Registry,
		parser:   b.Parser,
		tracer:   b.Tracer,
		verbose:  b.Verbose,
	}
}

// --- report_parse_error command ---

type reportParseErrorCmd struct {
	errorText string
	tracer    tracing.Tracer
}

func (r *reportParseErrorCmd) Name() string { return "report_parse_error" }

func (r *reportParseErrorCmd) Execute() core.Result {
	child, done := r.tracer.Push(r.Name())
	defer done()

	child.Event("parse_error_reported",
		attribute.String("error_class", llm.ClassifyParseError(r.errorText)),
	)

	feedback := fmt.Sprintf(
		"Your previous response was invalid. %s\n\n"+
			"Please respond with a single JSON object: {\"tool\": \"<tool_name>\", \"parameters\": {<params>}}",
		r.errorText,
	)
	return core.Result{Signal: core.ToolDone, Output: feedback}
}

// ReportParseErrorBuilder constructs report_parse_error commands from
// the error description produced by parse_response.
type ReportParseErrorBuilder struct {
	Tracer tracing.Tracer
}

func (b *ReportParseErrorBuilder) Build(res core.Result) core.Command {
	return &reportParseErrorCmd{errorText: res.Output, tracer: b.Tracer}
}

// --- reset_history command ---

type resetHistoryCmd struct {
	history *llm.Conversation
	tracer  tracing.Tracer
}

func (r *resetHistoryCmd) Name() string { return "reset_history" }

func (r *resetHistoryCmd) Execute() core.Result {
	child, done := r.tracer.Push(r.Name())
	defer done()

	prevLen := r.history.Len()
	r.history.Reset()

	child.SetAttributes(
		attribute.Int("history.cleared_messages", prevLen),
	)

	return core.Result{
		Signal:      core.ToolDone,
		Output:      "Begin.",
		CommandName: r.Name(),
	}
}

// ResetHistoryBuilder constructs reset_history commands that clear
// the conversation history for a fresh LLM context.
type ResetHistoryBuilder struct {
	History *llm.Conversation
	Tracer  tracing.Tracer
}

func (b *ResetHistoryBuilder) Build(_ core.Result) core.Command {
	return &resetHistoryCmd{
		history: b.History,
		tracer:  b.Tracer,
	}
}
