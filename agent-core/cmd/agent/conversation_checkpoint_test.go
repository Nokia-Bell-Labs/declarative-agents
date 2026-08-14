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
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/validation"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
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

func TestValidationStateRestartsThroughCompositionDomainPorts(t *testing.T) {
	t.Parallel()
	prior := &validation.SpecState{
		Directory:       "/persisted-source",
		TargetDirectory: "/persisted-target",
		SuitePaths:      []string{"suite.yaml"},
		Corpus:          &spec.Corpus{RootDir: "/persisted-source"},
		Findings:        []spec.Finding{{Level: "warning", Message: "prior"}},
		HasErrors:       true,
		CorpusOptional:  true,
	}
	origin := checkpointAgentState(core.NoopCheckpoint{})
	origin.validation = prior
	domain, err := origin.snapshotDomain()
	require.NoError(t, err)

	checkpoint := core.NewInMemoryCheckpoint("validation-run")
	require.NoError(t, checkpoint.Save(
		core.Position{Snapshot: core.AgentSnapshot{Domain: domain}},
		core.Execution{{
			Result: core.ResultDigest{
				RedactionVersion: core.OutputRedactionVersion1,
				RedactionStatus:  core.OutputRedactionApplied,
			},
		}},
	))

	active := checkpointAgentState(checkpoint)
	active.validation = prior
	provider, resolver := validationReferencePorts(active)
	result := (&validation.ValidateSpecsBuilder{
		ToolName: "audit_specs", VS: prior,
		ReferenceProvider: provider, SnapshotResolver: resolver,
	}).Build(core.Result{}).Execute()
	require.NotEqual(t, core.CommandError, result.Signal, result.Output)
	require.NotEmpty(t, result.Receipt)

	fresh := &validation.SpecState{}
	undo := (&validation.ValidateSpecsBuilder{
		ToolName: "audit_specs", VS: fresh, SnapshotResolver: resolver,
	}).BuildReverser().Undo(core.Result{Receipt: result.Receipt})
	require.Equal(t, core.ToolDone, undo.Signal, undo.Output)
	require.Equal(t, "/persisted-source", fresh.Directory)
	require.Equal(t, "/persisted-target", fresh.TargetDirectory)
	require.Equal(t, prior.SuitePaths, fresh.SuitePaths)
	require.Equal(t, prior.Corpus, fresh.Corpus)
	require.Equal(t, []spec.Finding{{Level: "warning", Message: "prior"}}, fresh.Findings)
	require.True(t, fresh.HasErrors)
	require.True(t, fresh.CorpusOptional)
}

func TestValidationFactoryWiresSharedStateAndDomainPorts(t *testing.T) {
	t.Parallel()
	state := checkpointAgentState(core.NewInMemoryCheckpoint("validation-run"))
	builtins := toolregistry.NewBuiltinRegistry()
	registerBuiltinFactories(builtins, state, map[string]bool{"validate_specs": true})
	factory, ok := builtins.Resolve("validate_specs")
	require.True(t, ok)

	builder, err := factory(catalog.ToolDef{Name: "audit_specs"}, nil)
	require.NoError(t, err)
	validationBuilder := builder.(*validation.ValidateSpecsBuilder)
	require.Equal(t, "audit_specs", validationBuilder.ToolName)
	require.Same(t, state.validation, validationBuilder.VS)
	require.NotNil(t, validationBuilder.ReferenceProvider)
	require.NotNil(t, validationBuilder.SnapshotResolver)
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
