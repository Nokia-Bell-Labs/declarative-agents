// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package doltcheckpoint

import (
	"context"
	"database/sql"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
)

func resumeFromCheckpoint(params core.LoopParams, ctx context.Context) (core.RunResult, error) {
	state, err := core.LoadResume(params)
	if err != nil {
		return core.RunResult{}, err
	}
	if state.Finalized {
		return state.Params.InitialRun, nil
	}
	return core.Loop(state.Params, ctx)
}

func TestResumeFreshDoltAdapterFinalizesWithoutMachineStep(t *testing.T) {
	t.Parallel()
	terminal := func(s core.State) bool { return s == "Finished" }
	db := newFakeDB()
	saver := NewDoltCheckpoint(db, "run-resume", terminal)
	execution := sampleExecution()[:1]
	position := samplePosition()
	position.CurrentState = "AwaitingApproval"
	require.NoError(t, saver.Save(position, execution))

	position.CurrentState = "Finished"
	position.Snapshot.State = "Finished"
	position.Snapshot.Iteration = 7
	db.failOn = "DOLT_MERGE"
	require.ErrorIs(t, saver.Save(position, execution), ErrDolt)
	commits := len(db.commits)
	db.failOn = ""

	params := resumeLoopParams()
	params.Checkpoint = NewDoltCheckpoint(db, "run-resume", terminal)
	result, err := resumeFromCheckpoint(params, context.Background())

	require.NoError(t, err)
	require.Equal(t, core.StatusSucceeded, result.Status)
	require.Equal(t, core.State("Finished"), result.FinalState)
	require.Equal(t, 7, result.Iterations)
	require.Len(t, db.commits, commits, "resume performs lifecycle cleanup without a machine save")
	require.Equal(t, 2, countCalls(db.calls, "DOLT_MERGE"))
	require.Equal(t, 1, countCalls(db.calls, "DOLT_BRANCH('-d'"))
}

func TestResumeDoltCheckoutFailureIsLoadFailure(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	db.branches["permission-denied-run"] = true
	db.failOn = "DOLT_CHECKOUT"
	params := resumeLoopParams()
	params.Checkpoint = NewDoltCheckpoint(db, "permission-denied-run", nil)

	_, err := core.LoadResume(params)
	require.ErrorIs(t, err, core.ErrCheckpointLoadFailed)
	require.NotErrorIs(t, err, core.ErrNoCheckpoint)
	require.NotErrorIs(t, err, core.ErrCheckpointIncompatible)
	require.ErrorContains(t, err, `load: checkout branch "permission-denied-run"`)
	require.ErrorContains(t, err, sql.ErrConnDone.Error())
}
