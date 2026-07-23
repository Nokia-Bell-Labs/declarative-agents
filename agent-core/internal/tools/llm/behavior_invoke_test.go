// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"context"
	"fmt"
	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestInvokeLLM_Success(t *testing.T) {
	client := &fakeChatClient{
		response: modelllm.ChatResponse{
			Content:  `[tool_call]{"tool":"read","parameters":{"path":"main.go"}}[/tool_call]`,
			TokensIn: 100, TokensOut: 50,
		},
	}
	history := modelllm.NewConversation(nil, "", modelllm.ChatOptions{})
	reg := core.NewRegistry()

	builder := &InvokeLLMBuilder{
		Client:    client,
		History:   history,
		Registry:  reg,
		Assembler: &fakeAssembler{},
		Model:     "test-model",
		Tracer:    noopTracer(),
		Ctx:       context.Background(),
	}

	cmd := builder.Build(core.Result{Output: "implement the feature"})
	res := cmd.Execute()

	assert.Equal(t, core.LLMResponded, res.Signal)
	assert.Contains(t, res.Output, "tool_call")
	assert.Equal(t, 100, res.Cost.TokensIn)
	assert.Equal(t, 50, res.Cost.TokensOut)
	assert.Equal(t, 2, history.Len()) // user + assistant
}

func TestInvokeLLM_UndoRestoresPreviousHistoryLength(t *testing.T) {
	client := &fakeChatClient{
		response: modelllm.ChatResponse{Content: "assistant response"},
	}
	history := modelllm.NewConversation(nil, "", modelllm.ChatOptions{})
	history.Append(modelllm.Message{Role: modelllm.User, Content: "existing"})

	builder := &InvokeLLMBuilder{
		Client:    client,
		History:   history,
		Registry:  core.NewRegistry(),
		Assembler: &fakeAssembler{},
		Model:     "test-model",
		Tracer:    noopTracer(),
		Ctx:       context.Background(),
	}

	cmd := builder.Build(core.Result{Output: "new prompt"})
	res := cmd.Execute()
	require.Equal(t, core.LLMResponded, res.Signal)
	require.Equal(t, 3, history.Len())

	undo := cmd.Undo(core.Result{})
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, 1, history.Len())
	require.Equal(t, "existing", history.History()[0].Content)
}

func TestInvokeLLM_ReceiptRestoresConversationFromFreshInstance(t *testing.T) {
	client := &fakeChatClient{
		response: modelllm.ChatResponse{Content: "assistant response"},
	}
	history := modelllm.NewConversation(nil, "", modelllm.ChatOptions{})
	history.Append(modelllm.Message{Role: modelllm.User, Content: "existing"})

	builder := &InvokeLLMBuilder{
		Client:    client,
		History:   history,
		Registry:  core.NewRegistry(),
		Assembler: &fakeAssembler{},
		Model:     "test-model",
		Tracer:    noopTracer(),
		Ctx:       context.Background(),
	}

	cmd := builder.Build(core.Result{Output: "new prompt"})
	res := cmd.Execute()
	require.Equal(t, core.LLMResponded, res.Signal)
	require.NotEmpty(t, res.Receipt)
	require.Equal(t, 3, history.Len())

	cp := &core.InMemoryCheckpoint{}
	require.NoError(t, cp.Save(core.Position{}, core.Execution{{
		CommandName: "invoke_llm",
		Result:      safeCheckpointResult(),
		Receipt:     res.Receipt,
	}}))
	_, exec, err := cp.Load()
	require.NoError(t, err)
	require.Len(t, exec, 1)

	fresh := builder.Build(core.Result{Output: "new prompt"})
	undo := fresh.Undo(core.Result{Receipt: exec[0].Receipt})
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, 1, history.Len())
	require.Equal(t, "existing", history.History()[0].Content)
}

func TestInvokeLLM_UndoRestoresUserMessageAfterError(t *testing.T) {
	client := &fakeChatClient{err: fmt.Errorf("connection refused")}
	history := modelllm.NewConversation(nil, "", modelllm.ChatOptions{})
	history.Append(modelllm.Message{Role: modelllm.User, Content: "existing"})

	builder := &InvokeLLMBuilder{
		Client:    client,
		History:   history,
		Registry:  core.NewRegistry(),
		Assembler: &fakeAssembler{},
		Model:     "test-model",
		Tracer:    noopTracer(),
		Ctx:       context.Background(),
	}

	cmd := builder.Build(core.Result{Output: "new prompt"})
	res := cmd.Execute()
	require.Equal(t, core.CommandError, res.Signal)
	require.Equal(t, 2, history.Len())

	undo := cmd.Undo(core.Result{})
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, 1, history.Len())
}

func TestInvokeLLM_ChatError(t *testing.T) {
	client := &fakeChatClient{err: fmt.Errorf("connection refused")}
	history := modelllm.NewConversation(nil, "", modelllm.ChatOptions{})

	builder := &InvokeLLMBuilder{
		Client:    client,
		History:   history,
		Registry:  core.NewRegistry(),
		Assembler: &fakeAssembler{},
		Model:     "test-model",
		Tracer:    noopTracer(),
		Ctx:       context.Background(),
	}

	cmd := builder.Build(core.Result{Output: "hello"})
	res := cmd.Execute()

	assert.Equal(t, core.CommandError, res.Signal)
	assert.Error(t, res.Err)
	assert.Equal(t, 1, history.Len()) // only user message
}

func TestInvokeLLM_ContextOverflow(t *testing.T) {
	client := &fakeChatClient{}
	history := modelllm.NewConversation(nil, "", modelllm.ChatOptions{})

	builder := &InvokeLLMBuilder{
		Client:       client,
		History:      history,
		Registry:     core.NewRegistry(),
		Assembler:    &fakeAssembler{},
		Model:        "test-model",
		Tracer:       noopTracer(),
		ContextLimit: 1, // impossibly small
		Ctx:          context.Background(),
	}

	cmd := builder.Build(core.Result{Output: "this message will overflow the tiny context limit"})
	res := cmd.Execute()

	assert.Equal(t, core.CommandError, res.Signal)
	assert.Contains(t, res.Output, "context window")
}

func TestInvokeLLM_CallTimeout(t *testing.T) {
	history := modelllm.NewConversation(nil, "", modelllm.ChatOptions{})
	builder := &InvokeLLMBuilder{
		Client:      waitClient{},
		History:     history,
		Registry:    core.NewRegistry(),
		Assembler:   &fakeAssembler{},
		Model:       "test-model",
		Tracer:      noopTracer(),
		CallTimeout: time.Millisecond,
		Ctx:         context.Background(),
	}

	cmd := builder.Build(core.Result{Output: "wait for input"})
	res := cmd.Execute()

	assert.Equal(t, core.CommandError, res.Signal)
	assert.ErrorIs(t, res.Err, context.DeadlineExceeded)
	assert.Positive(t, res.Cost.Duration)
}
