// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

type recordingAssembler struct {
	states []core.State
}

func (r *recordingAssembler) AssembleMessages(_ *modelllm.Conversation, registry *core.Registry, state core.State) []modelllm.Message {
	r.states = append(r.states, state)
	_ = registry.Manifest(state)
	return []modelllm.Message{{Role: modelllm.System, Content: "system"}}
}

type fakeClient struct{}

func (fakeClient) Chat(context.Context, []modelllm.Message, modelllm.ChatOptions) (modelllm.ChatResponse, error) {
	return modelllm.ChatResponse{Content: `{"tool":"done","parameters":{"summary":"ok"}}`}, nil
}

// capturingClient records the ChatOptions of the last Chat call so tests can
// assert the decoding parameters that reach the model boundary.
type capturingClient struct{ opts modelllm.ChatOptions }

func (c *capturingClient) Chat(_ context.Context, _ []modelllm.Message, opts modelllm.ChatOptions) (modelllm.ChatResponse, error) {
	c.opts = opts
	return modelllm.ChatResponse{Content: `{"tool":"done","parameters":{"summary":"ok"}}`}, nil
}

type countingClient struct{ calls int }

func (c *countingClient) Chat(
	context.Context, []modelllm.Message, modelllm.ChatOptions,
) (modelllm.ChatResponse, error) {
	c.calls++
	return modelllm.ChatResponse{}, nil
}

type conversationAssembler struct{}

func (conversationAssembler) AssembleMessages(history *modelllm.Conversation, _ *core.Registry, _ core.State) []modelllm.Message {
	return history.Snapshot()
}

type sequencedClient struct {
	responses []modelllm.ChatResponse
	spanIDs   []oteltrace.SpanID
}

func (c *sequencedClient) Chat(
	ctx context.Context, _ []modelllm.Message, _ modelllm.ChatOptions,
) (modelllm.ChatResponse, error) {
	c.spanIDs = append(c.spanIDs, oteltrace.SpanFromContext(ctx).SpanContext().SpanID())
	response := c.responses[0]
	c.responses = c.responses[1:]
	return response, nil
}

func floatPtr(f float64) *float64 { return &f }
func intPtr(i int) *int           { return &i }

func TestDecodeInvokeLLMConfigParsesTemperatureAndSeed(t *testing.T) {
	t.Parallel()
	def := catalog.ToolDef{Name: "invoke_llm", Config: map[string]interface{}{
		"model": "qwen2.5:7b", "manifest_state": "Composing",
		"temperature": 0.7, "seed": 20260705,
	}}

	cfg, err := DecodeInvokeLLMConfig(def)

	require.NoError(t, err)
	require.NotNil(t, cfg.Temperature)
	require.InDelta(t, 0.7, *cfg.Temperature, 1e-9)
	require.NotNil(t, cfg.Seed)
	require.Equal(t, 20260705, *cfg.Seed)
}

func TestDecodeInvokeLLMConfigLeavesTemperatureAndSeedUnset(t *testing.T) {
	t.Parallel()
	def := catalog.ToolDef{Name: "invoke_llm", Config: map[string]interface{}{
		"model": "qwen2.5:7b", "manifest_state": "Composing",
	}}

	cfg, err := DecodeInvokeLLMConfig(def)

	require.NoError(t, err)
	require.Nil(t, cfg.Temperature)
	require.Nil(t, cfg.Seed)
}

func TestResolveTemperatureAndSeedApplyDeterministicDefaults(t *testing.T) {
	t.Parallel()
	require.Equal(t, defaultTemperature, resolveTemperature(catalog.LLMToolConfig{}))
	require.Equal(t, defaultSeed, resolveSeed(catalog.LLMToolConfig{}))
	require.InDelta(t, 0.7, resolveTemperature(catalog.LLMToolConfig{Temperature: floatPtr(0.7)}), 1e-9)
	require.Equal(t, 20260705, resolveSeed(catalog.LLMToolConfig{Seed: intPtr(20260705)}))
}

