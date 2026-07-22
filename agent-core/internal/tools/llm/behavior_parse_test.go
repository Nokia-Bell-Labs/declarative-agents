// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"encoding/json"
	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/filesystem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

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
