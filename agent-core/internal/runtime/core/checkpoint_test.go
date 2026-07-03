// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type memoryStateStore struct {
	data map[string][]byte
}

func (m *memoryStateStore) Save(_ context.Context, key string, data []byte) error {
	if m.data == nil {
		m.data = make(map[string][]byte)
	}
	m.data[key] = append([]byte(nil), data...)
	return nil
}

func (m *memoryStateStore) Load(_ context.Context, key string) ([]byte, error) {
	return append([]byte(nil), m.data[key]...), nil
}

func (m *memoryStateStore) List(_ context.Context, prefix string) ([]string, error) {
	keys := make([]string, 0, len(m.data))
	for key := range m.data {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (m *memoryStateStore) Delete(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
}

type alwaysCheckpointPolicy struct{}

func (alwaysCheckpointPolicy) ShouldCheckpoint(CheckpointEvent) bool { return true }

var _ StateStore = (*memoryStateStore)(nil)
var _ CheckpointPolicy = alwaysCheckpointPolicy{}

func TestCheckpointContractsCompileAndRoundTrip(t *testing.T) {
	t.Parallel()

	cp := CheckpointRecord{
		ID:        "cp-1",
		Iteration: 2,
		Timestamp: time.Unix(100, 0).UTC(),
		AgentState: AgentSnapshot{
			State:     State("Working"),
			Signal:    Signal("ToolDone"),
			Iteration: 2,
			TokensIn:  10,
			TokensOut: 5,
			TotalCost: 0.25,
		},
		ConversationLog: json.RawMessage(`[{"role":"user","content":"hello"}]`),
		DomainState:     json.RawMessage(`{"conversation_len":3}`),
		WorkspaceRef:    "abc123",
		History: []HistoryDigest{{
			Iteration:    2,
			CommandName:  "write",
			FromState:    State("Composing"),
			ToState:      State("Parsing"),
			Signal:       Signal("ToolDone"),
			WorkspaceRef: "abc123",
		}},
	}

	data, err := json.Marshal(cp)
	require.NoError(t, err)

	var got CheckpointRecord
	require.NoError(t, json.Unmarshal(data, &got))
	require.Equal(t, cp.ID, got.ID)
	require.Equal(t, cp.AgentState.State, got.AgentState.State)
	require.Equal(t, cp.WorkspaceRef, got.WorkspaceRef)
	require.JSONEq(t, string(cp.ConversationLog), string(got.ConversationLog))
	require.JSONEq(t, string(cp.DomainState), string(got.DomainState))
	require.Len(t, got.History, 1)
	require.Equal(t, "write", got.History[0].CommandName)
}

func TestNoopUndoReturnsSuccessfulResult(t *testing.T) {
	t.Parallel()

	res := NoopUndo("read")

	require.Equal(t, ToolDone, res.Signal)
	require.Equal(t, "read", res.CommandName)
	require.Contains(t, res.Output, "no-op")
}

func TestStateStoreContractPersistsBytes(t *testing.T) {
	t.Parallel()

	store := &memoryStateStore{}
	ctx := context.Background()

	require.NoError(t, store.Save(ctx, "checkpoint/1", []byte(`{"ok":true}`)))
	keys, err := store.List(ctx, "checkpoint/")
	require.NoError(t, err)
	require.Equal(t, []string{"checkpoint/1"}, keys)

	data, err := store.Load(ctx, "checkpoint/1")
	require.NoError(t, err)
	require.JSONEq(t, `{"ok":true}`, string(data))

	require.NoError(t, store.Delete(ctx, "checkpoint/1"))
	keys, err = store.List(ctx, "checkpoint/")
	require.NoError(t, err)
	require.Empty(t, keys)
}