func TestInvokeLLMPassesConfiguredTemperatureAndSeed(t *testing.T) {
	t.Parallel()
	client := &capturingClient{}
	tracer := tracing.NewRecordingTracer()
	span, done := tracer.Push("chat")
	defer done()
	builder := &InvokeLLMBuilder{
		Client: client, History: modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		Registry: core.NewRegistry(), Assembler: &recordingAssembler{}, State: "Composing",
		Model: "test", ProviderName: "test", Tracer: span, Ctx: context.Background(),
		Temperature: 0.7, Seed: 20260705,
	}

	res := builder.Build(core.Result{Output: "prompt"}).Execute()

	require.Equal(t, core.LLMResponded, res.Signal)
	require.InDelta(t, 0.7, client.opts.Temperature, 1e-9)
	require.Equal(t, 20260705, client.opts.Seed)
	require.Equal(t, int64(20260705), tracer.Spans[0].SetAttrs["gen_ai.request.seed"])
	require.Contains(t, tracer.Spans[0].SetAttrs, "gen_ai.request.temperature")
}

func TestInvokeLLMDefaultsPreserveDeterministicDecoding(t *testing.T) {
	t.Parallel()
	client := &capturingClient{}
	cfg := catalog.LLMToolConfig{}
	builder := &InvokeLLMBuilder{
		Client: client, History: modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		Registry: core.NewRegistry(), Assembler: &recordingAssembler{}, State: "Composing",
		Model: "test", ProviderName: "test", Tracer: tracing.NoopTracer{}, Ctx: context.Background(),
		Temperature: resolveTemperature(cfg), Seed: resolveSeed(cfg),
	}

	builder.Build(core.Result{Output: "prompt"}).Execute()

	require.InDelta(t, 0.0, client.opts.Temperature, 1e-9)
	require.Equal(t, 42, client.opts.Seed)
}

func TestInvokeLLMDispatchTracerIsolatesTwoTurns(t *testing.T) {
	t.Parallel()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() { require.NoError(t, provider.Shutdown(context.Background())) }()
	root := telemetry.TraceAdapter{
		T: telemetry.NewTraceFromProvider(provider, "invoke-turn-test", context.Background()),
	}
	client := &sequencedClient{responses: []modelllm.ChatResponse{
		{Content: "one", TokensIn: 11, TokensOut: 3},
		{Content: "second response", TokensIn: 22, TokensOut: 4},
	}}
	builder := &InvokeLLMBuilder{
		Client: client, History: modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		Registry: core.NewRegistry(), Assembler: conversationAssembler{}, State: "Composing",
		Model: "test", ProviderName: "test", Tracer: tracing.NoopTracer{},
		Ctx: context.Background(), CaptureLevel: CaptureFull, ContextLimit: 1000,
	}

	for _, prompt := range []string{"first prompt", "second prompt"} {
		cmd := builder.Build(core.Result{Output: prompt})
		aware, ok := cmd.(core.TracerAware)
		require.True(t, ok)
		dispatch, done := root.Push("chat test")
		aware.SetTracer(dispatch)
		require.Equal(t, core.LLMResponded, cmd.Execute().Signal)
		done()
	}

	spans := recorder.Ended()
	require.Len(t, spans, 2)
	require.Equal(t, spans[0].SpanContext().SpanID(), client.spanIDs[0])
	require.Equal(t, spans[1].SpanContext().SpanID(), client.spanIDs[1])

	firstAttrs := readOnlySpanAttrs(spans[0])
	secondAttrs := readOnlySpanAttrs(spans[1])
	require.Contains(t, firstAttrs["gen_ai.input.messages"], "first prompt")
	require.NotContains(t, firstAttrs["gen_ai.input.messages"], "second prompt")
	require.Contains(t, secondAttrs["gen_ai.input.messages"], "second prompt")
	require.Equal(t, []modelllm.Message{
		{Role: modelllm.User, Content: "first prompt"},
	}, decodeMessages(t, firstAttrs["gen_ai.input.messages"]))
	require.Equal(t, []modelllm.Message{
		{Role: modelllm.Assistant, Content: "one"},
	}, decodeMessages(t, firstAttrs["gen_ai.output.messages"]))
	require.Equal(t, []modelllm.Message{
		{Role: modelllm.User, Content: "first prompt"},
		{Role: modelllm.Assistant, Content: "one"},
		{Role: modelllm.User, Content: "second prompt"},
	}, decodeMessages(t, secondAttrs["gen_ai.input.messages"]))
	require.Equal(t, []modelllm.Message{
		{Role: modelllm.Assistant, Content: "second response"},
	}, decodeMessages(t, secondAttrs["gen_ai.output.messages"]))
	require.Equal(t, int64(1000), firstAttrs["context.limit"])
	require.Equal(t, int64(1000), secondAttrs["context.limit"])
	require.Less(t, firstAttrs["context.estimated_tokens"], secondAttrs["context.estimated_tokens"])
	require.Equal(t, int64(11), firstAttrs["gen_ai.usage.input_tokens"])
	require.Equal(t, int64(22), secondAttrs["gen_ai.usage.input_tokens"])
	require.Equal(t, int64(3), eventAttrs(t, spans[0], "chat.request_done")["response_content_len"])
	require.Equal(t, int64(15), eventAttrs(t, spans[1], "chat.request_done")["response_content_len"])
	require.Equal(t, expectedInvokeEvents(), spanEventNames(spans[0]))
	require.Equal(t, expectedInvokeEvents(), spanEventNames(spans[1]))
}

