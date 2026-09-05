// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package cohere implements llm.Client for Cohere v2 Chat.
package cohere

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry/genai"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

const (
	apiKeyEnvironment   = "COHERE_API_KEY"
	instrumentationName = "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm/cohere"
)

// APIKeyLookup resolves the process credential when Chat executes.
type APIKeyLookup func() (string, bool)

// Option configures an Adapter without performing I/O.
type Option func(*Adapter)

// Adapter wraps Cohere v2 Chat and implements llm.Client.
type Adapter struct {
	baseURL string
	model   string
	client  *http.Client
	tracer  tracing.Tracer
	apiKey  APIKeyLookup
}

var _ llm.Client = (*Adapter)(nil)

// NewAdapter creates an inert adapter. Chat owns network and credential I/O.
func NewAdapter(baseURL, model string, opts ...Option) (*Adapter, error) {
	a := &Adapter{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 5 * time.Minute},
		tracer:  tracing.NoopTracer{},
		apiKey:  environmentAPIKey,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a, nil
}

// Model returns the configured Cohere model.
func (a *Adapter) Model() string { return a.model }

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(a *Adapter) { a.client = client }
}

// WithTracer sets the fallback tracer used when Chat has no caller span.
func WithTracer(tracer tracing.Tracer) Option {
	return func(a *Adapter) { a.tracer = tracer }
}

// WithAPIKeyLookup replaces the lazy process credential lookup.
func WithAPIKeyLookup(lookup APIKeyLookup) Option {
	return func(a *Adapter) { a.apiKey = lookup }
}

func environmentAPIKey() (string, bool) {
	//nolint:forbidigo // srd048 R3: Cohere bearer credentials stay outside ToolDef configuration and resolve only at Chat.
	value, ok := os.LookupEnv(apiKeyEnvironment)
	value = strings.TrimSpace(value)
	return value, ok && value != ""
}

func (a *Adapter) chatSpan(ctx context.Context, model string) (context.Context, tracing.Tracer, func()) {
	attrs := genai.InferenceAttrs(genai.ProviderCohere, model, serverAddress(a.baseURL))
	if port := serverPort(a.baseURL); port > 0 {
		attrs = append(attrs, genai.AttrServerPort.Int(port))
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

func serverAddress(rawURL string) string {
	if parsed, err := url.Parse(rawURL); err == nil {
		return parsed.Host
	}
	return ""
}

func serverPort(rawURL string) int {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Port() == "" {
		return 0
	}
	var port int
	if n, err := fmt.Sscanf(parsed.Port(), "%d", &port); err != nil || n != 1 {
		return 0
	}
	return port
}

func contextWithTracerSpan(ctx context.Context, tracer tracing.Tracer) context.Context {
	span := oteltrace.SpanFromContext(tracer.Context())
	if span.SpanContext().IsValid() {
		return oteltrace.ContextWithSpan(ctx, span)
	}
	return ctx
}

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
