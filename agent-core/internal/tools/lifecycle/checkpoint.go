// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"context"
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

// CheckpointHistoryBuilder constructs checkpoint_history commands.
type CheckpointHistoryBuilder struct {
	Config     catalog.CheckpointHistoryConfig
	StateStore core.StateStore
	Ctx        context.Context
}

func (b *CheckpointHistoryBuilder) Build(_ core.Result) core.Command {
	return &checkpointHistoryCmd{config: b.Config, stateStore: b.StateStore, ctx: b.Ctx}
}

type checkpointHistoryCmd struct {
	config     catalog.CheckpointHistoryConfig
	stateStore core.StateStore
	ctx        context.Context
}

func (c *checkpointHistoryCmd) Name() string { return "checkpoint_history" }

func (c *checkpointHistoryCmd) Execute() core.Result {
	if c.stateStore == nil {
		return commandError(c.Name(), fmt.Errorf("checkpoint_history requires StateStore"))
	}
	checkpointID, err := core.ResolveLatestCheckpointID(c.context(), c.stateStore, c.config.SelectedCheckpoint())
	if err != nil {
		return commandError(c.Name(), err)
	}
	cp, err := core.LoadCheckpoint(c.context(), c.stateStore, checkpointID)
	if err != nil {
		return commandError(c.Name(), err)
	}
	return core.Result{Signal: core.ToolDone, CommandName: c.Name(), Output: core.FormatCheckpointHistory(cp)}
}

func (c *checkpointHistoryCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }
func (c *checkpointHistoryCmd) UndoMemento() (core.UndoMemento, error) {
	return core.NoopUndoMemento(c.Name()), nil
}

// CheckpointRollbackBuilder constructs checkpoint_rollback commands.
type CheckpointRollbackBuilder struct {
	Config     catalog.CheckpointRollbackConfig
	StateStore core.StateStore
	Workspace  core.Workspace
	Directory  string
	Tracer     tracing.Tracer
	Ctx        context.Context
}

func (b *CheckpointRollbackBuilder) Build(_ core.Result) core.Command {
	return &checkpointRollbackCmd{
		config: b.Config, stateStore: b.StateStore, workspace: b.Workspace,
		directory: b.Directory, tracer: b.Tracer, ctx: b.Ctx,
	}
}

type checkpointRollbackCmd struct {
	config     catalog.CheckpointRollbackConfig
	stateStore core.StateStore
	workspace  core.Workspace
	directory  string
	tracer     tracing.Tracer
	ctx        context.Context
}

func (c *checkpointRollbackCmd) Name() string { return "checkpoint_rollback" }

func (c *checkpointRollbackCmd) Execute() core.Result {
	workspace, err := c.prepareRollback()
	if err != nil {
		return commandError(c.Name(), err)
	}
	result, err := core.RollbackFromCheckpoint(core.RollbackFromCheckpointOptions{
		Store: c.stateStore, Workspace: workspace, CheckpointID: c.config.SelectedCheckpoint(),
		TargetIteration: *c.config.ToIteration, Ctx: c.context(),
	})
	if err != nil {
		return commandError(c.Name(), err)
	}
	return core.Result{Signal: core.ToolDone, CommandName: c.Name(), Output: c.rollbackSummary(result)}
}

func (c *checkpointRollbackCmd) Undo() core.Result {
	return undo.BoundaryCompensationUndo(c.Name(), "operator can resume from the original checkpoint or choose another rollback checkpoint")
}
func (c *checkpointRollbackCmd) UndoMemento() (core.UndoMemento, error) {
	payload := undo.BoundaryCompensationPayload{BoundaryCompensation: undo.BoundaryCompensation{
		Strategy: "operator_checkpoint_selection",
		Reason:   "rollback rewrites checkpoint state and may restore workspace state",
		Requires: []string{"checkpoint_id", "operator_decision"},
	}}
	return undo.BoundaryCompensationMemento(c.Name(), payload, "operator can resume from the original checkpoint or choose another rollback checkpoint")
}

func (c *checkpointRollbackCmd) prepareRollback() (core.Workspace, error) {
	if c.stateStore == nil {
		return nil, fmt.Errorf("checkpoint_rollback requires StateStore")
	}
	if !c.config.HasTargetIteration() {
		return nil, fmt.Errorf("checkpoint_rollback requires to_iteration")
	}
	workspace, err := c.configuredWorkspace()
	if err != nil || workspace != nil {
		return workspace, err
	}
	return nil, c.requireWorkspaceIfNeeded()
}

func (c *checkpointRollbackCmd) configuredWorkspace() (core.Workspace, error) {
	if c.workspace != nil {
		return c.workspace, nil
	}
	directory := c.config.Directory
	if directory == "" {
		directory = c.directory
	}
	if directory == "" {
		return nil, nil
	}
	return core.NewGitWorkspace(directory)
}

func (c *checkpointRollbackCmd) requireWorkspaceIfNeeded() error {
	result, err := c.previewRollback()
	if err != nil || result.WorkspaceRef == "" {
		return err
	}
	return fmt.Errorf("rollback target has workspace ref %q; directory is required for managed workspace restore", result.WorkspaceRef)
}

func (c *checkpointRollbackCmd) previewRollback() (core.RollbackCheckpointResult, error) {
	checkpointID, err := core.ResolveLatestCheckpointID(c.context(), c.stateStore, c.config.SelectedCheckpoint())
	if err != nil {
		return core.RollbackCheckpointResult{}, err
	}
	cp, err := core.LoadCheckpoint(c.context(), c.stateStore, checkpointID)
	if err != nil {
		return core.RollbackCheckpointResult{}, err
	}
	return core.RollbackCheckpoint(cp, *c.config.ToIteration)
}

func (c *checkpointRollbackCmd) rollbackSummary(result core.RollbackFromCheckpointResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "rolled back checkpoint %s to iteration %d\n", result.Original.ID, *c.config.ToIteration)
	fmt.Fprintf(&b, "new checkpoint: %s\n", result.Checkpoint.ID)
	if result.WorkspaceRef != "" {
		fmt.Fprintf(&b, "workspace ref: %s\n", result.WorkspaceRef)
	}
	return b.String()
}

func (c *checkpointHistoryCmd) context() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func (c *checkpointRollbackCmd) context() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func commandError(commandName string, err error) core.Result {
	return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
}
