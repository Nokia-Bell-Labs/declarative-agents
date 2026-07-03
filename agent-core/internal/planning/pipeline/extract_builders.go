// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/extract"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/graph"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type extractTaskCmd struct {
	ps          *State
	snapshot    pipelineSnapshot
	hasSnapshot bool
}

func (c *extractTaskCmd) Name() string { return "extract_task" }
func (c *extractTaskCmd) Undo(_ core.Result) core.Result {
	return undoPipelineSnapshot(c.Name(), c.ps, c.snapshot, c.hasSnapshot)
}

func (c *extractTaskCmd) UndoMemento() (core.UndoMemento, error) {
	return mementoPipelineSnapshot(c.Name(), c.snapshot, c.hasSnapshot, core.UndoMementoReversible)
}

func (c *extractTaskCmd) Execute() core.Result {
	c.snapshot = snapshotPipelineState(c.ps)
	c.hasSnapshot = true
	task := c.ps.Extractor.ExtractNext(c.ps.Graph, c.ps.MaxWeight)
	if task == nil {
		sig, msg := c.ps.classifyEmpty()
		return core.Result{CommandName: c.Name(), Signal: sig, Output: msg}
	}
	c.ps.retryCount = 0
	c.ps.CurrentTask = task
	c.ps.Tracer.Event("pipeline.task_extracted",
		attribute.String("task.id", task.ID),
		attribute.String("task.srd_id", task.SRDID),
		attribute.Int("task.weight", task.Weight),
		attribute.Int("task.node_count", len(task.NodeIDs)),
	)
	return core.Result{
		CommandName: c.Name(),
		Signal:      SigTaskExtracted,
		Output:      fmt.Sprintf("extracted task %s (weight=%d, nodes=%d)", task.ID, task.Weight, len(task.NodeIDs)),
	}
}

// ExtractTaskBuilder constructs extract_task commands.
type ExtractTaskBuilder struct {
	PS *State
}

func (b *ExtractTaskBuilder) Build(_ core.Result) core.Command {
	return &extractTaskCmd{ps: b.PS}
}

type extractAllCmd struct {
	ps          *State
	snapshot    pipelineSnapshot
	hasSnapshot bool
}

func (c *extractAllCmd) Name() string { return "extract_all" }
func (c *extractAllCmd) Undo(_ core.Result) core.Result {
	return undoPipelineSnapshot(c.Name(), c.ps, c.snapshot, c.hasSnapshot)
}

func (c *extractAllCmd) UndoMemento() (core.UndoMemento, error) {
	return mementoPipelineSnapshot(c.Name(), c.snapshot, c.hasSnapshot, core.UndoMementoReversible)
}

func (c *extractAllCmd) Execute() core.Result {
	c.snapshot = snapshotPipelineState(c.ps)
	c.hasSnapshot = true
	ready := c.ps.Graph.Ready()
	if len(ready) == 0 {
		sig, msg := c.ps.classifyEmpty()
		return core.Result{CommandName: c.Name(), Signal: sig, Output: msg}
	}
	c.ps.retryCount = 0
	c.ps.CurrentTask = allReadyTask(ready)
	return core.Result{
		CommandName: c.Name(),
		Signal:      SigTaskExtracted,
		Output:      fmt.Sprintf("extracted all %d nodes as single task", len(ready)),
	}
}

func allReadyTask(ready []*graph.Node) *extract.Task {
	nodeIDs := make([]string, len(ready))
	for i, n := range ready {
		nodeIDs[i] = n.ID
	}
	return &extract.Task{ID: "all", NodeIDs: nodeIDs, Weight: len(nodeIDs), SRDID: ready[0].SRDID}
}

// ExtractAllBuilder constructs extract_all commands.
type ExtractAllBuilder struct {
	PS *State
}

func (b *ExtractAllBuilder) Build(_ core.Result) core.Command {
	return &extractAllCmd{ps: b.PS}
}
