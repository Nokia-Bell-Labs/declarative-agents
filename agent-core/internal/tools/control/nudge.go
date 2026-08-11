// Copyright (c) 2026 Nokia. All rights reserved.

package control

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type nudgeRereadCmd struct {
	editResult string
	nudgeText  string
	tracer     tracing.Tracer
}

func (n *nudgeRereadCmd) Name() string                   { return "nudge_reread" }
func (n *nudgeRereadCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(n.Name()) }

func (n *nudgeRereadCmd) Execute() core.Result {
	child, done := n.tracer.Push(n.Name())
	defer done()
	child.SetAttributes(attribute.String("edit_result", n.editResult))
	return core.Result{
		Signal: core.ToolDone, Output: fmt.Sprintf("%s\n\n%s", n.editResult, n.nudgeText),
		CommandName: n.Name(),
	}
}

// NudgeRereadBuilder constructs nudge_reread commands.
type NudgeRereadBuilder struct {
	Tracer tracing.Tracer
	Text   string
}

func (b *NudgeRereadBuilder) Build(r core.Result) core.Command {
	return &nudgeRereadCmd{editResult: r.Output, nudgeText: b.Text, tracer: b.Tracer}
}
