// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestResetHistory(t *testing.T) {
	history := modelllm.NewConversation(nil, "", modelllm.ChatOptions{})
	history.Append(modelllm.Message{Role: modelllm.User, Content: "hello"})
	history.Append(modelllm.Message{Role: modelllm.Assistant, Content: "hi"})
	assert.Equal(t, 2, history.Len())

	builder := &ResetHistoryBuilder{History: history, Tracer: noopTracer()}
	cmd := builder.Build(core.Result{})
	require.Implements(t, (*core.SerialDispatchOnly)(nil), cmd)
	require.Equal(t, "reset_history", cmd.Name())
	res := cmd.Execute()

	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Equal(t, 0, history.Len())
}

func TestResetHistory_UndoRestoresPreviousMessages(t *testing.T) {
	history := modelllm.NewConversation(nil, "", modelllm.ChatOptions{})
	history.Append(modelllm.Message{Role: modelllm.User, Content: "hello"})
	history.Append(modelllm.Message{Role: modelllm.Assistant, Content: "hi"})

	builder := &ResetHistoryBuilder{History: history, Tracer: noopTracer()}
	cmd := builder.Build(core.Result{})
	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal)
	require.Equal(t, 0, history.Len())
	require.NotContains(t, res.Receipt, "hello")
	require.NotContains(t, res.Receipt, "hi")
	require.NotContains(t, res.Receipt, `"role"`)
	require.NotContains(t, res.Receipt, `"content"`)

	undo := cmd.Undo(core.Result{Receipt: res.Receipt})
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, 2, history.Len())
	require.Equal(t, "hello", history.History()[0].Content)
	require.Equal(t, "hi", history.History()[1].Content)
}

func TestResetHistory_AliasReceiptRestoresFromFreshRegistry(t *testing.T) {
	history := modelllm.NewConversation(nil, "", modelllm.ChatOptions{})
	history.Append(modelllm.Message{Role: modelllm.User, Content: "hello"})
	history.Append(modelllm.Message{Role: modelllm.Assistant, Content: "hi"})
	def := catalog.ToolDef{Name: "clear_conversation", Type: "builtin", Init: "reset_history"}
	const reference = "checkpoint:run-8/step-5"

	builder := &ResetHistoryBuilder{
		ToolName:                def.Name,
		History:                 history,
		Tracer:                  noopTracer(),
		ConversationRefProvider: staticConversationReference{ref: reference, available: true},
		ConversationRefResolver: fakeConversationReferenceResolver{conversations: map[string][]modelllm.Message{
			reference: {
				{Role: modelllm.User, Content: "hello"},
				{Role: modelllm.Assistant, Content: "hi"},
			},
		}},
	}
	cmd := builder.Build(core.Result{})
	require.Equal(t, def.Name, cmd.Name())
	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal)
	require.NotEmpty(t, res.Receipt)
	require.Equal(t, 0, history.Len())

	cp := &core.InMemoryCheckpoint{}
	require.NoError(t, cp.Save(core.Position{}, core.Execution{{
		CommandName: cmd.Name(),
		Result:      safeCheckpointResult(),
		Receipt:     res.Receipt,
	}}))
	_, exec, err := cp.Load()
	require.NoError(t, err)
	require.Len(t, exec, 1)

	freshHistory := modelllm.NewConversation(nil, "", modelllm.ChatOptions{})
	freshBuilder := &ResetHistoryBuilder{
		ToolName: def.Name,
		History:  freshHistory,
		ConversationRefResolver: fakeConversationReferenceResolver{conversations: map[string][]modelllm.Message{
			reference: {
				{Role: modelllm.User, Content: "hello"},
				{Role: modelllm.Assistant, Content: "hi"},
			},
		}},
	}
	freshRegistry := core.NewRegistry()
	freshRegistry.Register(def.ToToolSpec(), freshBuilder)
	resolved, ok := freshRegistry.Resolve(exec[0].CommandName)
	require.True(t, ok)
	reverser, ok := resolved.(core.Reverser)
	require.True(t, ok)
	fresh := reverser.BuildReverser()
	require.Equal(t, def.Name, fresh.Name())

	undo := fresh.Undo(core.Result{Receipt: exec[0].Receipt})
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, 2, freshHistory.Len())
	require.Equal(t, "hello", freshHistory.History()[0].Content)
	require.Equal(t, "hi", freshHistory.History()[1].Content)
}

