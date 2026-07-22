// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
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

	undo := cmd.Undo(core.Result{})
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, 2, history.Len())
	require.Equal(t, "hello", history.History()[0].Content)
	require.Equal(t, "hi", history.History()[1].Content)
}

func TestResetHistory_ReceiptRestoresFromFreshInstance(t *testing.T) {
	history := modelllm.NewConversation(nil, "", modelllm.ChatOptions{})
	history.Append(modelllm.Message{Role: modelllm.User, Content: "hello"})
	history.Append(modelllm.Message{Role: modelllm.Assistant, Content: "hi"})

	builder := &ResetHistoryBuilder{History: history, Tracer: noopTracer()}
	cmd := builder.Build(core.Result{})
	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal)
	require.NotEmpty(t, res.Receipt)
	require.Equal(t, 0, history.Len())

	cp := &core.InMemoryCheckpoint{}
	require.NoError(t, cp.Save(core.Position{}, core.Execution{{CommandName: "reset_history", Receipt: res.Receipt}}))
	_, exec, err := cp.Load()
	require.NoError(t, err)
	require.Len(t, exec, 1)

	fresh := builder.Build(core.Result{})
	undo := fresh.Undo(core.Result{Receipt: exec[0].Receipt})
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, 2, history.Len())
	require.Equal(t, "hello", history.History()[0].Content)
	require.Equal(t, "hi", history.History()[1].Content)
}
