// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package tracing

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestContextChildParentsOnTheCallerSpan(t *testing.T) {
	t.Parallel()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	ctx, parent := provider.Tracer("test").Start(context.Background(), "invoke_llm")

	childCtx, child, done := ContextChild(ctx, NoopTracer{}, "scope", "chat m", attribute.String("k", "v"))
	child.SetAttributes(attribute.Int("n", 1))
	child.RecordError(errors.New("boom"))
	done()
	parent.End()

	spans := exporter.GetSpans()
	require.Len(t, spans, 2)
	require.Equal(t, "chat m", spans[0].Name)
	require.Equal(t, parent.SpanContext().SpanID(), spans[0].Parent.SpanID())
	require.Equal(t, codes.Error, spans[0].Status.Code)
	require.Contains(t, spans[0].Attributes, attribute.String("k", "v"))
	require.Contains(t, spans[0].Attributes, attribute.Int("n", 1))
	require.NotEqual(t, parent.SpanContext().SpanID(), childCtxSpanID(childCtx))
}

func TestContextChildFallsBackWithoutACallerSpan(t *testing.T) {
	t.Parallel()
	recorder := NewRecordingTracer()

	_, child, done := ContextChild(context.Background(), recorder, "scope", "chat m")
	child.SetAttributes(attribute.Int("n", 1))
	done()

	require.Len(t, recorder.Spans, 1)
	require.Equal(t, "chat m", recorder.Spans[0].Name)
	require.True(t, recorder.Spans[0].Completed)

	_, noop, end := ContextChild(nil, nil, "scope", "chat m") //nolint:staticcheck // a nil context is tolerated
	end()
	require.IsType(t, NoopTracer{}, noop)
}

func childCtxSpanID(ctx context.Context) oteltrace.SpanID {
	return oteltrace.SpanFromContext(ctx).SpanContext().SpanID()
}
