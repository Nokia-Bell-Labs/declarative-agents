// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package registry_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

type namedCmd struct {
	name string
}

func (c namedCmd) Build(core.Result) core.Command { return c }
func (c namedCmd) Name() string                   { return c.name }
func (c namedCmd) Execute() core.Result {
	return core.Result{Signal: core.ToolDone, CommandName: c.name}
}
func (c namedCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.name) }

func TestDynamicToolActionAndParseResponseShareAvailabilityRule(t *testing.T) {
	t.Parallel()
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "read", Visibility: core.External, Phases: []core.State{"Composing"}, PhaseScoped: true}, namedCmd{name: "read"})
	reg.Register(core.ToolSpec{Name: "write", Visibility: core.External, Phases: []core.State{"Reviewing"}, PhaseScoped: true}, namedCmd{name: "write"})
	_, _, availability := reg.ResolveExternalTool("write", "Composing")
	require.Equal(t, core.ExternalToolUnavailableInState, availability)

	parseRes := (&toollm.ParseResponseBuilder{Registry: reg, State: "Composing", Tracer: tracing.NoopTracer{}}).
		Build(core.Result{Output: `{"tool":"write","parameters":{}}`}).
		Execute()
	dynamicRes := registry.BuildDynamicToolAction(registry.DynamicToolActionDeps{Registry: reg})(
		core.Result{State: "Composing", Output: `{"tool":"write","parameters":{}}`},
	).Execute()

	require.Equal(t, core.ParseFailed, parseRes.Signal)
	require.Contains(t, parseRes.Output, `tool "write" is not available in state "Composing"`)
	require.Contains(t, parseRes.Output, `available tools: [read]`)
	require.Equal(t, core.CommandError, dynamicRes.Signal)
	require.Contains(t, dynamicRes.Output, `tool "write" is not available for dynamic dispatch in state "Composing"`)
}