func TestParseResponseDispatchTracerCapturesRawOutput(t *testing.T) {
	t.Parallel()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() { require.NoError(t, provider.Shutdown(context.Background())) }()
	root := telemetry.TraceAdapter{
		T: telemetry.NewTraceFromProvider(provider, "parse-dispatch-test", context.Background()),
	}
	raw := `{"tool":"done","parameters":{"summary":"complete"}}`
	cmd := (&ParseResponseBuilder{
		Registry: core.NewRegistry(), Tracer: tracing.NoopTracer{}, CaptureLevel: CaptureFull,
	}).Build(core.Result{Output: raw})
	aware, ok := cmd.(core.TracerAware)
	require.True(t, ok)
	dispatch, done := root.Push("execute_tool parse_response")
	aware.SetTracer(dispatch)

	res := cmd.Execute()
	done()

	require.Equal(t, core.TaskCompleted, res.Signal)
	spans := recorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, raw, readOnlySpanAttrs(spans[0])["llm.raw_output"])
}

func TestInvokeLLMUsesRuntimeStateForManifest(t *testing.T) {
	t.Parallel()
	assembler := &recordingAssembler{}
	builder := &InvokeLLMBuilder{
		Client: fakeClient{}, History: modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		Registry: core.NewRegistry(), Assembler: assembler, State: "Configured",
		Model: "test", ProviderName: "test", Tracer: tracing.NoopTracer{}, Ctx: context.Background(),
	}

	res := builder.Build(core.Result{State: "Composing", Output: "prompt"}).Execute()

	require.Equal(t, core.LLMResponded, res.Signal)
	require.Equal(t, []core.State{"Composing"}, assembler.states)
}

// TestNewInvokeLLMBuilderDoesNotProbeAtRegistration pins GH-1375: constructing
// the invoke_llm builder must not probe the backend, so an unreachable Ollama
// never fails tool registration. Profiles may declare a preflight transition;
// otherwise dispatch reports the unreachable backend as CommandError.
func TestNewInvokeLLMBuilderDoesNotProbeAtRegistration(t *testing.T) {
	t.Parallel()
	def := catalog.ToolDef{Name: "invoke_llm", Config: map[string]interface{}{
		"provider":       "ollama",
		"provider_url":   "http://127.0.0.1:1",
		"model":          "qwen2.5:7b",
		"manifest_state": "Composing",
	}}

	builder, err := NewInvokeLLMBuilder(def, InvokeLLMFactoryDeps{Ctx: context.Background()})

	require.NoError(t, err)
	require.NotNil(t, builder)
}

