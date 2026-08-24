// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package doltcheckpoint

import (
	"context"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
)

// TestLoop_DoltFinalizesActionlessTerminalTransition proves the production Loop
// drives the real DoltCheckpoint adapter with the actual terminal Position. The
// finalization Save retains the two-method port and unchanged Execution, so it
// creates no synthetic command Entry or duplicate step write.
func TestLoop_DoltFinalizesActionlessTerminalTransition(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "loop-terminal", func(state core.State) bool {
		return state == "Finished"
	})
	params := simpleLoopParams(tracing.NoopTracer{})
	params.AgentName = "loop-terminal"
	params.Checkpoint = cp

	rr, err := core.Loop(params, context.Background())

	require.NoError(t, err)
	require.Equal(t, core.StatusSucceeded, rr.Status)
	require.Equal(t, core.State("Finished"), rr.FinalState)
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
	cp := NewDoltCheckpoint(db, "suspended-run", func(state core.State) bool {
		return state == "Failed"
	})
	params := suspendLoopParams(
		tracing.NoopTracer{},
		&staticBuilder{cmd: &fakeCmd{name: "suspend", signal: core.AwaitApproval}},
	)
	params.AgentName = "suspended-run"
	params.Checkpoint = cp

	rr, err := core.Loop(params, context.Background())

	require.NoError(t, err)
	require.Equal(t, core.StatusSuspended, rr.Status)
	require.True(t, db.branches["suspended-run"])
	require.Zero(t, countCalls(db.calls, "DOLT_MERGE"))
	require.Zero(t, countCalls(db.calls, "DOLT_BRANCH('-d'"))
	pos, execution, err := NewDoltCheckpoint(db, "suspended-run", cp.terminal).Load()
	require.NoError(t, err)
	require.Equal(t, core.State("AwaitingApproval"), pos.CurrentState)
	require.Len(t, execution, 1)
}

func TestLoop_PeriodicSaveFailureStopsAtUnpersistedStep(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	db.failOn = "REPLACE INTO execution_steps"
	db.failOnCall = 2
	cp := NewDoltCheckpoint(db, "periodic-save-failure", func(state core.State) bool {
		return state == "Finished"
	})
	params := simpleLoopParams(tracing.NoopTracer{})
	params.AgentName = "periodic-save-failure"
	params.Checkpoint = cp

	rr, err := core.Loop(params, context.Background())

	require.ErrorIs(t, err, core.ErrCheckpointSaveFailed)
	require.Equal(t, core.StatusFailed, rr.Status)
	require.Equal(t, core.State("Working"), rr.FinalState)
	require.Equal(t, 2, rr.Iterations)
	require.ErrorIs(t, rr.LastError, core.ErrCheckpointSaveFailed)
	require.ErrorContains(t, rr.LastError, "adapter *doltcheckpoint.DoltCheckpoint Save at iteration 2")
	require.ErrorContains(t, rr.LastError, "dolt checkpoint")
	require.Len(t, db.commits, 1, "the failed step must not be committed as resumable")
}
