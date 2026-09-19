// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// ContextChild opens a child span named name under the span ctx carries, from
// the tracer provider that span came from. With no active span in ctx it falls
// back to fallback.Push, so a direct call without a caller span still records
// one. It returns the context carrying the child, the child's Tracer, and the
// function that ends it. No state outside the returned values is mutated, so
// concurrent calls through one client stay independent.
func ContextChild(
	ctx context.Context, fallback Tracer, instrumentation, name string, attrs ...attribute.KeyValue,
) (context.Context, Tracer, func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	parent := oteltrace.SpanFromContext(ctx)
	if parent.SpanContext().IsValid() {
		tracer := parent.TracerProvider().Tracer(instrumentation)
		child, done := contextTracer{tracer: tracer, ctx: ctx}.Push(name, attrs...)
		return child.Context(), child, done
	}
	if fallback == nil {
		return ctx, NoopTracer{}, func() {}
	}
	child, done := fallback.Push(name, attrs...)
	span := oteltrace.SpanFromContext(child.Context())
	if span.SpanContext().IsValid() {
		ctx = oteltrace.ContextWithSpan(ctx, span)
	}
	return ctx, child, done
}

// contextTracer is a request-scoped Tracer over an OTel tracer and the context
// holding its current span.
type contextTracer struct {
	tracer oteltrace.Tracer
	ctx    context.Context
}

func (t contextTracer) Push(name string, attrs ...attribute.KeyValue) (Tracer, func()) {
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
