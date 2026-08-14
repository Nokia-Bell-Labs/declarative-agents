// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"encoding/json"
	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
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

	assert.Equal(t, "parse_response", cmd.Name())
	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Equal(t, "parse_response", res.CommandName)
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
	builder := &ReportParseErrorBuilder{
		Tracer: noopTracer(),
		FeedbackTemplate: "Your previous response was invalid. {{error}}\n\n" +
			"Please respond with a single JSON object: {\"tool\": \"<tool_name>\", \"parameters\": {<params>}}",
	}
	cmd := builder.Build(core.Result{Output: "malformed JSON: unexpected EOF"})
	res := cmd.Execute()

	assert.Equal(t, "report_parse_error", cmd.Name())
	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Equal(t, "report_parse_error", res.CommandName)
	assert.Equal(t,
		"Your previous response was invalid. malformed JSON: unexpected EOF\n\n"+
			"Please respond with a single JSON object: {\"tool\": \"<tool_name>\", \"parameters\": {<params>}}",
		res.Output,
	)
}

func TestReportParseError_ImplementationPlanYAMLFeedback(t *testing.T) {
	builder := &ReportParseErrorBuilder{
		Tracer: noopTracer(),
		FeedbackTemplate: "Your previous response was invalid. {{error}}\n\n" +
			"Please respond with exactly one top-level YAML mapping and no other document content. " +
			"The mapping must contain exactly these six keys: title, summary, files, requirements, " +
			"design_decisions, and acceptance_criteria. Do not return a root sequence/list, multiple " +
			"plans, a wrapper/envelope key, Markdown fences, prose, or any keys outside this mapping.",
	}
	cmd := builder.Build(core.Result{Output: "parse plan: missing required field: requirements (list is empty)"})
	res := cmd.Execute()

	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Equal(t,
		"Your previous response was invalid. parse plan: missing required field: requirements (list is empty)\n\n"+
			"Please respond with exactly one top-level YAML mapping and no other document content. "+
			"The mapping must contain exactly these six keys: title, summary, files, requirements, "+
			"design_decisions, and acceptance_criteria. Do not return a root sequence/list, multiple "+
			"plans, a wrapper/envelope key, Markdown fences, prose, or any keys outside this mapping.",
		res.Output,
	)
}

func TestReportParseError_ProfileSuppliesDifferentFeedback(t *testing.T) {
	builder := &ReportParseErrorBuilder{
		Tracer: noopTracer(), FeedbackTemplate: "Custom correction for {{error}}.",
	}
	result := builder.Build(core.Result{Output: "bad shape"}).Execute()
	require.Equal(t, "Custom correction for bad shape.", result.Output)
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

func TestReportParseError_AliasReceiptRestoresRetryCounterFromFreshRegistry(t *testing.T) {
	tracker := &ParseErrorRetryTracker{MaxConsecutive: 3}
	tracker.ReportParseError()
	def := catalog.ToolDef{Name: "explain_parse_failure", Type: "builtin", Init: "report_parse_error"}
	builder := &ReportParseErrorBuilder{
		ToolName: def.Name,
		Tracer:   noopTracer(),
		Retry:    tracker,
	}

	cmd := builder.Build(core.Result{Output: "bad JSON"})
	require.Equal(t, def.Name, cmd.Name())
	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal)
	require.Equal(t, def.Name, res.CommandName)
	require.NotEmpty(t, res.Receipt)
	require.JSONEq(t, `{"retry_receipt_version":1,"parse_retry_counter":1}`, res.Receipt)
	require.Equal(t, 2, tracker.Snapshot())

	cp := &core.InMemoryCheckpoint{}
	require.NoError(t, cp.Save(core.Position{}, core.Execution{{
		CommandName: cmd.Name(),
		Result:      safeCheckpointResult(),
		Receipt:     res.Receipt,
	}}))
	_, exec, err := cp.Load()
	require.NoError(t, err)
	require.Len(t, exec, 1)

	freshTracker := &ParseErrorRetryTracker{MaxConsecutive: 3}
	freshBuilder := &ReportParseErrorBuilder{ToolName: def.Name, Retry: freshTracker}
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
	require.Equal(t, def.Name, undo.CommandName)
	require.Equal(t, 1, freshTracker.Snapshot())
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

func TestParseResponse_AliasReceiptsRestoreSuccessfulAndFailedParsesFromFreshRegistry(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{
		Name:        "read",
		InputSchema: json.RawMessage(`{"type":"object","required":["path"]}`),
		Visibility:  core.External,
	}, &filesystem.ReadBuilder{Root: "/tmp"})
	def := catalog.ToolDef{Name: "decode_response", Type: "builtin", Init: "parse_response"}

	tests := []struct {
		name       string
		raw        string
		wantSignal core.Signal
	}{
		{
			name:       "successful parse resets retry counter",
			raw:        `{"tool":"read","parameters":{"path":"main.go"}}`,
			wantSignal: core.ToolDone,
		},
		{
			name:       "failed parse preserves retry counter",
			raw:        `not json`,
			wantSignal: core.ParseFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := &ParseErrorRetryTracker{MaxConsecutive: 3}
			tracker.ReportParseError()
			tracker.ReportParseError()
			builder := &ParseResponseBuilder{
				ToolName: def.Name,
				Registry: reg,
				Parser:   &fakeParser{},
				Tracer:   noopTracer(),
				Retry:    tracker,
			}

			cmd := builder.Build(core.Result{Output: tt.raw})
			require.Equal(t, def.Name, cmd.Name())
			res := cmd.Execute()
			require.Equal(t, tt.wantSignal, res.Signal)
			require.Equal(t, def.Name, res.CommandName)
			require.NotEmpty(t, res.Receipt)
			require.JSONEq(t, `{"retry_receipt_version":1,"parse_retry_counter":2}`, res.Receipt)

			cp := &core.InMemoryCheckpoint{}
			require.NoError(t, cp.Save(core.Position{}, core.Execution{{
				CommandName: cmd.Name(),
				Result:      safeCheckpointResult(),
				Receipt:     res.Receipt,
			}}))
			_, exec, err := cp.Load()
			require.NoError(t, err)
			require.Len(t, exec, 1)

			freshTracker := &ParseErrorRetryTracker{MaxConsecutive: 3}
			freshBuilder := &ParseResponseBuilder{ToolName: def.Name, Retry: freshTracker}
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
			require.Equal(t, def.Name, undo.CommandName)
			require.Equal(t, 2, freshTracker.Snapshot())
		})
	}
}

