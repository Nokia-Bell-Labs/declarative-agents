// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

// CheckpointHistoryBuilder constructs checkpoint_history commands.
type CheckpointHistoryBuilder struct {
	Config     catalog.CheckpointHistoryConfig
	Checkpoint core.Checkpoint
}

func (b *CheckpointHistoryBuilder) Build(_ core.Result) core.Command {
	return &checkpointHistoryCmd{config: b.Config, checkpoint: b.Checkpoint}
}

type checkpointHistoryCmd struct {
	config     catalog.CheckpointHistoryConfig
	checkpoint core.Checkpoint
}

func (c *checkpointHistoryCmd) Name() string { return "checkpoint_history" }

func (c *checkpointHistoryCmd) Execute() core.Result {
	if c.checkpoint == nil {
		return commandError(c.Name(), fmt.Errorf("checkpoint_history requires a Checkpoint"))
	}
	pos, exec, err := c.checkpoint.Load()
	if err != nil {
		return commandError(c.Name(), err)
	}
	// Structured output matching the declared checkpoint-history schema
	// {run, history}: run echoes the selected run (explicit id or "latest"),
	// history is the stable digest (srd026 R2.1, R2.6; #493).
	out, err := json.Marshal(map[string]any{
		"run":     c.config.SelectedCheckpoint(),
		"history": core.FormatExecutionHistory(pos, exec),
	})
	if err != nil {
		return commandError(c.Name(), fmt.Errorf("encode history output: %w", err))
	}
	return core.Result{Signal: core.ToolDone, CommandName: c.Name(), Output: string(out)}
}

func (c *checkpointHistoryCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

// CheckpointRollbackBuilder constructs checkpoint_rollback commands.
type CheckpointRollbackBuilder struct {
	Config     catalog.CheckpointRollbackConfig
	Checkpoint core.CheckpointReverter
	Registry   core.CommandResolver
	RunID      string
	Tracer     tracing.Tracer
}

func (b *CheckpointRollbackBuilder) Build(_ core.Result) core.Command {
	return &checkpointRollbackCmd{
		config: b.Config, checkpoint: b.Checkpoint, registry: b.Registry,
		runID: b.RunID, tracer: b.Tracer,
	}
}

type checkpointRollbackCmd struct {
	config     catalog.CheckpointRollbackConfig
	checkpoint core.CheckpointReverter
	registry   core.CommandResolver
	runID      string
	tracer     tracing.Tracer
}

func (c *checkpointRollbackCmd) Name() string { return "checkpoint_rollback" }

// Execute rolls the run back to the target iteration in two parts: (1) the
// CheckpointReverter reverts the persisted DB state git-style to the target
// step, then (2) the reverse receipt walk reverses external effects (files,
// resources) of the entries after the target by rebuilding each tool through
// core.Reverser and calling its receipt-driven Undo (srd036 R6; #44).
func (c *checkpointRollbackCmd) Execute() core.Result {
	if c.checkpoint == nil {
		return commandError(c.Name(), fmt.Errorf("checkpoint_rollback requires a revertible Checkpoint backend"))
	}
	if !c.config.HasTargetIteration() {
		return commandError(c.Name(), fmt.Errorf("checkpoint_rollback requires to_iteration"))
	}
	_, execution, err := c.checkpoint.Load()
	if err != nil {
		return commandError(c.Name(), err)
	}
	report, err := rollbackViaReceipts(rollbackViaReceiptsOptions{
		Reverter:        c.checkpoint,
		Registry:        c.registry,
		Tracer:          c.tracer,
		RunID:           c.runID,
		Execution:       execution,
		TargetIteration: *c.config.ToIteration,
	})
	if err != nil {
		var partial *PartialRollbackError
		if errors.As(err, &partial) {
			// The DB Revert succeeded but external effects are only partly
			// reversed; report CommandError and keep the structured report plus
			// failure detail so an operator can choose retry, resume, or stop
			// (srd026 R3.7, R6.3).
			return core.Result{
				Signal:      core.CommandError,
				CommandName: c.Name(),
				Output:      rollbackOutput(report, partial),
				Err:         partial,
			}
		}
		return commandError(c.Name(), err)
	}
	return core.Result{Signal: core.ToolDone, CommandName: c.Name(), Output: rollbackOutput(report, nil)}
}

func (c *checkpointRollbackCmd) Undo(_ core.Result) core.Result {
	return undo.BoundaryCompensationUndo(c.Name(), "operator can resume from the original checkpoint or choose another rollback checkpoint")
}

func commandError(commandName string, err error) core.Result {
	return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
}