func TestResetHistory_LegacyReceiptRestoresConversation(t *testing.T) {
	history := modelllm.NewConversation(nil, "", modelllm.ChatOptions{})
	history.Append(modelllm.Message{Role: modelllm.User, Content: "current"})
	cmd := (&ResetHistoryBuilder{History: history, Tracer: noopTracer()}).Build(core.Result{})
	legacy := []modelllm.Message{
		{Role: modelllm.User, Content: "legacy user"},
		{Role: modelllm.Assistant, Content: "legacy assistant"},
	}

	undo := cmd.Undo(core.Result{Receipt: encodeLegacyConversationReceipt(t, legacy)})

	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, legacy, history.History())
}

func TestResetHistory_V2RestartUndoRequiresValidReference(t *testing.T) {
	tests := []struct {
		name     string
		receipt  string
		resolver ConversationReferenceResolver
		wantErr  string
	}{
		{
			name:    "missing reference",
			receipt: encodeConversationReceipt(1, ""),
			wantErr: "no conversation reference",
		},
		{
			name:    "missing resolver",
			receipt: encodeConversationReceipt(1, "checkpoint:run/step"),
			wantErr: "resolver is not configured",
		},
		{
			name:    "invalid reference",
			receipt: encodeConversationReceipt(1, "checkpoint:missing"),
			resolver: fakeConversationReferenceResolver{
				conversations: map[string][]modelllm.Message{},
			},
			wantErr: "conversation reference not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history := modelllm.NewConversation(nil, "", modelllm.ChatOptions{})
			history.Append(modelllm.Message{Role: modelllm.Assistant, Content: "unchanged"})
			fresh := (&ResetHistoryBuilder{
				History:                 history,
				Tracer:                  noopTracer(),
				ConversationRefResolver: tt.resolver,
			}).Build(core.Result{})

			undo := fresh.Undo(core.Result{Receipt: tt.receipt})

			require.Equal(t, core.CommandError, undo.Signal)
			require.ErrorContains(t, undo.Err, tt.wantErr)
			require.Equal(t, []modelllm.Message{{Role: modelllm.Assistant, Content: "unchanged"}}, history.History())
			require.NotContains(t, undo.Output, "unchanged")
		})
	}
}

func TestResetHistory_V2RestartUndoRestoresEmptyHistoryWithoutReference(t *testing.T) {
	t.Parallel()
	history := modelllm.NewConversation(nil, "", modelllm.ChatOptions{})
	history.Append(modelllm.Message{Role: modelllm.Assistant, Content: "current"})
	fresh := (&ResetHistoryBuilder{
		History: history,
		Tracer:  noopTracer(),
	}).Build(core.Result{})

	undo := fresh.Undo(core.Result{Receipt: encodeConversationReceipt(0, "")})

	require.Equal(t, core.ToolDone, undo.Signal)
	require.Empty(t, history.History())
}

func TestResetHistory_RejectsUnrelatedAndUnknownReceiptFields(t *testing.T) {
	tests := []struct {
		name    string
		receipt string
		wantErr string
		secret  string
	}{
		{
			name:    "retry receipt is not legacy conversation",
			receipt: encodeRetryReceipt(3),
			wantErr: "missing conversation field",
		},
		{
			name:    "unrelated receipt is not legacy conversation",
			receipt: `{"resource_id":"external-123"}`,
			wantErr: "missing conversation field",
		},
		{
			name:    "v2 rejects unknown field",
			receipt: `{"version":2,"prior_conversation_length":1,"extra":"metadata"}`,
			wantErr: `unknown field "extra"`,
		},
		{
			name:    "v2 rejects content-looking field",
			receipt: `{"version":2,"prior_conversation_length":1,"assistant":"receipt secret"}`,
			wantErr: `unknown field "assistant"`,
			secret:  "receipt secret",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history := modelllm.NewConversation(nil, "", modelllm.ChatOptions{})
			want := []modelllm.Message{{Role: modelllm.User, Content: "must remain"}}
			history.Restore(want)
			fresh := (&ResetHistoryBuilder{History: history, Tracer: noopTracer()}).Build(core.Result{})

			undo := fresh.Undo(core.Result{Receipt: tt.receipt})

			require.Equal(t, core.CommandError, undo.Signal)
			require.ErrorContains(t, undo.Err, tt.wantErr)
			require.Equal(t, want, history.History())
			if tt.secret != "" {
				require.NotContains(t, undo.Output, tt.secret)
			}
		})
	}
}

func safeCheckpointResult() core.ResultDigest {
	return core.ResultDigest{
		RedactionVersion: core.OutputRedactionVersion1,
		RedactionStatus:  core.OutputRedactionApplied,
	}
}
