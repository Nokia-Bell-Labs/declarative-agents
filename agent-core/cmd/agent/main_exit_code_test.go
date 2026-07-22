// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// TestExitCodeForStatus pins the process exit contract a caller reads a run's
// outcome from (srd018-cli-flag-contract R6). The mapping is taken from the
// terminal RunStatus rather than a terminal state name, so a machine may name
// its terminal states freely (R6.3).
func TestExitCodeForStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status core.RunStatus
		want   int
		why    string
	}{
		{
			name: "succeeded exits zero", status: core.StatusSucceeded, want: ExitSucceeded,
			why: "a success terminal is the caller's pass signal",
		},
		{
			name: "suspended exits zero", status: core.StatusSuspended, want: ExitSucceeded,
			why: "a suspension is a deliberate pause with a persisted checkpoint, not a failure (R6.4)",
		},
		{
			name: "failed exits machine-failed", status: core.StatusFailed, want: ExitMachineFailed,
			why: "a failure terminal must be visible in the exit status (R6.1)",
		},
		{
			name: "budget exceeded exits machine-failed", status: core.StatusBudgetExceeded, want: ExitMachineFailed,
			why: "an exhausted budget did not reach a success terminal",
		},
		{
			name: "cancelled exits machine-failed", status: core.StatusCancelled, want: ExitMachineFailed,
			why: "a cancelled run did not reach a success terminal",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, exitCodeForStatus(tc.status), tc.why)
		})
	}
}

// TestExitCodesAreDistinct pins R6.2: a caller must be able to tell a clean
// domain failure from a binary that could not complete a run at all.
func TestExitCodesAreDistinct(t *testing.T) {
	t.Parallel()

	require.NotEqual(t, ExitRunError, ExitMachineFailed,
		"a failed terminal and a failed invocation must be distinguishable")
	require.NotEqual(t, ExitSucceeded, ExitMachineFailed)
	require.NotEqual(t, ExitSucceeded, ExitRunError)
}
