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
	params.Program = ProgramRef{Profile: "/profiles/origin/profile.yaml", Digest: "sha256"}

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
	require.Equal(t, params.Program, pos.Snapshot.Program)
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
	require.Empty(t, db.store.transitions, "terminal history does not reach main")
	require.Empty(t, db.store.steps, "terminal execution does not reach main")
	require.Empty(t, db.store.results, "terminal forward-plane output does not reach main")
	require.Empty(t, db.store.receipts, "terminal reverse-plane receipts do not reach main")
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
	pos, execution, err := NewDoltCheckpoint(db, "suspended-run", cp.terminal).Load()
	require.NoError(t, err)
	require.Equal(t, State("AwaitingApproval"), pos.CurrentState)
	require.Len(t, execution, 1)
}

func TestLoop_TerminalFinalizationFailureIsNotReportedAsSuccess(t *testing.T) {
	t.Parallel()
	params := simpleLoopParams(&loopRecorder{})
	params.Checkpoint = &failOnSaveCheckpoint{
		failOn: 3,
		err:    errors.New("finalization unavailable"),
	}

	rr, err := Loop(params, context.Background())

	require.ErrorIs(t, err, ErrCheckpointSaveFailed)
	require.ErrorContains(t, err, "finalization unavailable")
	require.Equal(t, StatusFailed, rr.Status)
	require.Equal(t, State("Finished"), rr.FinalState)
	require.Equal(t, 2, rr.Iterations)
	require.ErrorContains(t, rr.LastError, "terminal checkpoint not persisted")
	require.ErrorContains(t, rr.LastError, "finalization unavailable")
}

func TestLoop_PeriodicSaveFailureStopsAtUnpersistedStep(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	db.failOn = "REPLACE INTO execution_steps"
	db.failOnCall = 2
	cp := NewDoltCheckpoint(db, "periodic-save-failure", func(state State) bool {
		return state == "Finished"
	})
	params := simpleLoopParams(&loopRecorder{})
	params.AgentName = "periodic-save-failure"
	params.Checkpoint = cp

	rr, err := Loop(params, context.Background())

	require.ErrorIs(t, err, ErrCheckpointSaveFailed)
	require.Equal(t, StatusFailed, rr.Status)
	require.Equal(t, State("Working"), rr.FinalState)
	require.Equal(t, 2, rr.Iterations)
	require.ErrorIs(t, rr.LastError, ErrCheckpointSaveFailed)
	require.ErrorContains(t, rr.LastError, "adapter *core.DoltCheckpoint Save at iteration 2")
	require.ErrorContains(t, rr.LastError, "dolt checkpoint")
	require.Len(t, db.commits, 1, "the failed step must not be committed as resumable")
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

func TestLoop_ConversationSnapshotFailureDoesNotReplaceConsistentCheckpoint(t *testing.T) {
	t.Parallel()
	cp := &InMemoryCheckpoint{}
	snapshotCalls := 0
	params := simpleLoopParams(&loopRecorder{})
	params.Checkpoint = cp
	params.Hooks.SnapshotConversation = func() (json.RawMessage, error) {
		snapshotCalls++
		if snapshotCalls == 2 {
			return nil, errors.New("conversation encoder unavailable")
		}
		return json.RawMessage(`[{"role":"user","content":"durable"}]`), nil
	}

	rr, err := Loop(params, context.Background())

	require.ErrorIs(t, err, ErrConversationSnapshotFailed)
	require.ErrorContains(t, err, "conversation encoder unavailable")
	require.Equal(t, StatusFailed, rr.Status)
	require.Equal(t, State("Working"), rr.FinalState)
	require.Equal(t, 2, rr.Iterations)
	require.ErrorIs(t, rr.LastError, ErrConversationSnapshotFailed)
	require.ErrorContains(t, rr.LastError, "conversation encoder unavailable")

	pos, execution, err := cp.Load()
	require.NoError(t, err)
	require.Equal(t, 1, pos.Snapshot.Iteration)
	require.Len(t, execution, 1)
	require.JSONEq(t, `[{"role":"user","content":"durable"}]`, string(pos.Snapshot.Conversation))
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

func TestLoop_NoopCheckpointIgnoresConversationSnapshotFailure(t *testing.T) {
	t.Parallel()
	snapshotCalls := 0
	params := simpleLoopParams(&loopRecorder{})
	params.Checkpoint = NoopCheckpoint{}
	params.Hooks.SnapshotConversation = func() (json.RawMessage, error) {
		snapshotCalls++
		return nil, errors.New("should not be called when persistence is disabled")
	}

	rr, err := Loop(params, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, rr.Status)
	require.NoError(t, rr.LastError)
	require.Zero(t, snapshotCalls)
}

type failOnSaveCheckpoint struct {
	InMemoryCheckpoint
	saves  int
	failOn int
	err    error
}

func (c *failOnSaveCheckpoint) Save(position Position, execution Execution) error {
	c.saves++
	if c.saves == c.failOn {
		return c.err
	}
	return c.InMemoryCheckpoint.Save(position, execution)
}
