// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// loadFailCheckpoint is a Checkpoint whose Load fails with a backend error other
// than ErrNoCheckpoint, standing in for a corrupt store or connection failure.
type loadFailCheckpoint struct{ err error }

func (c loadFailCheckpoint) Save(Position, Execution) error { return nil }
func (c loadFailCheckpoint) Load() (Position, Execution, error) {
	return Position{}, nil, c.err
}

// incompatibleCheckpoint holds a run persisted at a state the resume machine no
// longer defines, mirroring a machine changed since suspension.
func incompatibleCheckpoint() *InMemoryCheckpoint {
	cp := &InMemoryCheckpoint{}
	_ = cp.Save(
		Position{CurrentState: "RemovedState", LastSignal: AwaitApproval},
		Execution{{Iteration: 1, CommandName: "suspend", ToState: "RemovedState", Signal: AwaitApproval}},
	)
	return cp
}

// TestResumeErrorTaxonomy proves resume distinguishes the three failure classes
// required by srd025 R6.5 — missing, incompatible, and load failure — and admits
// a compatible checkpoint (GH-490).
func TestResumeErrorTaxonomy(t *testing.T) {
	t.Parallel()
	backendErr := fmt.Errorf("dolt adapter Load: connection refused")

	cases := []struct {
		name       string
		checkpoint Checkpoint
		wantErr    error // nil means success
	}{
		{name: "missing", checkpoint: &InMemoryCheckpoint{}, wantErr: ErrNoCheckpoint},
		{name: "incompatible", checkpoint: incompatibleCheckpoint(), wantErr: ErrCheckpointIncompatible},
		{name: "load failure", checkpoint: loadFailCheckpoint{err: backendErr}, wantErr: ErrCheckpointLoadFailed},
		{name: "compatible", checkpoint: suspendedCheckpoint(), wantErr: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := resumeLoopParams()
			params.Checkpoint = tc.checkpoint

			_, err := LoadResume(params)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.True(t, errors.Is(err, tc.wantErr), "err %v is not classified as %v", err, tc.wantErr)
			// Each class is distinct from the others.
			for _, other := range []error{ErrNoCheckpoint, ErrCheckpointIncompatible, ErrCheckpointLoadFailed} {
				if other == tc.wantErr {
					continue
				}
				require.False(t, errors.Is(err, other), "err %v must not also be classified as %v", err, other)
			}
		})
	}
}

// TestResumeCompatibilityUsesMachineSpecBeforeTableBuilt proves a checkpoint at a
// state the machine still declares resumes on the entrypoints that carry a
// MachineSpec but no pre-built transition table — the agent CLI --resume path,
// where initFromMachine builds the table only after LoadResume returns (GH-521).
func TestResumeCompatibilityUsesMachineSpecBeforeTableBuilt(t *testing.T) {
	t.Parallel()
	spec := MachineSpec{
		InitialState: "Idle",
		States:       StateSpecsFromNames("Idle", "AwaitingApproval", "Done"),
	}
	suspended := func(state State) *InMemoryCheckpoint {
		cp := &InMemoryCheckpoint{}
		_ = cp.Save(
			Position{CurrentState: state, LastSignal: AwaitApproval},
			Execution{{Iteration: 1, CommandName: "suspend", ToState: state, Signal: AwaitApproval}},
		)
		return cp
	}

	// A mid-run state the spec still declares resumes even though the table is empty.
	params := LoopParams{InitialState: "Idle", MachineSpec: &spec, Checkpoint: suspended("AwaitingApproval")}
	_, err := LoadResume(params)
	require.NoError(t, err)

	// A state the spec no longer declares stays classified as incompatible.
	params.Checkpoint = suspended("RemovedState")
	_, err = LoadResume(params)
	require.ErrorIs(t, err, ErrCheckpointIncompatible)
}

// TestResumeLoadFailurePreservesBackendDetail proves the load-failure error keeps
// the adapter's message for operator diagnosis (srd025 R6.5, srd035 R6.5).
func TestResumeLoadFailurePreservesBackendDetail(t *testing.T) {
	t.Parallel()
	params := resumeLoopParams()
	params.Checkpoint = loadFailCheckpoint{err: fmt.Errorf("dolt adapter Load: connection refused")}

	_, err := LoadResume(params)
	require.ErrorIs(t, err, ErrCheckpointLoadFailed)
	require.ErrorContains(t, err, "connection refused")
}
