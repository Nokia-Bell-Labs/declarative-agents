// Copyright (c) 2026 Nokia. All rights reserved.

package control

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// RereadNudge is appended after edits so the model reads current file content.
const RereadNudge = `The file was modified successfully. ` +
	`IMPORTANT: You MUST call the read tool on the modified file to see ` +
	`its current contents before making any further edits. Do not assume ` +
	`you know what the file looks like — re-read it now.`

type nudgeRereadCmd struct {
	editResult string
	tracer     tracing.Tracer
}

func (n *nudgeRereadCmd) Name() string      { return "nudge_reread" }
func (n *nudgeRereadCmd) Undo() core.Result { return core.NoopUndo(n.Name()) }

func (n *nudgeRereadCmd) Execute() core.Result {
	child, done := n.tracer.Push(n.Name())
	defer done()
	child.SetAttributes(attribute.String("edit_result", n.editResult))
	return core.Result{
		Signal: core.ToolDone, Output: fmt.Sprintf("%s\n\n%s", n.editResult, RereadNudge),
		CommandName: n.Name(),
	}
}

// NudgeRereadBuilder constructs nudge_reread commands.
type NudgeRereadBuilder struct {
	Tracer tracing.Tracer
}

func (b *NudgeRereadBuilder) Build(r core.Result) core.Command {
	return &nudgeRereadCmd{editResult: r.Output, tracer: b.Tracer}
}
