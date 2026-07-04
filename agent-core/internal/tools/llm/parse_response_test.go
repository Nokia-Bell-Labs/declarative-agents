// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestParseResponseValidatesStateScopedTools(t *testing.T) {
	t.Parallel()
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "read", Visibility: core.External, Phases: []core.State{"Composing"}, PhaseScoped: true}, nil)

	res := (&ParseResponseBuilder{Registry: reg, State: "Composing", Tracer: tracing.NoopTracer{}}).
		Build(core.Result{State: "Parsing", Output: `{"tool":"read","parameters":{}}`}).
		Execute()

	require.Equal(t, core.ToolDone, res.Signal)
	var payload struct {
		Tool string `json:"tool"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.Output), &payload))
	require.Equal(t, "read", payload.Tool)
}

func TestParseResponseRejectsOutOfStateTool(t *testing.T) {
	t.Parallel()
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "read", Visibility: core.External, Phases: []core.State{"Composing"}, PhaseScoped: true}, nil)
	reg.Register(core.ToolSpec{Name: "write", Visibility: core.External, Phases: []core.State{"Reviewing"}, PhaseScoped: true}, nil)

	res := (&ParseResponseBuilder{Registry: reg, State: "Composing", Tracer: tracing.NoopTracer{}}).
		Build(core.Result{Output: `{"tool":"write","parameters":{}}`}).
		Execute()

	require.Equal(t, core.ParseFailed, res.Signal)
	require.Contains(t, res.Output, `tool "write" is not available in state "Composing"`)
	require.Contains(t, res.Output, `available tools: [read]`)
}

func TestParseResponseUnknownToolStillReportsUnknown(t *testing.T) {
	t.Parallel()
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "read", Visibility: core.External, Phases: []core.State{"Composing"}, PhaseScoped: true}, nil)

	res := (&ParseResponseBuilder{Registry: reg, State: "Composing", Tracer: tracing.NoopTracer{}}).
		Build(core.Result{Output: `{"tool":"missing","parameters":{}}`}).
		Execute()

	require.Equal(t, core.ParseFailed, res.Signal)
	require.Contains(t, res.Output, `unknown tool "missing"`)
}

func TestParseResponsePreservesSchemaValidationForAvailableTool(t *testing.T) {
	t.Parallel()
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{
		Name:        "read",
		Visibility:  core.External,
		Phases:      []core.State{"Composing"},
		PhaseScoped: true,
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
	}, nil)

	res := (&ParseResponseBuilder{Registry: reg, State: "Composing", Tracer: tracing.NoopTracer{}}).
		Build(core.Result{Output: `{"tool":"read","parameters":{}}`}).
		Execute()

	require.Equal(t, core.ParseFailed, res.Signal)
	require.Contains(t, res.Output, `tool "read" missing required parameters: [path]`)
}
