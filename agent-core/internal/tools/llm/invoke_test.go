// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
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

func (fakeClient) ListModels(context.Context) ([]modelllm.ModelInfo, error) {
	return nil, nil
}

// capturingClient records the ChatOptions of the last Chat call so tests can
// assert the decoding parameters that reach the model boundary.
type capturingClient struct{ opts modelllm.ChatOptions }

func (c *capturingClient) Chat(_ context.Context, _ []modelllm.Message, opts modelllm.ChatOptions) (modelllm.ChatResponse, error) {
	c.opts = opts
	return modelllm.ChatResponse{Content: `{"tool":"done","parameters":{"summary":"ok"}}`}, nil
}

func (*capturingClient) ListModels(context.Context) ([]modelllm.ModelInfo, error) {
	return nil, nil
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
