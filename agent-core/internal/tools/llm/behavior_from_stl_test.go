// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/prompt"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/filesystem"
)

// --- test doubles ---

type fakeChatClient struct {
	response modelllm.ChatResponse
	err      error
}

func (s *fakeChatClient) Chat(_ context.Context, _ []modelllm.Message, _ modelllm.ChatOptions) (modelllm.ChatResponse, error) {
	return s.response, s.err
}

func (s *fakeChatClient) ListModels(_ context.Context) ([]modelllm.ModelInfo, error) {
	return nil, nil
}

type waitClient struct{}

func (w waitClient) Chat(ctx context.Context, _ []modelllm.Message, _ modelllm.ChatOptions) (modelllm.ChatResponse, error) {
	<-ctx.Done()
	return modelllm.ChatResponse{}, ctx.Err()
}

func (w waitClient) ListModels(_ context.Context) ([]modelllm.ModelInfo, error) {
	return nil, nil
}

type fakeAssembler struct{}

func (s *fakeAssembler) AssembleMessages(conv *modelllm.Conversation, _ *core.Registry, _ core.State) []modelllm.Message {
	msgs := []modelllm.Message{{Role: modelllm.System, Content: "You are a helper."}}
	msgs = append(msgs, conv.Messages()...)
	return msgs
}

type fakeParser struct{}

func (s *fakeParser) ExtractToolCall(raw string) string {
	return modelllm.ExtractBraces(raw)
}

func (s *fakeParser) EnvelopeConfig() (*prompt.Envelope, bool) {
	return nil, false
}

func noopTracer() tracing.Tracer {
	return tracing.NoopTracer{}
}

// --- invoke_llm tests ---

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
	require.NoError(t, cp.Save(core.Position{}, core.Execution{{CommandName: "invoke_llm", Receipt: res.Receipt}}))
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

// --- parse_response tests ---

func TestParseResponse_ValidToolCall(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{
		Name:        "read",
		Description: "Read a file",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		Visibility:  core.External,
	}, &filesystem.ReadBuilder{Root: "/tmp"})

	builder := &ParseResponseBuilder{
		Registry: reg,
		Parser:   &fakeParser{},
		Tracer:   noopTracer(),
	}

	cmd := builder.Build(core.Result{Output: `{"tool":"read","parameters":{"path":"main.go"}}`})
	res := cmd.Execute()

	assert.Equal(t, core.ToolDone, res.Signal)
	var tr modelllm.ToolRequest
	require.NoError(t, json.Unmarshal([]byte(res.Output), &tr))
	assert.Equal(t, "read", tr.ToolName)
}

func TestParseResponse_DoneTool(t *testing.T) {
	builder := &ParseResponseBuilder{
		Registry: core.NewRegistry(),
		Parser:   &fakeParser{},
		Tracer:   noopTracer(),
	}

	cmd := builder.Build(core.Result{Output: `{"tool":"done","parameters":{"summary":"task complete"}}`})
	res := cmd.Execute()

	assert.Equal(t, core.TaskCompleted, res.Signal)
	assert.Equal(t, "task complete", res.Output)
}

func TestParseResponse_MalformedJSON(t *testing.T) {
	builder := &ParseResponseBuilder{
		Registry: core.NewRegistry(),
		Parser:   &fakeParser{},
		Tracer:   noopTracer(),
	}

	cmd := builder.Build(core.Result{Output: `not json at all`})
	res := cmd.Execute()

	assert.Equal(t, core.ParseFailed, res.Signal)
	assert.Contains(t, res.Output, "malformed JSON")
}

func TestParseResponse_UnknownTool(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{
		Name:       "read",
		Visibility: core.External,
	}, &filesystem.ReadBuilder{Root: "/tmp"})

	builder := &ParseResponseBuilder{
		Registry: reg,
		Parser:   &fakeParser{},
		Tracer:   noopTracer(),
	}

	cmd := builder.Build(core.Result{Output: `{"tool":"nonexistent","parameters":{}}`})
	res := cmd.Execute()

	assert.Equal(t, core.ParseFailed, res.Signal)
	assert.Contains(t, res.Output, "unknown tool")
}

func TestParseResponse_MissingRequiredParam(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{
		Name:        "read",
		Description: "Read a file",
		InputSchema: json.RawMessage(`{"type":"object","required":["path"]}`),
		Visibility:  core.External,
	}, &filesystem.ReadBuilder{Root: "/tmp"})

	builder := &ParseResponseBuilder{
		Registry: reg,
		Parser:   &fakeParser{},
		Tracer:   noopTracer(),
	}

	cmd := builder.Build(core.Result{Output: `{"tool":"read","parameters":{}}`})
	res := cmd.Execute()

	assert.Equal(t, core.ParseFailed, res.Signal)
	assert.Contains(t, res.Output, "missing required parameters")
}

