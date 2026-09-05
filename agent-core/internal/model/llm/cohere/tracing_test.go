// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package cohere

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestChat_Tracing(t *testing.T) {
	t.Parallel()
	t.Run("attributes", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeResponse(t, w, []contentBlock{{Type: "text", Text: "ok"}}, 19, 7)
		}))
		defer server.Close()
		tracer := tracing.NewRecordingTracer()
		adapter, err := NewAdapter(server.URL, "command-r7b-12-2024",
			WithHTTPClient(server.Client()), WithAPIKeyLookup(staticKey("key")), WithTracer(tracer))
		require.NoError(t, err)

		_, err = adapter.Chat(context.Background(), nil, llm.ChatOptions{Model: "command-r7b-12-2024"})
		require.NoError(t, err)
		require.Len(t, tracer.Spans, 1)
		span := tracer.Spans[0]
		require.Equal(t, "chat command-r7b-12-2024", span.Name)
		require.Equal(t, "chat", span.Attrs["gen_ai.operation.name"])
		require.Equal(t, "cohere", span.Attrs["gen_ai.provider.name"])
		require.Equal(t, "command-r7b-12-2024", span.Attrs["gen_ai.request.model"])
		require.NotEmpty(t, span.Attrs["server.address"])
		require.NotZero(t, span.Attrs["server.port"])
		require.Equal(t, int64(19), span.SetAttrs["gen_ai.usage.input_tokens"])
		require.Equal(t, int64(7), span.SetAttrs["gen_ai.usage.output_tokens"])
		require.Equal(t, "command-r7b-12-2024", span.SetAttrs["gen_ai.response.model"])
	})
	t.Run("error type", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()
		tracer := tracing.NewRecordingTracer()
		adapter, err := NewAdapter(server.URL, "model",
			WithHTTPClient(server.Client()), WithAPIKeyLookup(staticKey("key")), WithTracer(tracer))
		require.NoError(t, err)

		_, err = adapter.Chat(context.Background(), nil, llm.ChatOptions{Model: "model"})
		require.Error(t, err)
		require.Len(t, tracer.Spans, 1)
		require.True(t, tracer.Spans[0].HasError)
		require.Equal(t, "503", tracer.Spans[0].SetAttrs["error.type"])
	})
}

func TestChat_ParentsTransportSpanFromCallerContext(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeResponse(t, w, []contentBlock{{Type: "text", Text: "ok"}}, 1, 1)
	}))
	defer server.Close()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() { require.NoError(t, provider.Shutdown(context.Background())) }()
	tracer := provider.Tracer("cohere-parenting-test")
	agentCtx, agentSpan := tracer.Start(context.Background(), "invoke_agent")
	dispatchCtx, dispatchSpan := tracer.Start(agentCtx, "chat command-r7b-12-2024")

	fallback := tracing.NewRecordingTracer()
	adapter, err := NewAdapter(server.URL, "command-r7b-12-2024",
		WithHTTPClient(server.Client()), WithAPIKeyLookup(staticKey("key")), WithTracer(fallback))
	require.NoError(t, err)
	_, err = adapter.Chat(dispatchCtx, nil, llm.ChatOptions{Model: "command-r7b-12-2024"})
	require.NoError(t, err)
	dispatchSpan.End()
	agentSpan.End()

	spans := recorder.Ended()
	require.Len(t, spans, 3)
	agent := findSpan(t, spans, "invoke_agent")
	dispatch := findChild(t, spans, agent.SpanContext().SpanID())
	transport := findChild(t, spans, dispatch.SpanContext().SpanID())
	require.Equal(t, "chat command-r7b-12-2024", transport.Name())
	require.Equal(t, agent.SpanContext().TraceID(), transport.SpanContext().TraceID())
	require.Empty(t, fallback.Spans)
}

func findSpan(t *testing.T, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, span := range spans {
		if span.Name() == name {
			return span
		}
	}
	t.Fatalf("span %q not found", name)
	return nil
}

func findChild(t *testing.T, spans []sdktrace.ReadOnlySpan, parent oteltrace.SpanID) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, span := range spans {
		if span.Parent().SpanID() == parent {
			return span
		}
	}
	t.Fatalf("child span of %s not found", parent)
	return nil
}
