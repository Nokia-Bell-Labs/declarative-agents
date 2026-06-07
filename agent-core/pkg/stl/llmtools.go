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
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/telemetry/genai"
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
	providerName string
	serverAddr   string
	userMessage  string
	tracer       tracing.Tracer
	contextLimit int
	numCtx       int
	verbose      bool
	ctx          context.Context
	callTimeout  time.Duration
}

func (c *invokeLLMCmd) Name() string { return "invoke_llm" }

// SpanName implements core.SpanOverride so Dispatch emits a semconv
// inference span instead of execute_tool.
func (c *invokeLLMCmd) SpanName() string {
	return genai.InferenceSpanName(c.model)
}

// SpanCreationAttrs implements core.SpanOverride.
func (c *invokeLLMCmd) SpanCreationAttrs() []attribute.KeyValue {
	return genai.InferenceAttrs(c.providerName, c.model, c.serverAddr)
}

func (c *invokeLLMCmd) Execute() core.Result {
	if c.ctx == nil {
		c.ctx = context.Background()
	}
	// No inner Push — Dispatch already creates the semconv span via SpanOverride.
	// Use c.tracer which IS the Dispatch child span.
	tr := c.tracer

	c.history.Append(llm.Message{Role: llm.User, Content: c.userMessage})
	tr.Event("history.user_appended",
		attribute.Int("history_len", c.history.Len()),
	)

	messages := c.assembler.AssembleMessages(c.history, c.registry, c.state)

	tr.Event("prompt.assembled",
		attribute.Int("message_count", len(messages)),
		attribute.Int("history_messages", c.history.Len()),
	)

	if c.contextLimit > 0 {
		est := llm.EstimateTokens(messages)
		tr.SetAttributes(
			attribute.Int("context.estimated_tokens", est),
			attribute.Int("context.limit", c.contextLimit),
		)
		if est > c.contextLimit {
			tr.Event("context.overflow",
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
		NumCtx:      c.numCtx,
	}

	if c.verbose {
		if inputJSON, err := json.Marshal(messages); err == nil {
			tr.SetAttributes(genai.AttrInputMessages.String(string(inputJSON)))
		}
	}

	chatCtx := c.ctx
	if c.callTimeout > 0 {
		var cancel context.CancelFunc
		chatCtx, cancel = context.WithTimeout(c.ctx, c.callTimeout)
		defer cancel()
	}

	tr.Event("chat.request_start")
	start := time.Now()
	chatResp, err := c.client.Chat(chatCtx, messages, opts)
	duration := time.Since(start)

	if err != nil {
		return core.Result{
			Signal: core.CommandError,
			Err:    err,
			Output: err.Error(),
			Cost:   core.Cost{Duration: duration},
		}
	}

	tr.Event("chat.request_done",
		attribute.Int("response_content_len", len(chatResp.Content)),
	)

	c.history.Append(llm.Message{Role: llm.Assistant, Content: chatResp.Content})
	tr.Event("history.assistant_appended",
		attribute.Int("history_len", c.history.Len()),
	)

	cost := core.Cost{
		Duration:  duration,
		TokensIn:  chatResp.TokensIn,
		TokensOut: chatResp.TokensOut,
		Dollars:   0,
	}

	tr.SetAttributes(
		genai.AttrUsageInputTokens.Int(cost.TokensIn),
		genai.AttrUsageOutputTokens.Int(cost.TokensOut),
		attribute.Int64("duration_ms", cost.Duration.Milliseconds()),
	)

	if c.verbose {
		tr.SetAttributes(genai.AttrOutputMessages.String(chatResp.Content))
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
	ProviderName string // e.g. "ollama"
	ServerAddr   string // e.g. "localhost:11434"
	Tracer       tracing.Tracer
	ContextLimit int
	NumCtx       int           // Ollama num_ctx: context window size for inference
	CallTimeout  time.Duration // per-call deadline; 0 = no limit
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
		providerName: b.ProviderName,
		serverAddr:   b.ServerAddr,
		userMessage:  res.Output,
		tracer:       b.Tracer,
		contextLimit: b.ContextLimit,
		numCtx:       b.NumCtx,
		callTimeout:  b.CallTimeout,
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
	tr := p.tracer

	tr.SetAttributes(attribute.Int("raw_response_bytes", len(p.raw)))
	if p.verbose {
		tr.SetAttributes(attribute.String("llm.raw_output", p.raw))
	}

	toolReq, sig, errMsg := p.parse(tr)
	if sig == core.ParseFailed {
		tr.Event("parse_failed", attribute.String("reason", errMsg))
		tr.SetAttributes(attribute.String("outcome", "failed"))
		return core.Result{Signal: core.ParseFailed, Output: errMsg}
	}

	isDone := toolReq.ToolName == doneToolName
	tr.SetAttributes(
		attribute.String("tool_name", toolReq.ToolName),
		attribute.String("outcome", string(sig)),
		attribute.Bool("is_done_tool", isDone),
	)
	if p.verbose {
		tr.SetAttributes(attribute.String("tool.params", string(toolReq.Params)))
	}

	if isDone {
		summary := llm.ExtractDoneSummary(toolReq.Params)
		tr.SetAttributes(attribute.String("done.summary", summary))
		return core.Result{Signal: sig, Output: summary, CommandName: p.Name()}
	}

	out, err := json.Marshal(toolReq)
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
	r.tracer.Event("parse_error_reported",
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
	prevLen := r.history.Len()
	r.history.Reset()

	r.tracer.SetAttributes(
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