func TestParseResponse_FlatParams(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{
		Name:        "read",
		Description: "Read a file",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		Visibility:  core.External,
	}, &filesystem.ReadBuilder{Root: "/tmp"})

	builder := &ParseResponseBuilder{
		Registry: reg,
		Parser:   &fakeParser{},
		Tracer:   noopTracer(),
	}

	cmd := builder.Build(core.Result{Output: `{"tool":"read","path":"main.go"}`})
	res := cmd.Execute()

	assert.Equal(t, core.ToolDone, res.Signal)
}

func TestParseResponse_FixNewlines(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{
		Name:        "write",
		Description: "Write a file",
		InputSchema: json.RawMessage(`{"type":"object","required":["path","content"]}`),
		Visibility:  core.External,
	}, &filesystem.WriteBuilder{Root: "/tmp"})

	builder := &ParseResponseBuilder{
		Registry: reg,
		Parser:   &fakeParser{},
		Tracer:   noopTracer(),
	}

	raw := `{"tool":"write","parameters":{"path":"f.go","content":"line1` + "\n" + `line2"}}`
	cmd := builder.Build(core.Result{Output: raw})
	res := cmd.Execute()

	assert.Equal(t, core.ToolDone, res.Signal)
}

// --- report_parse_error tests ---

func TestReportParseError(t *testing.T) {
	builder := &ReportParseErrorBuilder{Tracer: noopTracer()}
	cmd := builder.Build(core.Result{Output: "malformed JSON: unexpected EOF"})
	res := cmd.Execute()

	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Contains(t, res.Output, "invalid")
	assert.Contains(t, res.Output, "malformed JSON")
}

func TestReportParseError_EmitsBudgetExhaustedAtRetryLimit(t *testing.T) {
	tracker := &ParseErrorRetryTracker{MaxConsecutive: 2}
	builder := &ReportParseErrorBuilder{Tracer: noopTracer(), Retry: tracker}

	first := builder.Build(core.Result{Output: "bad JSON"})
	res := first.Execute()
	require.Equal(t, core.ToolDone, res.Signal)

	second := builder.Build(core.Result{Output: "still bad JSON"})
	res = second.Execute()
	require.Equal(t, core.BudgetExhausted, res.Signal)
	require.Contains(t, res.Output, "retry limit")
	require.Equal(t, 2, tracker.Snapshot())
}

func TestReportParseError_UndoRestoresRetryCounter(t *testing.T) {
	tracker := &ParseErrorRetryTracker{MaxConsecutive: 3}
	builder := &ReportParseErrorBuilder{Tracer: noopTracer(), Retry: tracker}

	cmd := builder.Build(core.Result{Output: "bad JSON"})
	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal)
	require.Equal(t, 1, tracker.Snapshot())

	undo := cmd.Undo(core.Result{})
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, 0, tracker.Snapshot())
}

func TestReportParseError_ReceiptRestoresRetryCounterFromFreshInstance(t *testing.T) {
	tracker := &ParseErrorRetryTracker{MaxConsecutive: 3}
	builder := &ReportParseErrorBuilder{Tracer: noopTracer(), Retry: tracker}

	cmd := builder.Build(core.Result{Output: "bad JSON"})
	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal)
	require.NotEmpty(t, res.Receipt)
	require.Equal(t, 1, tracker.Snapshot())

	cp := &core.InMemoryCheckpoint{}
	require.NoError(t, cp.Save(core.Position{}, core.Execution{{CommandName: "report_parse_error", Receipt: res.Receipt}}))
	_, exec, err := cp.Load()
	require.NoError(t, err)
	require.Len(t, exec, 1)

	fresh := builder.Build(core.Result{Output: "bad JSON"})
	undo := fresh.Undo(core.Result{Receipt: exec[0].Receipt})
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, 0, tracker.Snapshot())
}

func TestParseResponse_ResetsRetryCounterAfterSuccessfulParse(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{
		Name:        "read",
		Description: "Read a file",
		InputSchema: json.RawMessage(`{"type":"object","required":["path"]}`),
		Visibility:  core.External,
	}, &filesystem.ReadBuilder{Root: "/tmp"})
	tracker := &ParseErrorRetryTracker{MaxConsecutive: 3}
	tracker.ReportParseError()
	tracker.ReportParseError()

	builder := &ParseResponseBuilder{
		Registry: reg,
		Parser:   &fakeParser{},
		Tracer:   noopTracer(),
		Retry:    tracker,
	}
	cmd := builder.Build(core.Result{Output: `{"tool":"read","parameters":{"path":"main.go"}}`})
	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal)
	require.Equal(t, 0, tracker.Snapshot())

	undo := cmd.Undo(core.Result{})
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, 2, tracker.Snapshot())
}

// --- reset_history tests ---

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
