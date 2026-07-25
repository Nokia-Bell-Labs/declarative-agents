// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/graph"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// markTaskDoneCmd owns only the successful task's graph mutation. Retry policy
// and remaining-work routing are separate machine-selected words.
type markTaskDoneCmd struct {
	ps          *State
	snapshot    pipelineSnapshot
	hasSnapshot bool
}

func (c *markTaskDoneCmd) Name() string { return "mark_task_done" }
func (c *markTaskDoneCmd) Undo(_ core.Result) core.Result {
	return undoPipelineSnapshot(c.Name(), c.ps, c.snapshot, c.hasSnapshot)
}

func (c *markTaskDoneCmd) Execute() core.Result {
	c.snapshot = snapshotPipelineState(c.ps)
	c.hasSnapshot = true
	if c.ps.CurrentTask == nil || c.ps.Graph == nil {
		err := fmt.Errorf("mark_task_done: current task and graph are required")
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: err, Output: err.Error()}
	}
	if err := c.ps.advanceTaskNodesTo(graph.Done); err != nil {
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: err, Output: err.Error()}
	}
	c.ps.Tracer.Event("pipeline.task_completed",
		attribute.String("task.id", c.ps.CurrentTask.ID),
	)
	return core.Result{
		CommandName: c.Name(),
		Signal:      SigTaskCompleted,
		Output:      fmt.Sprintf("task %s completed", c.ps.CurrentTask.ID),
	}
}

// MarkTaskDoneBuilder constructs focused graph-completion commands.
type MarkTaskDoneBuilder struct {
	PS *State
}

func (b *MarkTaskDoneBuilder) Build(_ core.Result) core.Command {
	return &markTaskDoneCmd{ps: b.PS}
}

// remainingWorkCmd is a read-only graph query. The machine owns every route
// selected from its WorkRemaining, AllDone, and Blocked signals.
type remainingWorkCmd struct {
	PS *State
}

func (c *remainingWorkCmd) Name() string { return "remaining_work" }
func (c *remainingWorkCmd) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.Name())
}

func (c *remainingWorkCmd) Execute() core.Result {
	if c.PS.Graph == nil {
		err := fmt.Errorf("remaining_work: graph is required")
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: err, Output: err.Error()}
	}
	ready := len(c.PS.Graph.Ready())
	if ready > 0 {
		return core.Result{
			CommandName: c.Name(),
			Signal:      SigWorkRemaining,
			Output:      fmt.Sprintf("%d tasks ready", ready),
		}
	}
	signal, output := c.PS.classifyEmpty()
	return core.Result{CommandName: c.Name(), Signal: signal, Output: output}
}

// RemainingWorkBuilder constructs focused read-only graph queries.
type RemainingWorkBuilder struct {
	PS *State
}

func (b *RemainingWorkBuilder) Build(_ core.Result) core.Command {
	return &remainingWorkCmd{PS: b.PS}
}
