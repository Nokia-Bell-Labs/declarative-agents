// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

type checkResultCmd struct {
	ps          *State
	prevRes     core.Result
	snapshot    pipelineSnapshot
	hasSnapshot bool
}

func (c *checkResultCmd) Name() string { return "check_result" }
func (c *checkResultCmd) Undo() core.Result {
	return undoPipelineSnapshot(c.Name(), c.ps, c.snapshot, c.hasSnapshot)
}

func (c *checkResultCmd) UndoMemento() (core.UndoMemento, error) {
	return mementoPipelineSnapshot(c.Name(), c.snapshot, c.hasSnapshot, core.UndoMementoReversible)
}

func (c *checkResultCmd) Execute() core.Result {
	c.snapshot = snapshotPipelineState(c.ps)
	c.hasSnapshot = true
	if c.prevRes.Signal == core.ToolFailed || c.prevRes.Signal == core.CommandError {
		return c.retryResult()
	}
	c.markTaskDone()
	remaining := len(c.ps.Graph.Ready())
	msg := fmt.Sprintf("task completed; %d tasks remaining", remaining)
	if remaining == 0 {
		msg = "all tasks completed"
	}
	return core.Result{CommandName: c.Name(), Signal: core.ValidationPassed, Output: msg}
}

func (c *checkResultCmd) retryResult() core.Result {
	c.ps.retryCount++
	maxRetries := c.ps.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 2
	}
	if c.ps.retryCount >= maxRetries {
		c.ps.Tracer.Event("pipeline.retries_exhausted",
			attribute.String("task.id", c.ps.currentTaskID()),
			attribute.Int("retries", c.ps.retryCount),
		)
		return core.Result{
			CommandName: c.Name(),
			Signal:      SigRetriesExhausted,
			Output:      fmt.Sprintf("retries exhausted (%d/%d): %s", c.ps.retryCount, maxRetries, c.prevRes.Output),
		}
	}
	c.ps.Tracer.Event("pipeline.retry_available",
		attribute.String("task.id", c.ps.currentTaskID()),
		attribute.Int("retry", c.ps.retryCount),
		attribute.Int("max_retries", maxRetries),
	)
	return core.Result{
		CommandName: c.Name(),
		Signal:      SigRetryAvailable,
		Output:      fmt.Sprintf("retry %d/%d: %s", c.ps.retryCount, maxRetries, c.prevRes.Output),
	}
}

func (c *checkResultCmd) markTaskDone() {
	if c.ps.CurrentTask == nil || c.ps.Graph == nil {
		return
	}
	for _, nid := range c.ps.CurrentTask.NodeIDs {
		if n, _ := c.ps.Graph.Node(nid); n != nil {
			_ = n.MarkDone()
		}
	}
	c.ps.Tracer.Event("pipeline.task_completed",
		attribute.String("task.id", c.ps.CurrentTask.ID),
		attribute.Int("remaining", len(c.ps.Graph.Ready())),
	)
}

// CheckResultBuilder constructs check_result commands.
type CheckResultBuilder struct {
	PS *State
}

func (b *CheckResultBuilder) Build(res core.Result) core.Command {
	return &checkResultCmd{ps: b.PS, prevRes: res}
}
