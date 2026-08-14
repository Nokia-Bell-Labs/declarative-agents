// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type systemConversationAssembler struct{ content string }

func (a systemConversationAssembler) AssembleMessages(
	history *modelllm.Conversation, _ *core.Registry, _ core.State,
) []modelllm.Message {
	return append(
		[]modelllm.Message{{Role: modelllm.System, Content: a.content}},
		history.Snapshot()...,
	)
}

type staticConversationReference struct {
	ref       string
	available bool
}

func (s staticConversationReference) ConversationReference() (string, bool) {
	return s.ref, s.available
}

func TestInvokeLLMDeltaCaptureIsolatesTwoTurns(t *testing.T) {
	t.Parallel()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() { require.NoError(t, provider.Shutdown(context.Background())) }()
	root := telemetry.TraceAdapter{
		T: telemetry.NewTraceFromProvider(provider, "invoke-delta-test", context.Background()),
	}
	client := &sequencedClient{responses: []modelllm.ChatResponse{
		{Content: "first answer"}, {Content: "second answer"},
	}}
	builder := &InvokeLLMBuilder{
		Client: client, History: modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		Registry: core.NewRegistry(), Assembler: systemConversationAssembler{content: "stable system"},
		Model: "test", ProviderName: "test", Tracer: tracing.NoopTracer{},
		Ctx: context.Background(), CaptureLevel: CaptureDelta,
	}

	for _, prompt := range []string{"first prompt", "second prompt"} {
		cmd := builder.Build(core.Result{Output: prompt})
		dispatch, done := root.Push("chat test")
		cmd.(core.TracerAware).SetTracer(dispatch)
		require.Equal(t, core.LLMResponded, cmd.Execute().Signal)
		done()
	}

	spans := recorder.Ended()
	require.Len(t, spans, 2)
	assertDeltaTurn(t, spans[0], "first prompt", "first answer", "second prompt")
	assertDeltaTurn(t, spans[1], "second prompt", "second answer", "first prompt")
	wantHash := renderedSystemPromptHash([]modelllm.Message{
		{Role: modelllm.System, Content: "stable system"},
	})
	require.Equal(t, wantHash, readOnlySpanAttrs(spans[0])["llm.system_prompt.hash"])
	require.Equal(t, wantHash, readOnlySpanAttrs(spans[1])["llm.system_prompt.hash"])
}

func TestDeltaCaptureRecordsOnlyDeltaContent(t *testing.T) {
	t.Parallel()
	invokeTracer := tracing.NewRecordingTracer()
	invokeSpan, invokeDone := invokeTracer.Push("chat")
	builder := &InvokeLLMBuilder{
		Client: fakeClient{}, History: modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		Registry: core.NewRegistry(), Assembler: conversationAssembler{},
		Model: "test", ProviderName: "test", Tracer: invokeSpan,
		Ctx: context.Background(), CaptureLevel: CaptureDelta,
	}
	require.Equal(t, core.LLMResponded, builder.Build(core.Result{Output: "secret prompt"}).Execute().Signal)
	invokeDone()
	require.Equal(t,
		modelllm.Message{Role: modelllm.User, Content: "secret prompt"},
		decodeMessage(t, invokeTracer.Spans[0].SetAttrs["llm.input.delta"]))
	require.Equal(t,
		modelllm.Message{Role: modelllm.Assistant, Content: `{"tool":"done","parameters":{"summary":"ok"}}`},
		decodeMessage(t, invokeTracer.Spans[0].SetAttrs["llm.output.delta"]))
	require.NotContains(t, invokeTracer.Spans[0].SetAttrs, "gen_ai.input.messages")
	require.NotContains(t, invokeTracer.Spans[0].SetAttrs, "gen_ai.output.messages")

	parseTracer := tracing.NewRecordingTracer()
	parseSpan, parseDone := parseTracer.Push("parse")
	raw := `{"tool":"done","parameters":{"summary":"secret response"}}`
	res := (&ParseResponseBuilder{
		Registry: core.NewRegistry(), Tracer: parseSpan, CaptureLevel: CaptureDelta,
	}).Build(core.Result{Output: raw}).Execute()
	parseDone()
	require.Equal(t, core.TaskCompleted, res.Signal)
	require.NotContains(t, parseTracer.Spans[0].SetAttrs, "llm.raw_output")
	require.Equal(t, int64(len(raw)), parseTracer.Spans[0].SetAttrs["raw_response_bytes"])
}

