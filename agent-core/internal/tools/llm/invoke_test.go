// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
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
