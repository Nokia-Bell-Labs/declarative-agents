// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRollbackCallsUndoInReverseOrder(t *testing.T) {
	t.Parallel()
	var order []string
	history := History{
		rollbackHistoryEntry(1, "one", "Start", "One", "ref1", rollbackCommand("one", func() Result {
			order = append(order, "one")
			return NoopUndo("one")
		})),
		rollbackHistoryEntry(2, "two", "One", "Two", "ref2", rollbackCommand("two", func() Result {
			order = append(order, "two")
			return NoopUndo("two")
		})),
		rollbackHistoryEntry(3, "three", "Two", "Three", "ref3", rollbackCommand("three", func() Result {
			order = append(order, "three")
			return NoopUndo("three")
		})),
	}

	result, err := Rollback(history, 1, nil, nil)

	require.NoError(t, err)
	require.Equal(t, State("One"), result.State)
	require.Equal(t, []string{"three", "two"}, order)
	require.Len(t, result.Undone, 2)
	require.False(t, result.Partial)
}

func TestRollbackRestoresTargetWorkspaceRef(t *testing.T) {
	t.Parallel()
	ws := &recordingWorkspace{}
	history := History{
		rollbackHistoryEntry(1, "one", "Start", "One", "ref1", rollbackCommand("one", func() Result {
			return NoopUndo("one")
		})),
		rollbackHistoryEntry(2, "two", "One", "Two", "ref2", rollbackCommand("two", func() Result {
			return NoopUndo("two")
		})),
	}

	result, err := Rollback(history, 1, ws, nil)

	require.NoError(t, err)
	require.Equal(t, State("One"), result.State)
	require.Equal(t, "ref1", result.WorkspaceRef)
	require.Equal(t, []string{"ref1"}, ws.restored)
}

func TestRollbackToIterationZeroUsesInitialStateAndWorkspaceRef(t *testing.T) {
	t.Parallel()
	ws := &recordingWorkspace{}
	var order []string
	history := History{
		rollbackHistoryEntry(1, "one", "Start", "One", "ref1", rollbackCommand("one", func() Result {
			order = append(order, "one")
			return NoopUndo("one")
		})),
		rollbackHistoryEntry(2, "two", "One", "Two", "ref2", rollbackCommand("two", func() Result {
			order = append(order, "two")
			return NoopUndo("two")
		})),
	}

	result, err := RollbackTo(RollbackOptions{
		History:             history,
		TargetIteration:     0,
		Workspace:           ws,
		InitialWorkspaceRef: "initial-ref",
	})

	require.NoError(t, err)
	require.Equal(t, State("Start"), result.State)
	require.Equal(t, "initial-ref", result.WorkspaceRef)
	require.Equal(t, []string{"two", "one"}, order)
	require.Equal(t, []string{"initial-ref"}, ws.restored)
}

func TestRollbackReportsPartialFailureAndContinues(t *testing.T) {
	t.Parallel()
	ws := &recordingWorkspace{err: fmt.Errorf("restore failed")}
	var order []string
	history := History{
		rollbackHistoryEntry(1, "one", "Start", "One", "ref1", rollbackCommand("one", func() Result {
			order = append(order, "one")
			return NoopUndo("one")
		})),
		rollbackHistoryEntry(2, "two", "One", "Two", "ref2", rollbackCommand("two", func() Result {
			order = append(order, "two")
			return Result{Signal: CommandError, CommandName: "two", Err: fmt.Errorf("undo failed"), Output: "undo failed"}
		})),
		rollbackHistoryEntry(3, "three", "Two", "Three", "ref3", nil),
	}

	result, err := Rollback(history, 1, ws, nil)

	require.Error(t, err)
	require.True(t, result.Partial)
	require.Equal(t, []string{"two"}, order)
	require.Len(t, result.Undone, 2)
	require.Contains(t, err.Error(), "missing command")
	require.Contains(t, err.Error(), "undo failed")
	require.Contains(t, err.Error(), "restore failed")
	require.Equal(t, []string{"ref1"}, ws.restored)
}

func TestRollbackCombinesInMemoryUndoAndGitWorkspaceRestore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := initGitWorkspaceRepo(t)
	ws, err := NewGitWorkspace(repo)
	require.NoError(t, err)

	filePath := filepath.Join(repo, "file.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("v1\n"), 0o644))
	ref1, err := ws.Checkpoint(ctx, "iteration-1")
	require.NoError(t, err)

	state := []string{"after-1", "after-2"}
	cmd1 := rollbackCommand("cmd1", func() Result {
		state = state[:0]
		return NoopUndo("cmd1")
	})
	cmd2 := rollbackCommand("cmd2", func() Result {
		state = state[:1]
		return NoopUndo("cmd2")
	})

	require.NoError(t, os.WriteFile(filePath, []byte("v2\n"), 0o644))
	ref2, err := ws.Checkpoint(ctx, "iteration-2")
	require.NoError(t, err)
	require.NotEqual(t, ref1, ref2)

	history := History{
		rollbackHistoryEntry(1, "cmd1", "Start", "One", ref1, cmd1),
		rollbackHistoryEntry(2, "cmd2", "One", "Two", ref2, cmd2),
	}

	result, err := RollbackTo(RollbackOptions{
		History:         history,
		TargetIteration: 1,
		Workspace:       ws,
		Ctx:             ctx,
	})

	require.NoError(t, err)
	require.Equal(t, State("One"), result.State)
	require.Equal(t, []string{"after-1"}, state)
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, "v1\n", string(data))
	current, err := ws.CurrentRef(ctx)
	require.NoError(t, err)
	require.Equal(t, ref1, current)
}

func rollbackHistoryEntry(iteration int, name string, from, to State, ref string, cmd Command) HistoryEntry {
	return HistoryEntry{
		Iteration:    iteration,
		Command:      cmd,
		CommandName:  name,
		FromState:    from,
		ToState:      to,
		Signal:       ToolDone,
		Result:       ResultDigest{Signal: ToolDone},
		WorkspaceRef: ref,
	}
}

func rollbackCommand(name string, undo func() Result) Command {
	return &rollbackFuncCmd{name: name, undo: undo}
}

type rollbackFuncCmd struct {
	name string
	undo func() Result
}

func (r *rollbackFuncCmd) Name() string    { return r.name }
func (r *rollbackFuncCmd) Execute() Result { return Result{Signal: ToolDone, CommandName: r.name} }
func (r *rollbackFuncCmd) Undo() Result    { return r.undo() }

type recordingWorkspace struct {
	restored []string
	err      error
}

func (r *recordingWorkspace) Checkpoint(context.Context, string) (string, error) {
	return "", r.err
}

func (r *recordingWorkspace) Restore(_ context.Context, ref string) error {
	r.restored = append(r.restored, ref)
	return r.err
}

func (r *recordingWorkspace) CurrentRef(context.Context) (string, error) {
	return "", r.err
}
