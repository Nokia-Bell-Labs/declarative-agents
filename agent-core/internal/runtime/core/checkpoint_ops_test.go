// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResolveLatestCheckpointIDReturnsExplicitID(t *testing.T) {
	t.Parallel()
	id, err := ResolveLatestCheckpointID(context.Background(), nil, "cp-42")
	require.NoError(t, err)
	require.Equal(t, "cp-42", id)
}

func TestResolveLatestCheckpointIDResolvesLatest(t *testing.T) {
	t.Parallel()
	store := &memoryStateStore{}
	saveCheckpointForOps(t, store, CheckpointRecord{ID: "older", Timestamp: time.Unix(100, 0).UTC()})
	saveCheckpointForOps(t, store, CheckpointRecord{ID: "newer", Timestamp: time.Unix(200, 0).UTC()})

	id, err := ResolveLatestCheckpointID(context.Background(), store, "latest")
	require.NoError(t, err)
	require.Equal(t, "newer", id)
}

func TestResolveLatestCheckpointIDEmptyStringMeansLatest(t *testing.T) {
	t.Parallel()
	store := &memoryStateStore{}
	saveCheckpointForOps(t, store, CheckpointRecord{ID: "only", Timestamp: time.Unix(100, 0).UTC()})

	id, err := ResolveLatestCheckpointID(context.Background(), store, "")
	require.NoError(t, err)
	require.Equal(t, "only", id)
}

func TestResolveLatestCheckpointIDNoCheckpoints(t *testing.T) {
	t.Parallel()
	store := &memoryStateStore{}

	_, err := ResolveLatestCheckpointID(context.Background(), store, "latest")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no checkpoints found")
}

func TestFormatCheckpointHistoryWithEntries(t *testing.T) {
	t.Parallel()
	cp := CheckpointRecord{
		ID:        "cp-1",
		Iteration: 2,
		AgentState: AgentSnapshot{
			State: "Working",
		},
		WorkspaceRef: "ref-2",
		History: []HistoryDigest{
			{Iteration: 1, CommandName: "read", FromState: "Start", ToState: "Reading", Signal: ToolDone, WorkspaceRef: "ref-1"},
			{Iteration: 2, CommandName: "write", FromState: "Reading", ToState: "Working", Signal: EditDone, WorkspaceRef: "ref-2"},
		},
	}

	out := FormatCheckpointHistory(cp)

	require.Contains(t, out, "checkpoint: cp-1")
	require.Contains(t, out, "iteration: 2")
	require.Contains(t, out, "state: Working")
	require.Contains(t, out, "workspace_ref: ref-2")
	require.Contains(t, out, "1  read  Start -> Reading  signal=ToolDone  workspace=ref-1")
	require.Contains(t, out, "2  write  Reading -> Working  signal=EditDone  workspace=ref-2")
}

func TestFormatCheckpointHistoryEmpty(t *testing.T) {
	t.Parallel()
	cp := CheckpointRecord{
		ID:        "cp-empty",
		Iteration: 0,
		AgentState: AgentSnapshot{
			State: "Idle",
		},
	}

	out := FormatCheckpointHistory(cp)

	require.Contains(t, out, "checkpoint: cp-empty")
	require.Contains(t, out, "history: <empty>")
	require.NotContains(t, out, "workspace_ref:")
}

func TestRollbackCheckpointExecutesBoundaryCompensation(t *testing.T) {
	t.Parallel()
	executor := &recordingCompensationExecutor{}
	cp := checkpointWithBoundaryCompensation(t)

	_, err := RollbackCheckpointWithOptions(RollbackCheckpointOptions{
		Checkpoint: cp, TargetIteration: 1, Compensation: executor,
	})

	require.NoError(t, err)
	require.Len(t, executor.mementos, 1)
	require.Equal(t, "rest_set", executor.mementos[0].CommandName)
}

func TestRollbackCheckpointReportsMissingBoundaryCompensationExecutor(t *testing.T) {
	t.Parallel()

	_, err := RollbackCheckpoint(checkpointWithBoundaryCompensation(t), 1)

	require.Error(t, err)
	require.Contains(t, err.Error(), "boundary compensation executor missing")
}

func saveCheckpointForOps(t *testing.T, store StateStore, cp CheckpointRecord) {
	t.Helper()
	data, err := json.Marshal(cp)
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), "checkpoint/"+cp.ID, data))
}

func checkpointWithBoundaryCompensation(t *testing.T) CheckpointRecord {
	t.Helper()
	memento, err := NewUndoMemento("rest_set", UndoMementoCompensatable, map[string]interface{}{
		"boundary_compensation": map[string]interface{}{
			"strategy": "restore", "rest_ref": "github", "resource": "issue",
			"operation": "set", "compensation": map[string]interface{}{"operation": "restore_issue"},
		},
	})
	require.NoError(t, err)
	noop := NoopUndoMemento("read")
	return CheckpointRecord{
		ID: "cp-rest", Iteration: 2,
		AgentState: AgentSnapshot{State: "AfterWrite", Iteration: 2},
		History: []HistoryDigest{
			{Iteration: 1, CommandName: "read", FromState: "Start", ToState: "Read", Signal: ToolDone, Undo: &noop},
			{Iteration: 2, CommandName: "rest_set", FromState: "Read", ToState: "AfterWrite", Signal: ToolDone, Undo: &memento},
		},
	}
}

type recordingCompensationExecutor struct {
	mementos []UndoMemento
}

func (r *recordingCompensationExecutor) Compensate(_ context.Context, memento UndoMemento) Result {
	r.mementos = append(r.mementos, memento)
	return Result{Signal: ToolDone, CommandName: memento.CommandName}
}
