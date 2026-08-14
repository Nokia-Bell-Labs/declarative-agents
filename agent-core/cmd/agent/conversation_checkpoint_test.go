// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
)

func TestResetHistoryFactoryRestartsFromCheckpointReference(t *testing.T) {
	t.Parallel()
	messages := []modelllm.Message{
		{Role: modelllm.User, Content: "prior user"},
		{Role: modelllm.Assistant, Content: "prior assistant"},
	}
	checkpoint := checkpointWithConversation(t, "reset-run", messages)
	state := checkpointAgentState(checkpoint)
	state.conversation.Restore(messages)

	builder, err := resetHistoryFactory(state)(catalog.ToolDef{}, nil)
	require.NoError(t, err)
	result := builder.Build(core.Result{}).Execute()
	require.NotContains(t, result.Receipt, "prior user")
	require.NotContains(t, result.Receipt, "prior assistant")

	fresh := checkpointAgentState(checkpoint)
	builder, err = resetHistoryFactory(fresh)(catalog.ToolDef{}, nil)
	require.NoError(t, err)
	reverser, ok := builder.(core.Reverser)
	require.True(t, ok)
	undo := reverser.BuildReverser().Undo(core.Result{Receipt: result.Receipt})
	require.Equal(t, core.ToolDone, undo.Signal, undo.Output)
	require.Equal(t, messages, fresh.conversation.History())
}

func TestInvokeLLMRestartsThroughCompositionCheckpointPorts(t *testing.T) {
	t.Parallel()
	messages := []modelllm.Message{{Role: modelllm.User, Content: "checkpoint secret"}}
	checkpoint := checkpointWithConversation(t, "invoke-run", messages)
	state := checkpointAgentState(checkpoint)
	state.conversation.Restore(messages)
	provider, resolver := llmConversationReferencePorts(state)
	builder := &toollm.InvokeLLMBuilder{
		Client: checkpointChatClient{}, History: state.conversation,
		Registry: core.NewRegistry(), Assembler: checkpointAssembler{},
		Model: "test", Tracer: tracing.NoopTracer{}, Ctx: context.Background(),
		ConversationRefProvider: provider, ConversationRefResolver: resolver,
	}

	result := builder.Build(core.Result{Output: "next"}).Execute()
	require.Equal(t, core.LLMResponded, result.Signal)
	require.NotContains(t, result.Receipt, "checkpoint secret")
	require.NotContains(t, result.Receipt, "next")

	fresh := checkpointAgentState(checkpoint)
	freshProvider, freshResolver := llmConversationReferencePorts(fresh)
	freshBuilder := *builder
	freshBuilder.History = fresh.conversation
	freshBuilder.ConversationRefProvider = freshProvider
	freshBuilder.ConversationRefResolver = freshResolver
	undo := freshBuilder.BuildReverser().Undo(core.Result{Receipt: result.Receipt})
	require.Equal(t, core.ToolDone, undo.Signal, undo.Output)
	require.Equal(t, messages, fresh.conversation.History())
}

func TestRequestLocalStateDoesNotExposeHostCheckpointReferences(t *testing.T) {
	t.Parallel()
	host := checkpointAgentState(checkpointWithConversation(
		t, "host-run", []modelllm.Message{{Role: modelllm.User, Content: "host"}},
	))
	local := requestLocalState(host, core.NewRegistry())
	provider, resolver := llmConversationReferencePorts(local)
	require.Nil(t, provider)
	require.Nil(t, resolver)
}

func checkpointWithConversation(
	t *testing.T,
	runID string,
	messages []modelllm.Message,
) *core.InMemoryCheckpoint {
	t.Helper()
	snapshot, err := json.Marshal(messages)
	require.NoError(t, err)
	checkpoint := core.NewInMemoryCheckpoint(runID)
	require.NoError(t, checkpoint.Save(core.Position{
		Snapshot: core.AgentSnapshot{Conversation: snapshot},
	}, core.Execution{{
		Result: core.ResultDigest{
			RedactionVersion: core.OutputRedactionVersion1,
			RedactionStatus:  core.OutputRedactionApplied,
		},
	}}))
	return checkpoint
}

func checkpointAgentState(checkpoint core.Checkpoint) *agentState {
	return newAgentState(runtimeConfig{}, agentStateDeps{
		Registry: core.NewRegistry(), Tracer: tracing.NoopTracer{},
		Checkpoint: checkpoint, Ctx: context.Background(),
	})
}

type checkpointChatClient struct{}

func (checkpointChatClient) Chat(
	context.Context,
	[]modelllm.Message,
	modelllm.ChatOptions,
) (modelllm.ChatResponse, error) {
	return modelllm.ChatResponse{Content: "answer"}, nil
}

type checkpointAssembler struct{}

func (checkpointAssembler) AssembleMessages(
	conversation *modelllm.Conversation,
	_ *core.Registry,
	_ core.State,
) []modelllm.Message {
	return conversation.Snapshot()
}
