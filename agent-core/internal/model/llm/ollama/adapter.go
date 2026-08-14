// Copyright (c) 2026 Nokia. All rights reserved.

// Package ollama implements the llm.Client interface for the Ollama
// inference server through POST /api/chat.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry/genai"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

const instrumentationName = "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm/ollama"

// chatReq is the JSON body sent to Ollama POST /api/chat.
type chatReq struct {
	Model    string   `json:"model"`
	Messages []msgDTO `json:"messages"`
	Stream   bool     `json:"stream"`
	Options  chatOpts `json:"options"`
}

type msgDTO struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatOpts struct {
	Temperature float64 `json:"temperature"`
	Seed        int     `json:"seed"`
	NumCtx      int     `json:"num_ctx,omitempty"`
}

// chatResp is the JSON body returned from Ollama POST /api/chat.
type chatResp struct {
	Message         msgDTO `json:"message"`
	EvalCount       int    `json:"eval_count"`
	PromptEvalCount int    `json:"prompt_eval_count"`
}

// Adapter wraps the Ollama HTTP API and implements llm.Client.
type Adapter struct {
	baseURL string
	model   string
	client  *http.Client
	tracer  tracing.Tracer
}

var _ llm.Client = (*Adapter)(nil)

// NewAdapter creates an Adapter. Availability checks are explicit profile
// words rather than hidden constructor I/O.
func NewAdapter(baseURL, model string, opts ...Option) (*Adapter, error) {
	a := &Adapter{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 5 * time.Minute},
		tracer:  tracing.NoopTracer{},
	}
	for _, opt := range opts {
		opt(a)
	}

	return a, nil
}

// Model returns the model name this adapter was created for.
func (a *Adapter) Model() string { return a.model }

// Chat sends a chat request to Ollama POST /api/chat and returns the
// response. Satisfies llm.Client.
func (a *Adapter) Chat(ctx context.Context, messages []llm.Message, opts llm.ChatOptions) (llm.ChatResponse, error) {
	ctx, tr, span := a.chatSpan(ctx, opts.Model)
	defer span()

	dtos := make([]msgDTO, len(messages))
	for i, m := range messages {
		dtos[i] = msgDTO{Role: string(m.Role), Content: m.Content}
	}

	req := chatReq{
		Model:    opts.Model,
		Messages: dtos,
		Stream:   false,
		Options:  chatOpts{Temperature: opts.Temperature, Seed: opts.Seed, NumCtx: opts.NumCtx},
	}

	body, err := json.Marshal(req)
	if err != nil {
		tr.RecordError(fmt.Errorf("marshal chat request: %w", err))
		return llm.ChatResponse{}, fmt.Errorf("marshal chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		tr.RecordError(fmt.Errorf("create HTTP request: %w", err))
		return llm.ChatResponse{}, fmt.Errorf("create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		tr.RecordError(fmt.Errorf("ollama chat request failed: %w", err))
		return llm.ChatResponse{}, fmt.Errorf("ollama chat request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("ollama /api/chat returned status %d: %s", resp.StatusCode, string(respBody))
		tr.SetAttributes(genai.AttrErrorType.String(fmt.Sprintf("%d", resp.StatusCode)))
		tr.RecordError(err)
		return llm.ChatResponse{}, err
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		tr.RecordError(fmt.Errorf("read chat response: %w", err))
		return llm.ChatResponse{}, fmt.Errorf("read chat response: %w", err)
	}

	var cr chatResp
	if err := json.Unmarshal(respBody, &cr); err != nil {
		tr.RecordError(fmt.Errorf("parse chat response: %w", err))
		return llm.ChatResponse{}, fmt.Errorf("parse chat response: %w", err)
	}

	usageAttrs := genai.UsageAttrs(cr.PromptEvalCount, cr.EvalCount)
	tr.SetAttributes(append(usageAttrs, genai.AttrResponseModel.String(opts.Model))...)

	return llm.ChatResponse{
		Content:   cr.Message.Content,
		TokensIn:  cr.PromptEvalCount,
		TokensOut: cr.EvalCount,
	}, nil
}

// chatSpan creates a semconv inference span parented by the active span in the
// caller context. The adapter tracer remains a safe fallback for direct calls
// that do not carry a span; no per-call adapter state is mutated.
func (a *Adapter) chatSpan(ctx context.Context, model string) (context.Context, tracing.Tracer, func()) {
	serverAddr := ""
	if u, err := url.Parse(a.baseURL); err == nil {
		serverAddr = u.Host
	}

	attrs := genai.InferenceAttrs(genai.ProviderOllama, model, serverAddr)
	if u, err := url.Parse(a.baseURL); err == nil && u.Port() != "" {
		port := 0
		n, err := fmt.Sscanf(u.Port(), "%d", &port)
		if err == nil && n == 1 && port > 0 {
			attrs = append(attrs, genai.AttrServerPort.Int(port))
		}
	}

	if ctx == nil {
		ctx = context.Background()
	}
	parent := oteltrace.SpanFromContext(ctx)
	if parent.SpanContext().IsValid() {
		tracer := parent.TracerProvider().Tracer(instrumentationName)
		child, done := contextTracer{tracer: tracer, ctx: ctx}.Push(genai.InferenceSpanName(model), attrs...)
		return child.Context(), child, done
	}
	if a.tracer == nil {
		return ctx, tracing.NoopTracer{}, func() {}
	}
	child, done := a.tracer.Push(genai.InferenceSpanName(model), attrs...)
	return contextWithTracerSpan(ctx, child), child, done
}

func contextWithTracerSpan(ctx context.Context, tracer tracing.Tracer) context.Context {
	span := oteltrace.SpanFromContext(tracer.Context())
	if span.SpanContext().IsValid() {
		return oteltrace.ContextWithSpan(ctx, span)
	}
	return ctx
}

// contextTracer is a request-scoped adapter over an OTel tracer. It exists so
// Chat can honor an arbitrary caller context without replacing Adapter.tracer.
type contextTracer struct {
	tracer oteltrace.Tracer
	ctx    context.Context
}

func (t contextTracer) Push(name string, attrs ...attribute.KeyValue) (tracing.Tracer, func()) {
	ctx, span := t.tracer.Start(t.ctx, name, oteltrace.WithAttributes(attrs...))
	return contextTracer{tracer: t.tracer, ctx: ctx}, func() { span.End() }
}

func (t contextTracer) Event(name string, attrs ...attribute.KeyValue) {
	oteltrace.SpanFromContext(t.ctx).AddEvent(name, oteltrace.WithAttributes(attrs...))
}

func (t contextTracer) SetAttributes(attrs ...attribute.KeyValue) {
	oteltrace.SpanFromContext(t.ctx).SetAttributes(attrs...)
}

func (t contextTracer) RecordError(err error) {
	span := oteltrace.SpanFromContext(t.ctx)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

func (t contextTracer) Context() context.Context { return t.ctx }

var _ tracing.Tracer = contextTracer{}
