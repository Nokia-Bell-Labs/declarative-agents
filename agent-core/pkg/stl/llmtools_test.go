// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/prompt"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// --- test doubles ---

type stubClient struct {
	response llm.ChatResponse
	err      error
}

func (s *stubClient) Chat(_ context.Context, _ []llm.Message, _ llm.ChatOptions) (llm.ChatResponse, error) {
	return s.response, s.err
}

func (s *stubClient) ListModels(_ context.Context) ([]llm.ModelInfo, error) {
	return nil, nil
}

type stubAssembler struct{}

func (s *stubAssembler) AssembleMessages(conv *llm.Conversation, _ *core.Registry, _ core.State) []llm.Message {
	msgs := []llm.Message{{Role: llm.System, Content: "You are a helper."}}
	msgs = append(msgs, conv.Messages()...)
	return msgs
}

type stubParser struct{}

func (s *stubParser) ExtractToolCall(raw string) string {
	return llm.ExtractBraces(raw)
}

func (s *stubParser) EnvelopeConfig() (*prompt.Envelope, bool) {
	return nil, false
}

func noopTracer() tracing.Tracer {
	return tracing.NoopTracer{}
}

type alwaysCheckpointPolicy struct{}

func (alwaysCheckpointPolicy) ShouldCheckpoint(core.CheckpointEvent) bool { return true }

// --- invoke_llm tests ---

func TestInvokeLLM_Success(t *testing.T) {
	client := &stubClient{
		response: llm.ChatResponse{
			Content:  `[tool_call]{"tool":"read","parameters":{"path":"main.go"}}[/tool_call]`,
			TokensIn: 100, TokensOut: 50,
		},
	}
	history := llm.NewConversation(nil, "", llm.ChatOptions{})
	reg := core.NewRegistry()

	builder := &InvokeLLMBuilder{
		Client:    client,
		History:   history,
		Registry:  reg,
		Assembler: &stubAssembler{},
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
	client := &stubClient{
		response: llm.ChatResponse{Content: "assistant response"},
	}
	history := llm.NewConversation(nil, "", llm.ChatOptions{})
	history.Append(llm.Message{Role: llm.User, Content: "existing"})

	builder := &InvokeLLMBuilder{
		Client:    client,
		History:   history,
		Registry:  core.NewRegistry(),
		Assembler: &stubAssembler{},
		Model:     "test-model",
		Tracer:    noopTracer(),
		Ctx:       context.Background(),
	}

	cmd := builder.Build(core.Result{Output: "new prompt"})
	res := cmd.Execute()
	require.Equal(t, core.LLMResponded, res.Signal)
	require.Equal(t, 3, history.Len())

	undo := cmd.Undo()
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, 1, history.Len())
	require.Equal(t, "existing", history.History()[0].Content)
}

func TestInvokeLLM_HistoryCapturesUndoMemento(t *testing.T) {
	client := &stubClient{
		response: llm.ChatResponse{Content: "assistant response"},
	}
	history := llm.NewConversation(nil, "", llm.ChatOptions{})
	history.Append(llm.Message{Role: llm.User, Content: "existing"})
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "invoke_llm", Visibility: core.Internal}, &InvokeLLMBuilder{
		Client:    client,
		History:   history,
		Registry:  reg,
		Assembler: &stubAssembler{},
		Model:     "test-model",
		Tracer:    noopTracer(),
		Ctx:       context.Background(),
	})

	spec := core.MachineSpec{
		Name:           "llm-memento-test",
		InitialState:   "Start",
		States:         core.StateSpecsFromNames("Start", "Working", "Finished"),
		TerminalStates: []string{"Finished"},
		Signals:        core.SignalSpecsFromNames("Seed", "LLMResponded"),
		Transitions: []core.TransitionSpec{
			{State: "Start", Signal: "Seed", Next: "Working", Action: "invoke_llm"},
			{State: "Working", Signal: "LLMResponded", Next: "Finished"},
		},
	}

	rr, err := core.Loop(core.LoopParams{
		MachineSpec:      &spec,
		Registry:         reg,
		Trace:            noopTracer(),
		Budget:           core.Budget{MaxIterations: 10},
		CheckpointPolicy: alwaysCheckpointPolicy{},
		Hooks: core.LoopHooks{
			TerminalStatus: func(s core.State) core.RunStatus {
				if s == "Finished" {
					return core.StatusSucceeded
				}
				return core.StatusFailed
			},
		},
	}, context.Background())

	require.NoError(t, err)
	require.Equal(t, core.StatusSucceeded, rr.Status)
	require.Len(t, rr.History, 1)
	require.NotNil(t, rr.History[0].Undo)
	require.Equal(t, core.UndoMementoReversible, rr.History[0].Undo.Kind)
	require.NoError(t, core.ValidateUndoMemento(*rr.History[0].Undo))

	var payload struct {
		Conversation []llm.Message `json:"conversation"`
	}
	require.NoError(t, json.Unmarshal(rr.History[0].Undo.Payload, &payload))
	require.Len(t, payload.Conversation, 1)
	require.Equal(t, "existing", payload.Conversation[0].Content)
}

