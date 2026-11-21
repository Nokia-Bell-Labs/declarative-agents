// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestLoop_WallClockBudgetTerminatesAsCancelled pins the 1.0 specification's
// R-RUN statement on the wall-clock budget: an exceeded max_duration cancels
// the run's context and the run ends cancelled, without routing a further
// grammar transition, even when the machine declares a route for the
// interrupted dispatch's CommandError.
func TestLoop_WallClockBudgetTerminatesAsCancelled(t *testing.T) {
	t.Parallel()
	command := &dispatchContextBlockingCmd{started: make(chan struct{}), finished: make(chan struct{})}
	recovered := false
	params := LoopParams{
		InitialState: "Start",
		Registry:     NewRegistry(),
		Table: TransitionTable{
			{State: "Start", Signal: Seed}: {
				NextState: "Working",
				Action:    func(Result) Command { return command },
			},
			{State: "Working", Signal: CommandError}: {
				NextState: "Recovered",
				Action: func(Result) Command {
					recovered = true
					return &fakeCmd{name: "recover", signal: ToolDone}
				},
			},
		},
		IsTerminal: func(state State) bool { return state == "Recovered" },
		Trace:      &loopRecorder{},
		Budget:     Budget{MaxIterations: 10, MaxDuration: 20 * time.Millisecond},
	}

	result, err := Loop(params, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusCancelled, result.Status)
	require.Equal(t, State("Working"), result.FinalState)
	require.False(t, recovered, "a cancelled run must not route another grammar transition")
}
