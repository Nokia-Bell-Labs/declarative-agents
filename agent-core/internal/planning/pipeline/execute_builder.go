// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/support/execute"
)

type executeTaskCmd struct {
	ps *State
}

func (c *executeTaskCmd) Name() string { return "execute_task" }
func (c *executeTaskCmd) Undo() core.Result {
	err := fmt.Errorf("undo execute_task requires child agent history or workspace compensation")
	return core.Result{Signal: core.CommandError, CommandName: c.Name(), Output: err.Error(), Err: err}
}

func (c *executeTaskCmd) UndoMemento() (core.UndoMemento, error) {
	currentTaskID := ""
	if c.ps.CurrentTask != nil {
		currentTaskID = c.ps.CurrentTask.ID
	}
	memento, err := core.NewUndoMemento(c.Name(), core.UndoMementoCompensatable, struct {
		BoundaryCompensation BoundaryCompensationInfo `json:"boundary_compensation"`
	}{
		BoundaryCompensation: BoundaryCompensationInfo{
			Strategy:       "child_agent_workspace_restore",
			Reason:         "execute_task runs the generator agent for a planner task",
			Requires:       []string{"child_history", "Workspace"},
			WorkspacePaths: []string{c.ps.Directory},
			ChildRunID:     currentTaskID,
		},
	})
	if err != nil {
		return core.UndoMemento{}, err
	}
	memento.Description = "restore or compensate child generator workspace effects"
	return memento, nil
}

func (c *executeTaskCmd) Execute() core.Result {
	if c.ps.CurrentTask == nil || c.ps.CurrentPlan == nil {
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Output: "no current task or plan"}
	}
	result, err := execute.Execute(
		c.ps.Ctx,
		c.ps.Tracer,
		c.ps.ExecConfig,
		c.ps.CurrentTask.ID,
		c.ps.Directory,
		c.ps.CurrentPlan,
	)
	if err != nil {
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: err, Output: err.Error()}
	}
	return c.executionResult(result)
}

func (c *executeTaskCmd) executionResult(result *execute.Result) core.Result {
	signal := SigExecutionDone
	output := result.Stdout
	if !result.Success() {
		signal = SigExecutionFailed
		output = fmt.Sprintf("exit %d\nstdout: %s\nstderr: %s",
			result.ExitCode,
			llm.Truncate(result.Stdout, 2000),
			llm.Truncate(result.Stderr, 2000),
		)
	}
	return core.Result{CommandName: c.Name(), Signal: signal, Output: output, Cost: core.Cost{Duration: result.Duration}}
}

// ExecuteTaskBuilder constructs execute_task commands.
type ExecuteTaskBuilder struct {
	PS *State
}

func (b *ExecuteTaskBuilder) Build(_ core.Result) core.Command {
	return &executeTaskCmd{ps: b.PS}
}
