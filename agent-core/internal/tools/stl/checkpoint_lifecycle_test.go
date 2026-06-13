// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
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
	require.Equal(t, core.ToolDone, cmd.Undo().Signal)
}

func lifecycleCheckpoint(id string, ts time.Time) core.Checkpoint {
	return core.Checkpoint{
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

func saveLifecycleCheckpoint(t *testing.T, store core.StateStore, cp core.Checkpoint) {
	t.Helper()
	data, err := json.Marshal(cp)
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), "checkpoint/"+cp.ID, data))
}
