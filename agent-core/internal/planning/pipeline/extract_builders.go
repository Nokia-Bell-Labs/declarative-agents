// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/extract"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/graph"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/plan"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type extractTaskCmd struct {
	ps *State
}

func (c *extractTaskCmd) Name() string { return "extract_task" }
func (c *extractTaskCmd) Undo(prior core.Result) core.Result {
	return undoPipelineReceipt(c.Name(), c.ps, nil, prior.Receipt)
}

func (c *extractTaskCmd) Execute() (result core.Result) {
	snapshot := snapshotPipelineState(c.ps)
	defer func() { result = withPipelineReceipt(result, snapshot, nil) }()
	task := c.ps.Extractor.ExtractNext(c.ps.Graph, c.ps.MaxWeight)
	if task == nil {
		return core.Result{CommandName: c.Name(), Signal: SigNoTask, Output: "no task ready for extraction"}
	}
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

func (b *ExtractTaskBuilder) BuildReverser() core.Command {
	return &extractTaskCmd{ps: b.PS}
}

type selectAllReadyCmd struct {
	ps *State
}

func (c *selectAllReadyCmd) Name() string { return "select_all_ready" }
func (c *selectAllReadyCmd) Undo(prior core.Result) core.Result {
	return undoPipelineReceipt(c.Name(), c.ps, nil, prior.Receipt)
}

func (c *selectAllReadyCmd) Execute() (result core.Result) {
	snapshot := snapshotPipelineState(c.ps)
	defer func() { result = withPipelineReceipt(result, snapshot, nil) }()
	ready := c.ps.Graph.Ready()
	if len(ready) == 0 {
		return core.Result{CommandName: c.Name(), Signal: SigNoTask, Output: "no task ready for extraction"}
	}
	c.ps.CurrentTask = allReadyTask(ready)
	c.ps.Tracer.Event("pipeline.ready_selected",
		attribute.Int("task.node_count", len(ready)),
	)
	return core.Result{
		CommandName: c.Name(),
		Signal:      SigReadySelected,
		Output:      fmt.Sprintf("selected all %d ready nodes", len(ready)),
	}
}

func allReadyTask(ready []*graph.Node) *extract.Task {
	nodeIDs := make([]string, len(ready))
	for i, n := range ready {
		nodeIDs[i] = n.ID
	}
	return &extract.Task{ID: "all", NodeIDs: nodeIDs, Weight: len(nodeIDs), SRDID: ready[0].SRDID}
}

// SelectAllReadyBuilder constructs focused aggregate-selection commands.
type SelectAllReadyBuilder struct {
	PS *State
}

func (b *SelectAllReadyBuilder) Build(_ core.Result) core.Command {
	return &selectAllReadyCmd{ps: b.PS}
}

func (b *SelectAllReadyBuilder) BuildReverser() core.Command {
	return &selectAllReadyCmd{ps: b.PS}
}

type seedPassThroughPlanCmd struct {
	ps      *State
	title   string
	summary string
}

func (c *seedPassThroughPlanCmd) Name() string { return "seed_passthrough_plan" }
func (c *seedPassThroughPlanCmd) Undo(prior core.Result) core.Result {
	return undoPipelineReceipt(c.Name(), c.ps, nil, prior.Receipt)
}

func (c *seedPassThroughPlanCmd) Execute() (result core.Result) {
	snapshot := snapshotPipelineState(c.ps)
	defer func() { result = withPipelineReceipt(result, snapshot, nil) }()
	if c.ps.CurrentTask == nil {
		err := fmt.Errorf("seed_passthrough_plan: current task is required")
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: err, Output: err.Error()}
	}
	c.ps.CurrentPlan = &plan.ImplementationPlan{Title: c.title, Summary: c.summary}
	c.ps.Tracer.Event("pipeline.passthrough_plan_seeded",
		attribute.String("task.id", c.ps.CurrentTask.ID),
	)
	return core.Result{CommandName: c.Name(), Signal: SigPlanSeeded, Output: "seeded pass-through implementation plan"}
}

// SeedPassThroughPlanBuilder constructs profile-configured plan seeding commands.
type SeedPassThroughPlanBuilder struct {
	PS      *State
	Title   string
	Summary string
}

func (b *SeedPassThroughPlanBuilder) Build(_ core.Result) core.Command {
	return &seedPassThroughPlanCmd{ps: b.PS, title: b.Title, summary: b.Summary}
}

func (b *SeedPassThroughPlanBuilder) BuildReverser() core.Command {
	return &seedPassThroughPlanCmd{ps: b.PS, title: b.Title, summary: b.Summary}
}

type markNodesPlanningCmd struct {
	ps *State
}

func (c *markNodesPlanningCmd) Name() string { return "mark_nodes_planning" }
func (c *markNodesPlanningCmd) Undo(prior core.Result) core.Result {
	return undoPipelineReceipt(c.Name(), c.ps, nil, prior.Receipt)
}

func (c *markNodesPlanningCmd) Execute() (result core.Result) {
	snapshot := snapshotPipelineState(c.ps)
	defer func() { result = withPipelineReceipt(result, snapshot, nil) }()
	if c.ps.CurrentTask == nil {
		err := fmt.Errorf("mark_nodes_planning: current task is required")
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: err, Output: err.Error()}
	}
	if err := c.ps.advanceTaskNodesTo(graph.Planning); err != nil {
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: err, Output: err.Error()}
	}
	c.ps.Tracer.Event("pipeline.nodes_marked_planning",
		attribute.String("task.id", c.ps.CurrentTask.ID),
		attribute.Int("task.node_count", len(c.ps.CurrentTask.NodeIDs)),
	)
	return core.Result{CommandName: c.Name(), Signal: SigNodesPlanning, Output: "marked selected nodes planning"}
}

// MarkNodesPlanningBuilder constructs focused graph-lifecycle commands.
type MarkNodesPlanningBuilder struct {
	PS *State
}

func (b *MarkNodesPlanningBuilder) Build(_ core.Result) core.Command {
	return &markNodesPlanningCmd{ps: b.PS}
}

func (b *MarkNodesPlanningBuilder) BuildReverser() core.Command {
	return &markNodesPlanningCmd{ps: b.PS}
}
