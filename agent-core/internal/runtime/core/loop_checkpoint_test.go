// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"encoding/json"
	"errors"
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
	require.Equal(t, State("Working"), last.ToState, "the final command entry remains unchanged")
	require.Equal(t, State("Finished"), pos.CurrentState, "the actionless terminal transition is persisted")
	require.Equal(t, State("Finished"), pos.Snapshot.State)
	require.Equal(t, Signal("TaskCompleted"), pos.LastSignal)
}

// TestLoop_DoltFinalizesActionlessTerminalTransition proves the production Loop
// drives the real DoltCheckpoint adapter with the actual terminal Position. The
// finalization Save retains the two-method port and unchanged Execution, so it
// creates no synthetic command Entry or duplicate step write.
func TestLoop_DoltFinalizesActionlessTerminalTransition(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "loop-terminal", func(state State) bool {
		return state == "Finished"
	})
	params := simpleLoopParams(&loopRecorder{})
	params.AgentName = "loop-terminal"
	params.Checkpoint = cp

	rr, err := Loop(params, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, rr.Status)
	require.Equal(t, State("Finished"), rr.FinalState)
	require.Equal(t, 2, rr.Iterations)
	require.Equal(t, "Finished", db.store.machines["loop-terminal"].currentState)
	require.Len(t, db.store.steps, 2, "only dispatched commands become execution steps")
	require.Len(t, db.store.results, 2, "terminal finalization creates no fake forward-plane row")
	require.Equal(t, 2, countCalls(db.calls, "REPLACE INTO execution_steps"))
	require.Equal(t, 2, countCalls(db.calls, "REPLACE INTO tool_outputs"))
	require.Equal(t, 3, len(db.commits), "two command commits plus one terminal-position commit")
	require.Equal(t, "finalize terminal state Finished", db.commits[2].message)
	require.Equal(t, 1, countCalls(db.calls, "DOLT_MERGE"))
	require.Equal(t, 1, countCalls(db.calls, "DOLT_BRANCH('-d'"))
	require.False(t, db.branches["loop-terminal"])
}

func TestLoop_DoltPreservesSuspendedRunBranch(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "suspended-run", func(state State) bool {
		return state == "Failed"
	})
	params := suspendLoopParams(
		&loopRecorder{},
		&staticBuilder{cmd: &fakeCmd{name: "suspend", signal: AwaitApproval}},
	)
	params.AgentName = "suspended-run"
	params.Checkpoint = cp

	rr, err := Loop(params, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusSuspended, rr.Status)
	require.True(t, db.branches["suspended-run"])
	require.Zero(t, countCalls(db.calls, "DOLT_MERGE"))
	require.Zero(t, countCalls(db.calls, "DOLT_BRANCH('-d'"))
	pos, execution, err := cp.Load()
	require.NoError(t, err)
	require.Equal(t, State("AwaitingApproval"), pos.CurrentState)
	require.Len(t, execution, 1)
}

func TestLoop_TerminalFinalizationFailureIsNotReportedAsSuccess(t *testing.T) {
	t.Parallel()
	params := simpleLoopParams(&loopRecorder{})
	params.Checkpoint = &failingCheckpoint{err: errors.New("finalization unavailable")}

	rr, err := Loop(params, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusFailed, rr.Status)
	require.Equal(t, State("Finished"), rr.FinalState)
	require.ErrorContains(t, rr.LastError, "terminal checkpoint not persisted")
	require.ErrorContains(t, rr.LastError, "finalization unavailable")
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
