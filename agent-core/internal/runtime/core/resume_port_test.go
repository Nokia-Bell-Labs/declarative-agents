// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

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
			Result:      checkpointDigest(AwaitApproval, "", Cost{}),
		}},
	)
	return cp
}

func resumeFromCheckpoint(params LoopParams, ctx context.Context) (RunResult, error) {
	state, err := LoadResume(params)
	if err != nil {
		return RunResult{}, err
	}
	if state.Finalized {
		return state.Params.InitialRun, nil
	}
	return Loop(state.Params, ctx)
}

// TestResumeReentersLoopFromTypedPort covers rel02.0-uc001: a run suspended at an
// approval gate is resumed purely through the typed Checkpoint port and runs to
// completion, carrying the persisted counters forward (srd035 R6.2).
func TestResumeReentersLoopFromTypedPort(t *testing.T) {
	t.Parallel()
	params := resumeLoopParams()
	params.Checkpoint = suspendedCheckpoint()

	rr, err := resumeFromCheckpoint(params, context.Background())
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

func TestLoadResumeSeedsLastRedactedResult(t *testing.T) {
	t.Parallel()
	cp := &InMemoryCheckpoint{}
	digest := ResultDigest{
		Signal: ToolDone, Output: `{"public":"kept"}`,
		Cost:             Cost{TokensIn: 3},
		RedactionVersion: OutputRedactionVersion1,
		RedactedPaths:    []OutputRedactionPath{{"secret"}},
		RedactionStatus:  OutputRedactionApplied,
	}
	require.NoError(t, cp.Save(
		Position{CurrentState: "AwaitingApproval"},
		Execution{{
			CommandName: "fetch", ToState: "AwaitingApproval", Result: digest,
		}},
	))
	params := resumeLoopParams()
	params.Checkpoint = cp
	state, err := LoadResume(params)
	require.NoError(t, err)
	require.Equal(t, `{"public":"kept"}`, state.Params.InitialResult.Output)
	require.Equal(t, ToolDone, state.Params.InitialResult.Signal)
	require.Equal(t, "fetch", state.Params.InitialResult.CommandName)
	require.Equal(t, 3, state.Params.InitialResult.Cost.TokensIn)
	require.Equal(t, OutputRedactionVersion1, state.Params.InitialResult.Redaction.Version)
	require.Equal(t, []OutputRedactionPath{{"secret"}},
		state.Params.InitialResult.Redaction.Paths)
	require.NotContains(t, state.Params.InitialResult.Output, "secret")
}

func TestResumeNextBuilderReceivesPersistedOutput(t *testing.T) {
	t.Parallel()
	cp := &InMemoryCheckpoint{}
	require.NoError(t, cp.Save(
		Position{CurrentState: "AwaitingApproval"},
		Execution{{
			CommandName: "prepare",
			Result:      checkpointDigest(ToolDone, "distinctive persisted output", Cost{}),
		}},
	))
	var received string
	builder := previousOutputBuilder{received: &received}
	params := resumeLoopParams()
	params.Checkpoint = cp
	params.Table[TransitionInput{
		State: "AwaitingApproval", Signal: Approved,
	}] = TransitionValue{
		NextState: "Finishing", Action: builder.Build,
	}
	_, err := resumeFromCheckpoint(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, "distinctive persisted output", received)
}

type previousOutputBuilder struct{ received *string }

func (b previousOutputBuilder) Build(result Result) Command {
	*b.received = result.Output
	return &fakeCmd{name: "finish", signal: TaskCompleted}
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

func TestLoadResumeRejectsMissingResumeSignal(t *testing.T) {
	t.Parallel()
	params := resumeLoopParams()
	params.Checkpoint = suspendedCheckpoint()
	params.InitialSignal = ""

	_, err := LoadResume(params)
	require.ErrorContains(t, err, "machine resume_signal is required")
}

// TestResumeReportsMissingCheckpoint verifies a not-found snapshot surfaces
// ErrNoCheckpoint through the resume path.
func TestResumeReportsMissingCheckpoint(t *testing.T) {
	t.Parallel()
	params := resumeLoopParams()
	params.Checkpoint = &InMemoryCheckpoint{}

	_, err := resumeFromCheckpoint(params, context.Background())
	require.ErrorIs(t, err, ErrNoCheckpoint)
}

func TestResumeFreshDoltAdapterFinalizesWithoutMachineStep(t *testing.T) {
	t.Parallel()
	terminal := func(s State) bool { return s == "Finished" }
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
	require.Equal(t, StatusSucceeded, result.Status)
	require.Equal(t, State("Finished"), result.FinalState)
	require.Equal(t, 7, result.Iterations)
	require.Len(t, db.commits, commits, "resume performs lifecycle cleanup without a machine save")
	require.Equal(t, 2, countCalls(db.calls, "DOLT_MERGE"))
	require.Equal(t, 1, countCalls(db.calls, "DOLT_BRANCH('-d'"))
}
