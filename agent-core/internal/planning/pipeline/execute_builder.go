// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/graph"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"gopkg.in/yaml.v3"
)

const plannerTaskFilePath = "doc/task.yaml"

type markNodesExecutingCmd struct {
	ps *State
}

func (c *markNodesExecutingCmd) Name() string { return "mark_nodes_executing" }
func (c *markNodesExecutingCmd) Undo(prior core.Result) core.Result {
	return undoPipelineReceipt(c.Name(), c.ps, nil, prior.Receipt)
}

func (c *markNodesExecutingCmd) Execute() (result core.Result) {
	snapshot := snapshotPipelineState(c.ps)
	defer func() { result = withPipelineReceipt(result, snapshot, nil) }()
	if c.ps.CurrentTask == nil || c.ps.Graph == nil {
		err := fmt.Errorf("mark_nodes_executing: current task and graph are required")
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: err, Output: err.Error()}
	}
	if err := c.ps.advanceTaskNodesTo(graph.Executing); err != nil {
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: err, Output: err.Error()}
	}
	c.ps.Tracer.Event("pipeline.nodes_marked_executing")
	return core.Result{CommandName: c.Name(), Signal: SigNodesExecuting, Output: "marked current task nodes executing"}
}

// MarkNodesExecutingBuilder constructs focused graph-lifecycle commands.
type MarkNodesExecutingBuilder struct {
	PS *State
}

func (b *MarkNodesExecutingBuilder) Build(_ core.Result) core.Command {
	return &markNodesExecutingCmd{ps: b.PS}
}

func (b *MarkNodesExecutingBuilder) BuildReverser() core.Command {
	return &markNodesExecutingCmd{ps: b.PS}
}

type formatTaskFileCmd struct {
	ps *State
}

func (c *formatTaskFileCmd) Name() string { return "format_task_file" }
func (c *formatTaskFileCmd) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.Name())
}

func (c *formatTaskFileCmd) Execute() core.Result {
	if c.ps.CurrentPlan == nil {
		err := fmt.Errorf("format_task_file: current plan is required")
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: err, Output: err.Error()}
	}
	content, err := yaml.Marshal(c.ps.CurrentPlan)
	if err != nil {
		wrapped := fmt.Errorf("format_task_file: marshal plan: %w", err)
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: wrapped, Output: wrapped.Error()}
	}
	output, err := json.Marshal(map[string]any{"parameters": map[string]string{
		"path": plannerTaskFilePath, "content": string(content),
	}})
	if err != nil {
		wrapped := fmt.Errorf("format_task_file: encode write parameters: %w", err)
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: wrapped, Output: wrapped.Error()}
	}
	return core.Result{CommandName: c.Name(), Signal: SigTaskFileFormatted, Output: string(output)}
}

// FormatTaskFileBuilder constructs pure plan-to-write-request projections.
type FormatTaskFileBuilder struct {
	PS *State
}

func (b *FormatTaskFileBuilder) Build(_ core.Result) core.Command {
	return &formatTaskFileCmd{ps: b.PS}
}
