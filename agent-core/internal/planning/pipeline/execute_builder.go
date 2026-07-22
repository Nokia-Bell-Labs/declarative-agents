// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/graph"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/execute"
)

type executeTaskCmd struct {
	ps  *State
	run func(*State) (*execute.Result, error)
}

func (c *executeTaskCmd) Name() string { return "execute_task" }
func (c *executeTaskCmd) Undo(_ core.Result) core.Result {
	err := fmt.Errorf("undo execute_task requires child agent history or workspace compensation")
	return core.Result{Signal: core.CommandError, CommandName: c.Name(), Output: err.Error(), Err: err}
}

func (c *executeTaskCmd) Execute() core.Result {
	if c.ps.CurrentTask == nil || c.ps.CurrentPlan == nil {
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Output: "no current task or plan"}
	}
	if err := c.ps.advanceTaskNodesTo(graph.Executing); err != nil {
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: err, Output: err.Error()}
	}
	run := c.run
	if run == nil {
		run = runExecuteTask
	}
	result, err := run(c.ps)
	if err != nil {
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: err, Output: err.Error()}
	}
	return c.executionResult(result)
}

func runExecuteTask(ps *State) (*execute.Result, error) {
	return execute.Execute(ps.Ctx, ps.Tracer, ps.ExecConfig, ps.CurrentTask.ID, ps.Directory, ps.CurrentPlan)
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
	PS  *State
	run func(*State) (*execute.Result, error)
}

func (b *ExecuteTaskBuilder) Build(_ core.Result) core.Command {
	return &executeTaskCmd{ps: b.PS, run: b.run}
}
