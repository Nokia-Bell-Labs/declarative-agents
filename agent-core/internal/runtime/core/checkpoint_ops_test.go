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
	saveCheckpointForOps(t, store, Checkpoint{ID: "older", Timestamp: time.Unix(100, 0).UTC()})
	saveCheckpointForOps(t, store, Checkpoint{ID: "newer", Timestamp: time.Unix(200, 0).UTC()})

	id, err := ResolveLatestCheckpointID(context.Background(), store, "latest")
	require.NoError(t, err)
	require.Equal(t, "newer", id)
}

func TestResolveLatestCheckpointIDEmptyStringMeansLatest(t *testing.T) {
	t.Parallel()
	store := &memoryStateStore{}
	saveCheckpointForOps(t, store, Checkpoint{ID: "only", Timestamp: time.Unix(100, 0).UTC()})

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
	cp := Checkpoint{
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
	cp := Checkpoint{
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

func saveCheckpointForOps(t *testing.T, store StateStore, cp Checkpoint) {
	t.Helper()
	data, err := json.Marshal(cp)
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), "checkpoint/"+cp.ID, data))
}