func TestInvokeLLMFactoryWiresContextLimitBeforeProviderCall(t *testing.T) {
	t.Parallel()
	def := catalog.ToolDef{Name: "invoke_llm", Config: map[string]interface{}{
		"provider": "ollama", "provider_url": "http://127.0.0.1:1",
		"model": "qwen2.5:7b", "manifest_state": "Composing", "context_limit": 1,
	}}
	builder, err := NewInvokeLLMBuilder(def, InvokeLLMFactoryDeps{
		History:  modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		Registry: core.NewRegistry(), Tracer: tracing.NoopTracer{},
		Ctx: context.Background(),
	})
	require.NoError(t, err)
	require.Equal(t, 1, builder.ContextLimit)
	client := &countingClient{}
	builder.Client = client

	result := builder.Build(core.Result{
		State: "Composing", Output: "this prompt exceeds one estimated token",
	}).Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.ErrorContains(t, result.Err, "context window exhaustion")
	require.Zero(t, client.calls)
}

func TestInvokeLLMUnreachableBackendIsRoutableAtDispatch(t *testing.T) {
	t.Parallel()
	def := catalog.ToolDef{Name: "invoke_llm", Config: map[string]interface{}{
		"provider":       "ollama",
		"provider_url":   "http://127.0.0.1:1",
		"model":          "qwen2.5:7b",
		"manifest_state": "Composing",
	}}
	builder, err := NewInvokeLLMBuilder(def, InvokeLLMFactoryDeps{
		History:  modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		Registry: core.NewRegistry(),
		Tracer:   tracing.NoopTracer{},
		Ctx:      context.Background(),
	})
	require.NoError(t, err)

	res := builder.Build(core.Result{State: "Composing", Output: "hello"}).Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "ollama chat request failed")
}

func TestInvokeLLMFallsBackToConfiguredManifestState(t *testing.T) {
	t.Parallel()
	assembler := &recordingAssembler{}
	builder := &InvokeLLMBuilder{
		Client: fakeClient{}, History: modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		Registry: core.NewRegistry(), Assembler: assembler, State: "Configured",
		Model: "test", ProviderName: "test", Tracer: tracing.NoopTracer{}, Ctx: context.Background(),
	}

	res := builder.Build(core.Result{Output: "prompt"}).Execute()

	require.Equal(t, core.LLMResponded, res.Signal)
	require.Equal(t, []core.State{"Configured"}, assembler.states)
}

func readOnlySpanAttrs(span sdktrace.ReadOnlySpan) map[string]interface{} {
	attrs := make(map[string]interface{}, len(span.Attributes()))
	for _, attr := range span.Attributes() {
		attrs[string(attr.Key)] = tracing.AttrValue(attr.Value)
	}
	return attrs
}

func eventAttrs(t *testing.T, span sdktrace.ReadOnlySpan, name string) map[string]interface{} {
	t.Helper()
	for _, event := range span.Events() {
		if event.Name == name {
			attrs := make(map[string]interface{}, len(event.Attributes))
			for _, attr := range event.Attributes {
				attrs[string(attr.Key)] = tracing.AttrValue(attr.Value)
			}
			return attrs
		}
	}
	t.Fatalf("event %q not found", name)
	return nil
}

func spanEventNames(span sdktrace.ReadOnlySpan) string {
	names := make([]string, 0, len(span.Events()))
	for _, event := range span.Events() {
		names = append(names, event.Name)
	}
	return strings.Join(names, ",")
}

func expectedInvokeEvents() string {
	return "history.user_appended,prompt.assembled,chat.request_start,chat.request_done,history.assistant_appended"
}