func TestInvokeLLM_UndoRestoresUserMessageAfterError(t *testing.T) {
	client := &stubClient{err: fmt.Errorf("connection refused")}
	history := llm.NewConversation(nil, "", llm.ChatOptions{})
	history.Append(llm.Message{Role: llm.User, Content: "existing"})

	builder := &InvokeLLMBuilder{
		Client:    client,
		History:   history,
		Registry:  core.NewRegistry(),
		Assembler: &stubAssembler{},
		Model:     "test-model",
		Tracer:    noopTracer(),
		Ctx:       context.Background(),
	}

	cmd := builder.Build(core.Result{Output: "new prompt"})
	res := cmd.Execute()
	require.Equal(t, core.CommandError, res.Signal)
	require.Equal(t, 2, history.Len())

	undo := cmd.Undo()
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, 1, history.Len())
}

func TestInvokeLLM_ChatError(t *testing.T) {
	client := &stubClient{err: fmt.Errorf("connection refused")}
	history := llm.NewConversation(nil, "", llm.ChatOptions{})

	builder := &InvokeLLMBuilder{
		Client:    client,
		History:   history,
		Registry:  core.NewRegistry(),
		Assembler: &stubAssembler{},
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
	client := &stubClient{}
	history := llm.NewConversation(nil, "", llm.ChatOptions{})

	builder := &InvokeLLMBuilder{
		Client:       client,
		History:      history,
		Registry:     core.NewRegistry(),
		Assembler:    &stubAssembler{},
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

// --- parse_response tests ---

func TestParseResponse_ValidToolCall(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{
		Name:        "read",
		Description: "Read a file",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		Visibility:  core.External,
	}, &ReadBuilder{Root: "/tmp"})

	builder := &ParseResponseBuilder{
		Registry: reg,
		Parser:   &stubParser{},
		Tracer:   noopTracer(),
	}

	cmd := builder.Build(core.Result{Output: `{"tool":"read","parameters":{"path":"main.go"}}`})
	res := cmd.Execute()

	assert.Equal(t, core.ToolDone, res.Signal)
	var tr llm.ToolRequest
	require.NoError(t, json.Unmarshal([]byte(res.Output), &tr))
	assert.Equal(t, "read", tr.ToolName)
}

func TestParseResponse_DoneTool(t *testing.T) {
	builder := &ParseResponseBuilder{
		Registry: core.NewRegistry(),
		Parser:   &stubParser{},
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
		Parser:   &stubParser{},
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
	}, &ReadBuilder{Root: "/tmp"})

	builder := &ParseResponseBuilder{
		Registry: reg,
		Parser:   &stubParser{},
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
	}, &ReadBuilder{Root: "/tmp"})

	builder := &ParseResponseBuilder{
		Registry: reg,
		Parser:   &stubParser{},
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
	}, &ReadBuilder{Root: "/tmp"})

	builder := &ParseResponseBuilder{
		Registry: reg,
		Parser:   &stubParser{},
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
	}, &WriteBuilder{Root: "/tmp"})

	builder := &ParseResponseBuilder{
		Registry: reg,
		Parser:   &stubParser{},
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

	undo := cmd.Undo()
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
	}, &ReadBuilder{Root: "/tmp"})
	tracker := &ParseErrorRetryTracker{MaxConsecutive: 3}
	tracker.ReportParseError()
	tracker.ReportParseError()

	builder := &ParseResponseBuilder{
		Registry: reg,
		Parser:   &stubParser{},
		Tracer:   noopTracer(),
		Retry:    tracker,
	}
	cmd := builder.Build(core.Result{Output: `{"tool":"read","parameters":{"path":"main.go"}}`})
	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal)
	require.Equal(t, 0, tracker.Snapshot())

	undo := cmd.Undo()
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, 2, tracker.Snapshot())
}

// --- reset_history tests ---

func TestResetHistory(t *testing.T) {
	history := llm.NewConversation(nil, "", llm.ChatOptions{})
	history.Append(llm.Message{Role: llm.User, Content: "hello"})
	history.Append(llm.Message{Role: llm.Assistant, Content: "hi"})
	assert.Equal(t, 2, history.Len())

	builder := &ResetHistoryBuilder{History: history, Tracer: noopTracer()}
	cmd := builder.Build(core.Result{})
	res := cmd.Execute()

	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Equal(t, 0, history.Len())
}

func TestResetHistory_UndoRestoresPreviousMessages(t *testing.T) {
	history := llm.NewConversation(nil, "", llm.ChatOptions{})
	history.Append(llm.Message{Role: llm.User, Content: "hello"})
	history.Append(llm.Message{Role: llm.Assistant, Content: "hi"})

	builder := &ResetHistoryBuilder{History: history, Tracer: noopTracer()}
	cmd := builder.Build(core.Result{})
	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal)
	require.Equal(t, 0, history.Len())

	undo := cmd.Undo()
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, 2, history.Len())
	require.Equal(t, "hello", history.History()[0].Content)
	require.Equal(t, "hi", history.History()[1].Content)
}

func TestNudgeReread_UndoIsNoopBecauseCommandDoesNotMutateHistory(t *testing.T) {
	builder := &NudgeRereadBuilder{Tracer: noopTracer()}
	cmd := builder.Build(core.Result{Output: "edited file"})

	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal)
	require.Contains(t, res.Output, rereadNudge)

	undo := cmd.Undo()
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Contains(t, undo.Output, "no-op")
}
