// Copyright (c) 2026 Nokia. All rights reserved.

package registry

import (
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// ToolTracker records dynamically dispatched tool names.
type ToolTracker interface {
	Record(name string)
}

// DynamicToolActionDeps are the runtime ports for $tool dispatch.
type DynamicToolActionDeps struct {
	Registry *core.Registry
	Tracker  ToolTracker
	Tracer   tracing.Tracer
	Verbose  bool
}

// BuildDynamicToolAction builds the ActionFunc used by dynamic $tool dispatch.
func BuildDynamicToolAction(deps DynamicToolActionDeps) core.ActionFunc {
	return func(r core.Result) core.Command {
		var treq llm.ToolRequest
		if err := json.Unmarshal([]byte(r.Output), &treq); err != nil {
			return &standardFailCmd{err: fmt.Errorf("failed to unmarshal ToolRequest: %w", err)}
		}
		_, builder, availability := deps.Registry.ResolveExternalTool(treq.ToolName, r.State)
		switch availability {
		case core.ExternalToolUnknown:
			return &standardFailCmd{err: fmt.Errorf("no builder for tool %q", treq.ToolName)}
		case core.ExternalToolInternal:
			return &standardFailCmd{err: fmt.Errorf("tool %q is not available for dynamic dispatch", treq.ToolName)}
		case core.ExternalToolUnavailableInState:
			return &standardFailCmd{err: fmt.Errorf("tool %q is not available for dynamic dispatch in state %q", treq.ToolName, r.State)}
		}
		if deps.Tracker != nil {
			deps.Tracker.Record(treq.ToolName)
		}
		cmd := builder.Build(core.Result{Output: r.Output})
		if !deps.Verbose {
			return cmd
		}
		return &tracedDynamicToolCmd{inner: cmd, tracer: deps.Tracer, toolName: treq.ToolName, params: string(treq.Params)}
	}
}

type standardFailCmd struct {
	err error
}

func (f *standardFailCmd) Name() string                   { return "fail" }
func (f *standardFailCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(f.Name()) }

func (f *standardFailCmd) Execute() core.Result {
	return core.Result{Signal: core.CommandError, Err: f.err, Output: f.err.Error(), CommandName: "fail"}
}

type tracedDynamicToolCmd struct {
	inner    core.Command
	tracer   tracing.Tracer
	toolName string
	params   string
}

func (t *tracedDynamicToolCmd) Name() string                   { return t.inner.Name() }
func (t *tracedDynamicToolCmd) Undo(_ core.Result) core.Result { return t.inner.Undo(core.Result{}) }

// SetCommandState forwards the engine-injected command-state view to the wrapped
// command when it is command-state-aware, so a $tool-dispatched word (for example
// invoke_llm with user_prompt_from) still resolves its $from selectors in verbose
// mode where the wrapper would otherwise hide the interface from the engine.
func (t *tracedDynamicToolCmd) SetCommandState(view core.CommandStateView) {
	if aware, ok := t.inner.(core.CommandStateAware); ok {
		aware.SetCommandState(view)
	}
}

var _ core.CommandStateAware = (*tracedDynamicToolCmd)(nil)

func (t *tracedDynamicToolCmd) Execute() core.Result {
	child, done := t.tracer.Push("dispatch/"+t.toolName,
		attribute.String("tool.name", t.toolName),
		attribute.String("tool.params", t.params),
	)
	defer done()
	res := t.inner.Execute()
	child.SetAttributes(
		attribute.String("tool.output", llm.Truncate(res.Output, 8192)),
		attribute.String("tool.signal", string(res.Signal)),
	)
	return res
}