func TestCaptureOffRecordsNoContent(t *testing.T) {
	t.Parallel()
	invokeTracer := tracing.NewRecordingTracer()
	invokeSpan, invokeDone := invokeTracer.Push("chat")
	builder := &InvokeLLMBuilder{
		Client: fakeClient{}, History: modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		Registry: core.NewRegistry(), Assembler: conversationAssembler{},
		Model: "test", ProviderName: "test", Tracer: invokeSpan,
		Ctx: context.Background(), CaptureLevel: CaptureOff,
	}
	require.Equal(t, core.LLMResponded, builder.Build(core.Result{Output: "secret prompt"}).Execute().Signal)
	invokeDone()
	for _, key := range []string{
		"llm.input.delta", "llm.output.delta",
		"gen_ai.input.messages", "gen_ai.output.messages",
	} {
		require.NotContains(t, invokeTracer.Spans[0].SetAttrs, key)
	}

	parseTracer := tracing.NewRecordingTracer()
	parseSpan, parseDone := parseTracer.Push("parse")
	raw := `{"tool":"done","parameters":{"summary":"secret response"}}`
	res := (&ParseResponseBuilder{
		Registry: core.NewRegistry(), Tracer: parseSpan, CaptureLevel: CaptureOff,
	}).Build(core.Result{Output: raw}).Execute()
	parseDone()
	require.Equal(t, core.TaskCompleted, res.Signal)
	require.NotContains(t, parseTracer.Spans[0].SetAttrs, "llm.raw_output")
}

func TestInvokeLLMConversationReferenceIsOptional(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		provider ConversationReferenceProvider
		wantRef  string
	}{
		{name: "missing provider"},
		{name: "unavailable", provider: staticConversationReference{ref: "checkpoint:1"}},
		{name: "empty", provider: staticConversationReference{available: true}},
		{name: "available", provider: staticConversationReference{ref: "checkpoint:run/1", available: true}, wantRef: "checkpoint:run/1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tracer := tracing.NewRecordingTracer()
			span, done := tracer.Push("chat")
			builder := &InvokeLLMBuilder{
				Client: fakeClient{}, History: modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
				Registry: core.NewRegistry(), Assembler: conversationAssembler{},
				Model: "test", ProviderName: "test", Tracer: span,
				Ctx: context.Background(), CaptureLevel: CaptureOff,
				ConversationRefProvider: tc.provider,
			}
			require.Equal(t, core.LLMResponded, builder.Build(core.Result{Output: "prompt"}).Execute().Signal)
			done()
			got, exists := tracer.Spans[0].SetAttrs["llm.conversation.ref"]
			if tc.wantRef == "" {
				require.False(t, exists)
				return
			}
			require.Equal(t, tc.wantRef, got)
		})
	}
}

func TestRenderedSystemPromptHashTreatsAbsenceAsEmpty(t *testing.T) {
	t.Parallel()
	require.Equal(t,
		renderedSystemPromptHash(nil),
		renderedSystemPromptHash([]modelllm.Message{
			{Role: modelllm.User, Content: "ignored"},
		}))
	require.Equal(t,
		"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		renderedSystemPromptHash(nil))
}

func decodeMessage(t *testing.T, value interface{}) modelllm.Message {
	t.Helper()
	var message modelllm.Message
	require.NoError(t, json.Unmarshal([]byte(value.(string)), &message))
	return message
}

func decodeMessages(t *testing.T, value interface{}) []modelllm.Message {
	t.Helper()
	var messages []modelllm.Message
	require.NoError(t, json.Unmarshal([]byte(value.(string)), &messages))
	return messages
}

func assertDeltaTurn(
	t *testing.T, span sdktrace.ReadOnlySpan, input, output, excluded string,
) {
	t.Helper()
	attrs := readOnlySpanAttrs(span)
	require.Equal(t,
		modelllm.Message{Role: modelllm.User, Content: input},
		decodeMessage(t, attrs["llm.input.delta"]))
	require.Equal(t,
		modelllm.Message{Role: modelllm.Assistant, Content: output},
		decodeMessage(t, attrs["llm.output.delta"]))
	require.NotContains(t, attrs["llm.input.delta"], excluded)
	require.NotContains(t, attrs["llm.output.delta"], excluded)
	require.NotContains(t, attrs, "gen_ai.input.messages")
	require.NotContains(t, attrs, "gen_ai.output.messages")
}
