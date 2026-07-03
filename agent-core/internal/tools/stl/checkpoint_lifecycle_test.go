// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type lifecycleMemoryStore struct {
	data map[string][]byte
}

func (s *lifecycleMemoryStore) Save(_ context.Context, key string, data []byte) error {
	if s.data == nil {
		s.data = make(map[string][]byte)
	}
	s.data[key] = append([]byte(nil), data...)
	return nil
}

func (s *lifecycleMemoryStore) Load(_ context.Context, key string) ([]byte, error) {
	return append([]byte(nil), s.data[key]...), nil
}

func (s *lifecycleMemoryStore) List(_ context.Context, prefix string) ([]string, error) {
	keys := make([]string, 0, len(s.data))
	for key := range s.data {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (s *lifecycleMemoryStore) Delete(_ context.Context, key string) error {
	delete(s.data, key)
	return nil
}

type lifecycleWorkspace struct {
	restored string
}

func (w *lifecycleWorkspace) Checkpoint(context.Context, string) (string, error) { return "head", nil }
func (w *lifecycleWorkspace) CurrentRef(context.Context) (string, error)         { return "head", nil }
func (w *lifecycleWorkspace) Restore(_ context.Context, ref string) error {
	w.restored = ref
	return nil
}

func TestCheckpointHistoryExecuteFormatsLatestCheckpoint(t *testing.T) {
	t.Parallel()
	store := &lifecycleMemoryStore{}
	saveLifecycleCheckpoint(t, store, lifecycleCheckpoint("older", time.Unix(100, 0).UTC()))
	saveLifecycleCheckpoint(t, store, lifecycleCheckpoint("newer", time.Unix(200, 0).UTC()))

	cmd := (&CheckpointHistoryBuilder{StateStore: store}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal)
	require.Equal(t, "checkpoint_history", res.CommandName)
	require.Contains(t, res.Output, "checkpoint: newer")
	require.Contains(t, res.Output, "history:")
}

func TestCheckpointHistoryExecuteFormatsExplicitCheckpoint(t *testing.T) {
	t.Parallel()
	store := &lifecycleMemoryStore{}
	saveLifecycleCheckpoint(t, store, lifecycleCheckpoint("cp-1", time.Unix(100, 0).UTC()))

	builder := &CheckpointHistoryBuilder{
		Config:     CheckpointHistoryConfig{Checkpoint: "cp-1"},
		StateStore: store,
	}
	res := builder.Build(core.Result{}).Execute()

	require.Equal(t, core.ToolDone, res.Signal)
	require.Contains(t, res.Output, "checkpoint: cp-1")
	require.Contains(t, res.Output, "1  read  Idle -> Reading  signal=ToolDone")
}

func TestCheckpointHistoryExecuteRequiresStateStore(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointHistoryBuilder{}).Build(core.Result{})

	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "requires StateStore")
}

func TestCheckpointHistoryExecuteReportsUnreadableCheckpoint(t *testing.T) {
	t.Parallel()
	store := &lifecycleMemoryStore{}

	cmd := (&CheckpointHistoryBuilder{StateStore: store}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "no checkpoints found")
}

func TestCheckpointHistoryUndoMementoIsNoop(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointHistoryBuilder{}).Build(core.Result{})
	provider, ok := cmd.(core.UndoMementoProvider)
	require.True(t, ok)

	memento, err := provider.UndoMemento()

	require.NoError(t, err)
	require.Equal(t, core.UndoMementoNoop, memento.Kind)
	require.Equal(t, "checkpoint_history", memento.CommandName)
	require.Equal(t, core.ToolDone, cmd.Undo(core.Result{}).Signal)
}

func TestCheckpointRollbackExecutePersistsNewCheckpoint(t *testing.T) {
	t.Parallel()
	target := 1
	store := &lifecycleMemoryStore{}
	saveLifecycleCheckpoint(t, store, lifecycleRollbackCheckpoint("cp-1", ""))

	cmd := (&CheckpointRollbackBuilder{
		Config:     CheckpointRollbackConfig{Checkpoint: "cp-1", ToIteration: &target},
		StateStore: store,
	}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal)
	require.Contains(t, res.Output, "rolled back checkpoint cp-1 to iteration 1")
	require.Contains(t, res.Output, "new checkpoint: rollback-cp-1-to-1-")
	keys, err := store.List(context.Background(), "checkpoint/")
	require.NoError(t, err)
	require.Len(t, keys, 2)
}

