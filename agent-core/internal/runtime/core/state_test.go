// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateMachineStepUnhandledPairFallsBackToFailed(t *testing.T) {
	t.Parallel()
	machine := NewStateMachine(
		TransitionTable{
			{State: "Idle", Signal: "Start"}:   {NextState: "Working"},
			{State: "Working", Signal: "Done"}: {NextState: "Succeeded"},
		},
		func(state State) bool { return state == "Succeeded" || state == "Failed" },
	)

	tests := []struct {
		name   string
		state  State
		signal Signal
	}{
		{name: "unknown state", state: "Missing", signal: "Start"},
		{name: "unknown signal", state: "Idle", signal: "Missing"},
		{name: "signal valid for another current state", state: "Idle", signal: "Done"},
		{name: "terminal current state", state: "Succeeded", signal: "Done"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			next, command, err := machine.Step(tt.state, tt.signal, Result{})

			require.Error(t, err)
			assert.Equal(t, State("Failed"), next)
			assert.Nil(t, command)
			assert.ErrorContains(t, err, "unhandled state-signal pair")
			assert.ErrorContains(t, err, "state="+string(tt.state))
			assert.ErrorContains(t, err, "signal="+string(tt.signal))
		})
	}
}
