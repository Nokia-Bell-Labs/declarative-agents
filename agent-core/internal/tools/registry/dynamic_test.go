// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package registry

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type namedBuilder struct {
	name     string
	executed *bool
}

func (b namedBuilder) Build(core.Result) core.Command {
	if b.executed != nil {
		*b.executed = true
	}
	return namedCmd{name: b.name}
}

type namedCmd struct {
	name string
}

func (c namedCmd) Name() string { return c.name }
func (c namedCmd) Execute() core.Result {
	return core.Result{Signal: core.ToolDone, CommandName: c.name}
}
func (c namedCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.name) }

type stateAwareCmd struct {
	namedCmd
	view core.CommandStateView
}

func (c *stateAwareCmd) SetCommandState(view core.CommandStateView) { c.view = view }

type tracerAwareCmd struct {
	namedCmd
	tracer tracing.Tracer
}

func (c *tracerAwareCmd) SetTracer(tracer tracing.Tracer) { c.tracer = tracer }

type receiptCmd struct {
	namedCmd
	undone core.Result
}

func (c *receiptCmd) Undo(prior core.Result) core.Result {
	c.undone = prior
	if prior.Receipt != "persisted-receipt" {
		return core.Result{Signal: core.CommandError, Err: fmt.Errorf("missing persisted receipt")}
	}
	return core.NoopUndo(c.Name())
}

// The verbose $tool wrapper must forward the engine-injected command-state view
// to a command-state-aware inner (for example invoke_llm with user_prompt_from),
// which it would otherwise hide behind its own type.
func TestTracedDynamicToolForwardsCommandState(t *testing.T) {
	inner := &stateAwareCmd{namedCmd: namedCmd{name: "invoke_llm_deep"}}
	wrapper := &tracedDynamicToolCmd{inner: inner, tracer: tracing.NoopTracer{}, toolName: "invoke_llm_deep"}
	aware, ok := core.Command(wrapper).(core.CommandStateAware)
	require.True(t, ok, "wrapper must be CommandStateAware")

	view := core.NewCommandStateView(core.Execution{})
	aware.SetCommandState(view)
	require.Equal(t, view, inner.view)
}

func TestTracedDynamicToolForwardsTracer(t *testing.T) {
	t.Parallel()
	inner := &tracerAwareCmd{namedCmd: namedCmd{name: "invoke_llm_deep"}}
	wrapper := &tracedDynamicToolCmd{
		inner: inner, tracer: tracing.NoopTracer{}, toolName: "invoke_llm_deep",
	}
	aware, ok := core.Command(wrapper).(core.TracerAware)
	require.True(t, ok, "wrapper must be TracerAware")
	injected := &tracing.NoopTracer{}

	aware.SetTracer(injected)

	require.Same(t, injected, inner.tracer)
	require.Same(t, injected, wrapper.tracer)
}

func TestTracedDynamicToolForwardsPersistedUndoResult(t *testing.T) {
	t.Parallel()
	inner := &receiptCmd{namedCmd: namedCmd{name: "write"}}
	wrapper := &tracedDynamicToolCmd{inner: inner, tracer: tracing.NoopTracer{}, toolName: "write"}
	prior := core.Result{
		Output:      "persisted output",
		Signal:      core.ToolDone,
		CommandName: "write",
		Receipt:     "persisted-receipt",
	}

	res := wrapper.Undo(prior)

	require.NoError(t, res.Err)
	require.Equal(t, core.ToolDone, res.Signal)
	require.Equal(t, prior, inner.undone)
}

func TestBuildDynamicToolActionDispatches(t *testing.T) {
	t.Parallel()
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "read"}, namedBuilder{name: "read"})
	action := BuildDynamicToolAction(DynamicToolActionDeps{Registry: reg})

	cmd := action(core.Result{Output: `{"tool":"read","parameters":{"path":"x"}}`})
	res := cmd.Execute()

	require.Equal(t, "read", cmd.Name())
	require.Equal(t, core.ToolDone, res.Signal)
}

func TestBuildDynamicToolActionUnknownToolReturnsCommandError(t *testing.T) {
	t.Parallel()
	action := BuildDynamicToolAction(DynamicToolActionDeps{Registry: core.NewRegistry()})

	res := action(core.Result{Output: `{"tool":"missing","parameters":{}}`}).Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Contains(t, res.Output, "no builder")
}

func TestBuildDynamicToolActionRejectsInternalTool(t *testing.T) {
	t.Parallel()
	reg := core.NewRegistry()
	var executed bool
	reg.Register(core.ToolSpec{Name: "launch_monitor_rest", Visibility: core.Internal}, namedBuilder{
		name: "launch_monitor_rest", executed: &executed,
	})
	action := BuildDynamicToolAction(DynamicToolActionDeps{Registry: reg})

	cmd := action(core.Result{Output: `{"tool":"launch_monitor_rest","parameters":{}}`})
	res := cmd.Execute()

	require.Equal(t, "fail", cmd.Name())
	require.Equal(t, core.CommandError, res.Signal)
	require.Contains(t, res.Output, "not available for dynamic dispatch")
	require.False(t, executed)
}

func TestBuildDynamicToolActionRejectsOutOfStateTool(t *testing.T) {
	t.Parallel()
	reg := core.NewRegistry()
	var executed bool
	reg.Register(core.ToolSpec{Name: "write", Visibility: core.External, Phases: []core.State{"Reviewing"}, PhaseScoped: true}, namedBuilder{
		name: "write", executed: &executed,
	})
	action := BuildDynamicToolAction(DynamicToolActionDeps{Registry: reg})

	cmd := action(core.Result{State: "Composing", Output: `{"tool":"write","parameters":{}}`})
	res := cmd.Execute()

	require.Equal(t, "fail", cmd.Name())
	require.Equal(t, core.CommandError, res.Signal)
	require.Contains(t, res.Output, `tool "write" is not available for dynamic dispatch in state "Composing"`)
	require.False(t, executed)
}