func TestCheckpointRollbackExecuteRestoresWorkspace(t *testing.T) {
	t.Parallel()
	target := 1
	store := &lifecycleMemoryStore{}
	saveLifecycleCheckpoint(t, store, lifecycleRollbackCheckpoint("cp-1", "ref-1"))
	workspace := &lifecycleWorkspace{}

	cmd := (&CheckpointRollbackBuilder{
		Config:     CheckpointRollbackConfig{Checkpoint: "cp-1", ToIteration: &target},
		StateStore: store,
		Workspace:  workspace,
	}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal)
	require.Equal(t, "ref-1", workspace.restored)
	require.Contains(t, res.Output, "workspace ref: ref-1")
}

func TestCheckpointRollbackExecuteRequiresTargetIteration(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointRollbackBuilder{StateStore: &lifecycleMemoryStore{}}).Build(core.Result{})

	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "requires to_iteration")
}

func TestCheckpointRollbackExecuteRequiresStateStore(t *testing.T) {
	t.Parallel()
	target := 1
	cmd := (&CheckpointRollbackBuilder{
		Config: CheckpointRollbackConfig{ToIteration: &target},
	}).Build(core.Result{})

	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "requires StateStore")
}

func TestCheckpointRollbackExecuteReportsRollbackFailure(t *testing.T) {
	t.Parallel()
	target := 99
	store := &lifecycleMemoryStore{}
	saveLifecycleCheckpoint(t, store, lifecycleRollbackCheckpoint("cp-1", ""))

	cmd := (&CheckpointRollbackBuilder{
		Config:     CheckpointRollbackConfig{Checkpoint: "cp-1", ToIteration: &target},
		StateStore: store,
	}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "target iteration 99")
}

func TestCheckpointRollbackExecuteRequiresManagedWorkspaceForRef(t *testing.T) {
	t.Parallel()
	target := 1
	store := &lifecycleMemoryStore{}
	saveLifecycleCheckpoint(t, store, lifecycleRollbackCheckpoint("cp-1", "ref-1"))

	cmd := (&CheckpointRollbackBuilder{
		Config:     CheckpointRollbackConfig{Checkpoint: "cp-1", ToIteration: &target},
		StateStore: store,
	}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "directory is required")
	keys, err := store.List(context.Background(), "checkpoint/")
	require.NoError(t, err)
	require.Len(t, keys, 1)
}

func TestCheckpointRollbackUndoMementoIsCompensatable(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointRollbackBuilder{}).Build(core.Result{})
	provider, ok := cmd.(core.UndoMementoProvider)
	require.True(t, ok)

	memento, err := provider.UndoMemento()

	require.NoError(t, err)
	require.Equal(t, core.UndoMementoCompensatable, memento.Kind)
	require.Equal(t, "checkpoint_rollback", memento.CommandName)
	require.Equal(t, core.CommandError, cmd.Undo(core.Result{}).Signal)
	var payload BoundaryCompensationPayload
	require.NoError(t, json.Unmarshal(memento.Payload, &payload))
	require.Equal(t, "operator_checkpoint_selection", payload.BoundaryCompensation.Strategy)
	require.Contains(t, payload.BoundaryCompensation.Requires, "checkpoint_id")
}

func lifecycleCheckpoint(id string, ts time.Time) core.CheckpointRecord {
	return core.CheckpointRecord{
		ID:        id,
		Iteration: 1,
		Timestamp: ts,
		AgentState: core.AgentSnapshot{
			State:     "Reading",
			Signal:    core.ToolDone,
			Iteration: 1,
		},
		History: []core.HistoryDigest{{
			Iteration:   1,
			CommandName: "read",
			FromState:   "Idle",
			ToState:     "Reading",
			Signal:      core.ToolDone,
		}},
	}
}

func lifecycleRollbackCheckpoint(id, targetRef string) core.CheckpointRecord {
	noop := core.NoopUndoMemento("write")
	return core.CheckpointRecord{
		ID:        id,
		Iteration: 2,
		Timestamp: time.Unix(100, 0).UTC(),
		AgentState: core.AgentSnapshot{
			State:     "Working",
			Signal:    core.ToolDone,
			Iteration: 2,
		},
		History: []core.HistoryDigest{
			{Iteration: 1, CommandName: "read", FromState: "Idle", ToState: "Reading", Signal: core.ToolDone, WorkspaceRef: targetRef},
			{Iteration: 2, CommandName: "write", FromState: "Reading", ToState: "Working", Signal: core.ToolDone, Undo: &noop},
		},
	}
}

func saveLifecycleCheckpoint(t *testing.T, store core.StateStore, cp core.CheckpointRecord) {
	t.Helper()
	data, err := json.Marshal(cp)
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), "checkpoint/"+cp.ID, data))
}
