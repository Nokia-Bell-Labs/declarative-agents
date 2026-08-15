// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package ollama

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

func TestChat_EmitsSemconvInferenceSpan(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(chatAPIHandler("hello", 100, 15))
	defer srv.Close()

	tr := tracing.NewRecordingTracer()
	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()), WithTracer(tr))
	require.NoError(t, err)

	msgs := []llm.Message{{Role: llm.User, Content: "hi"}}
	resp, err := a.Chat(context.Background(), msgs, llm.ChatOptions{Model: "llama3"})
	require.NoError(t, err)
	require.Equal(t, "hello", resp.Content)

	require.Len(t, tr.Spans, 1, "Chat should create exactly one span")
	s := tr.Spans[0]

	require.Equal(t, "chat llama3", s.Name)
	require.True(t, s.Completed, "span must be completed")

	require.Equal(t, "chat", s.Attrs["gen_ai.operation.name"])
	require.Equal(t, "ollama", s.Attrs["gen_ai.provider.name"])
	require.Equal(t, "llama3", s.Attrs["gen_ai.request.model"])

	require.Equal(t, int64(100), s.SetAttrs["gen_ai.usage.input_tokens"])
	require.Equal(t, int64(15), s.SetAttrs["gen_ai.usage.output_tokens"])
	require.Equal(t, "llama3", s.SetAttrs["gen_ai.response.model"])
}

func TestChat_ParentsTransportSpanFromCallerContext(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(chatAPIHandler("hello", 100, 15))
	defer srv.Close()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() { require.NoError(t, provider.Shutdown(context.Background())) }()
	tracer := provider.Tracer("ollama-parenting-test")
	agentCtx, agentSpan := tracer.Start(context.Background(), "invoke_agent")
	dispatchCtx, dispatchSpan := tracer.Start(agentCtx, "chat llama3")

	fallback := tracing.NewRecordingTracer()
	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()), WithTracer(fallback))
	require.NoError(t, err)
	_, err = a.Chat(dispatchCtx, []llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{Model: "llama3"})
	require.NoError(t, err)
	dispatchSpan.End()
	agentSpan.End()

	spans := recorder.Ended()
	require.Len(t, spans, 3)
	agent := spanNamed(t, spans, "invoke_agent")
	dispatch := childSpan(t, spans, agent.SpanContext().SpanID())
	transport := childSpan(t, spans, dispatch.SpanContext().SpanID())
	require.Equal(t, "chat llama3", dispatch.Name())
	require.Equal(t, "chat llama3", transport.Name())
	require.Equal(t, agent.SpanContext().TraceID(), transport.SpanContext().TraceID())
	require.Empty(t, fallback.Spans, "caller context must win without mutating the adapter fallback tracer")
}

func TestChat_SpanRecordsErrorOnHTTPFailure(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("overloaded"))
	}))
	defer srv.Close()

	tr := tracing.NewRecordingTracer()
	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()), WithTracer(tr))
	require.NoError(t, err)

	_, err = a.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{Model: "llama3"})
	require.Error(t, err)

	require.Len(t, tr.Spans, 1)
	s := tr.Spans[0]
	require.True(t, s.HasError, "span should record error")
	require.Equal(t, "503", s.SetAttrs["error.type"])
}

func TestChat_NilTracerDoesNotPanic(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(chatAPIHandler("ok", 10, 5))
	defer srv.Close()

	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.NoError(t, err)

	resp, err := a.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{Model: "llama3"})
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Content)
}

func TestNewAdapter_NilTracerNoops(t *testing.T) {
	t.Parallel()
	a, err := NewAdapter("http://127.0.0.1:11434", "llama3")
	require.NoError(t, err)
	require.NotNil(t, a)
}

func spanNamed(t *testing.T, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, span := range spans {
		if span.Name() == name {
			return span
		}
	}
	t.Fatalf("span %q not found", name)
	return nil
}

func childSpan(t *testing.T, spans []sdktrace.ReadOnlySpan, parentID oteltrace.SpanID) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, span := range spans {
		if span.Parent().SpanID() == parentID {
			return span
		}
	}
	t.Fatalf("child span of %s not found", parentID.String())
	return nil
}
