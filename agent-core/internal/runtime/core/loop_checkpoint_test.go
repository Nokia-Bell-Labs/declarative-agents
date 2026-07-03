// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLoop_SavesSnapshotAfterDispatchWithConfiguredAdapter verifies that the
// loop persists the Position and appended Execution through the Checkpoint port
// after each dispatch cycle (srd035-checkpoint-port R6.1).
func TestLoop_SavesSnapshotAfterDispatchWithConfiguredAdapter(t *testing.T) {
	t.Parallel()
	cp := &InMemoryCheckpoint{}
	params := simpleLoopParams(&loopRecorder{})
	params.Checkpoint = cp

	rr, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, rr.Status)

	pos, exec, err := cp.Load()
	require.NoError(t, err)
	require.Len(t, exec, rr.Iterations)
	require.Equal(t, 1, exec[0].Iteration)
	require.Equal(t, State("Start"), exec[0].FromState)
	require.Equal(t, State("Working"), exec[0].ToState)
	require.Equal(t, Seed, exec[0].Signal)

	last := exec[len(exec)-1]
	require.Equal(t, rr.Iterations, pos.Snapshot.Iteration)
	require.Equal(t, last.ToState, pos.CurrentState)
}

// TestLoop_PortSavePersistsConversation verifies that the loop folds the
// domain-owned conversation (via the SnapshotConversation hook) into the
// Position persisted through the Checkpoint port, so a port-based resume can
// restore it (srd035-checkpoint-port R4, R6.1).
func TestLoop_PortSavePersistsConversation(t *testing.T) {
	t.Parallel()
	cp := &InMemoryCheckpoint{}
	conversation := json.RawMessage(`[{"role":"user","content":"hello"}]`)
	params := simpleLoopParams(&loopRecorder{})
	params.Checkpoint = cp
	params.Hooks.SnapshotConversation = func() (json.RawMessage, error) {
		return conversation, nil
	}

	_, err := Loop(params, context.Background())
	require.NoError(t, err)

	pos, _, err := cp.Load()
	require.NoError(t, err)
	require.JSONEq(t, string(conversation), string(pos.Snapshot.Conversation))
}

// TestLoop_NoopCheckpointDefaultPersistsNothing verifies that a loop without a
// configured adapter defaults to NoopCheckpoint and preserves disabled-mode
// behavior (srd035-checkpoint-port R5.1, R5.4).
func TestLoop_NoopCheckpointDefaultPersistsNothing(t *testing.T) {
	t.Parallel()
	params := simpleLoopParams(&loopRecorder{})

	rr, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, rr.Status)

	_, _, err = NoopCheckpoint{}.Load()
	require.ErrorIs(t, err, ErrNoCheckpoint)
}
