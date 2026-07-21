// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// failingCheckpoint is a Checkpoint whose Save always fails, standing in for a
// persistence backend hiccup.
type failingCheckpoint struct{ err error }

func (f *failingCheckpoint) Save(Position, Execution) error     { return f.err }
func (f *failingCheckpoint) Load() (Position, Execution, error) { return Position{}, nil, nil }

// TestLoop_SuspendFailsWhenCheckpointSaveFails proves that when the dispatch that
// suspends the run cannot persist its checkpoint, the run reports StatusFailed
// rather than StatusSuspended: a suspended status must never be returned without
// a resumable checkpoint (srd025 R5.3, srd035 R6.5; GH-492).
func TestLoop_SuspendFailsWhenCheckpointSaveFails(t *testing.T) {
	t.Parallel()
	params := suspendLoopParams(&loopRecorder{}, &fakeBuilder{name: "suspend", signal: AwaitApproval})
	params.Checkpoint = &failingCheckpoint{err: fmt.Errorf("dolt adapter Save: connection refused")}

	rr, err := Loop(params, context.Background())

	require.NoError(t, err)
	require.NotEqual(t, StatusSuspended, rr.Status, "must not report a resumable suspend without a persisted checkpoint")
	require.Equal(t, StatusFailed, rr.Status)
	require.ErrorContains(t, rr.LastError, "suspend checkpoint not persisted")
	require.ErrorContains(t, rr.LastError, "connection refused")
}

// TestLoop_SuspendSucceedsWhenCheckpointSaveSucceeds guards the positive path so
// the terminal-on-failure change does not regress ordinary suspension.
func TestLoop_SuspendSucceedsWhenCheckpointSaveSucceeds(t *testing.T) {
	t.Parallel()
	params := suspendLoopParams(&loopRecorder{}, &fakeBuilder{name: "suspend", signal: AwaitApproval})
	params.Checkpoint = &failingCheckpoint{err: nil}

	rr, err := Loop(params, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusSuspended, rr.Status)
	require.NoError(t, rr.LastError)
}
