// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// suspendedCheckpoint returns an InMemoryCheckpoint holding a run suspended at an
// approval gate, mirroring what the loop persists after a suspend dispatch.
func suspendedCheckpoint() *InMemoryCheckpoint {
	cp := &InMemoryCheckpoint{}
	_ = cp.Save(
		Position{
			CurrentState: "AwaitingApproval",
			LastSignal:   AwaitApproval,
			Snapshot: AgentSnapshot{
				State:        "AwaitingApproval",
				Signal:       AwaitApproval,
				Iteration:    1,
				TokensIn:     10,
				TokensOut:    5,
				TotalCost:    0.25,
				Conversation: json.RawMessage(`[{"role":"user","content":"before"}]`),
			},
		},
		Execution{{
			Iteration:   1,
			CommandName: "suspend",
			FromState:   "Start",
			ToState:     "AwaitingApproval",
			Signal:      AwaitApproval,
			Result:      ResultDigest{Signal: AwaitApproval},
		}},
	)
	return cp
}

// TestResumeReentersLoopFromTypedPort covers rel02.0-uc001: a run suspended at an
// approval gate is resumed purely through the typed Checkpoint port and runs to
// completion, carrying the persisted counters forward (srd035 R6.2).
func TestResumeReentersLoopFromTypedPort(t *testing.T) {
	t.Parallel()
	params := resumeLoopParams()
	params.Checkpoint = suspendedCheckpoint()

	rr, err := Resume(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, rr.Status)
	require.Equal(t, State("Finished"), rr.FinalState)
	require.Equal(t, 2, rr.Iterations)
	require.Equal(t, 10, rr.TokensIn)
	require.Equal(t, 5, rr.TokensOut)
	require.Equal(t, 0.25, rr.TotalCost)
}

// TestLoadResumeExposesTypedSnapshotForDomainRestore verifies that LoadResume
// seeds the loop at the restored position and returns the typed snapshot so the
// domain can restore conversation without a restore hook (srd035 R4, R6.2).
func TestLoadResumeExposesTypedSnapshotForDomainRestore(t *testing.T) {
	t.Parallel()
	params := resumeLoopParams()
	params.Checkpoint = suspendedCheckpoint()

	state, err := LoadResume(params)
	require.NoError(t, err)
	require.Equal(t, State("AwaitingApproval"), state.Params.InitialState)
	require.Equal(t, Approved, state.Params.InitialSignal)
	require.Equal(t, 1, state.Params.InitialRun.Iterations)
	require.Len(t, state.Params.InitialExecution, 1)
	require.JSONEq(t, `[{"role":"user","content":"before"}]`, string(state.Position.Snapshot.Conversation))
}

// TestResumeHonorsExplicitResumeSignal verifies the resume signal override
// (params.InitialSignal) is preserved instead of defaulting to Approved.
func TestResumeHonorsExplicitResumeSignal(t *testing.T) {
	t.Parallel()
	params := resumeLoopParams()
	params.Checkpoint = suspendedCheckpoint()
	params.InitialSignal = Rejected

	state, err := LoadResume(params)
	require.NoError(t, err)
	require.Equal(t, Rejected, state.Params.InitialSignal)
}

// TestResumeReportsMissingCheckpoint verifies a not-found snapshot surfaces
// ErrNoCheckpoint through the resume path.
func TestResumeReportsMissingCheckpoint(t *testing.T) {
	t.Parallel()
	params := resumeLoopParams()
	params.Checkpoint = &InMemoryCheckpoint{}

	_, err := Resume(params, context.Background())
	require.ErrorIs(t, err, ErrNoCheckpoint)
}
