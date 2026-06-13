// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// CheckpointHistoryBuilder constructs checkpoint_history commands.
type CheckpointHistoryBuilder struct {
	Config     CheckpointHistoryConfig
	StateStore core.StateStore
	Ctx        context.Context
}

func (b *CheckpointHistoryBuilder) Build(_ core.Result) core.Command {
	return &checkpointHistoryCmd{
		config:     b.Config,
		stateStore: b.StateStore,
		ctx:        b.Ctx,
	}
}

type checkpointHistoryCmd struct {
	config     CheckpointHistoryConfig
	stateStore core.StateStore
	ctx        context.Context
}

func (c *checkpointHistoryCmd) Name() string { return "checkpoint_history" }

func (c *checkpointHistoryCmd) Execute() core.Result {
	if c.stateStore == nil {
		return lifecycleCommandError(c.Name(), fmt.Errorf("checkpoint_history requires StateStore"))
	}
	checkpointID, err := core.ResolveLatestCheckpointID(c.context(), c.stateStore, c.config.SelectedCheckpoint())
	if err != nil {
		return lifecycleCommandError(c.Name(), err)
	}
	cp, err := core.LoadCheckpoint(c.context(), c.stateStore, checkpointID)
	if err != nil {
		return lifecycleCommandError(c.Name(), err)
	}
	return core.Result{
		Signal:      core.ToolDone,
		CommandName: c.Name(),
		Output:      core.FormatCheckpointHistory(cp),
	}
}

func (c *checkpointHistoryCmd) Undo() core.Result {
	return core.NoopUndo(c.Name())
}

func (c *checkpointHistoryCmd) UndoMemento() (core.UndoMemento, error) {
	return core.NoopUndoMemento(c.Name()), nil
}

func (c *checkpointHistoryCmd) context() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func lifecycleCommandError(commandName string, err error) core.Result {
	return core.Result{
		Signal:      core.CommandError,
		CommandName: commandName,
		Output:      err.Error(),
		Err:         err,
	}
}