func TestParseRetryReversers_ReturnNamedErrorsForInvalidReceipts(t *testing.T) {
	tests := []struct {
		name    string
		receipt string
		wantErr string
	}{
		{name: "missing", wantErr: "no retry counter snapshot recorded"},
		{name: "malformed", receipt: `{`, wantErr: "decode receipt"},
		{
			name:    "missing counter",
			receipt: `{"retry_receipt_version":1}`,
			wantErr: "missing parse_retry_counter",
		},
		{
			name:    "unrelated",
			receipt: `{"version":2,"prior_conversation_length":0}`,
			wantErr: "unknown field",
		},
	}
	builders := []struct {
		name    string
		builder core.Reverser
	}{
		{
			name: "parse response",
			builder: &ParseResponseBuilder{
				ToolName: "decode_response",
				Retry:    &ParseErrorRetryTracker{},
			},
		},
		{
			name: "report parse error",
			builder: &ReportParseErrorBuilder{
				ToolName: "explain_parse_failure",
				Retry:    &ParseErrorRetryTracker{},
			},
		},
	}
	for _, builder := range builders {
		t.Run(builder.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					cmd := builder.builder.BuildReverser()
					undo := cmd.Undo(core.Result{Receipt: tt.receipt})

					require.Equal(t, core.CommandError, undo.Signal)
					require.Equal(t, cmd.Name(), undo.CommandName)
					require.ErrorContains(t, undo.Err, tt.wantErr)
					require.Contains(t, undo.Output, cmd.Name())
				})
			}
		})
	}
}

func TestParseRetryReversers_AcceptLegacyUnversionedReceipts(t *testing.T) {
	parseTracker := &ParseErrorRetryTracker{}
	reportTracker := &ParseErrorRetryTracker{}
	builders := []struct {
		builder core.Reverser
		tracker *ParseErrorRetryTracker
	}{
		{builder: &ParseResponseBuilder{
			ToolName: "decode_response",
			Retry:    parseTracker,
		}, tracker: parseTracker},
		{builder: &ReportParseErrorBuilder{
			ToolName: "explain_parse_failure",
			Retry:    reportTracker,
		}, tracker: reportTracker},
	}
	for _, tt := range builders {
		cmd := tt.builder.BuildReverser()
		undo := cmd.Undo(core.Result{Receipt: `{"parse_retry_counter":2}`})

		require.Equal(t, core.ToolDone, undo.Signal)
		require.Equal(t, cmd.Name(), undo.CommandName)
		require.Equal(t, 2, tt.tracker.Snapshot())
	}
}
