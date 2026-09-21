// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	"github.com/stretchr/testify/require"
)

// TestServerStopIncompleteDrainSucceeds is the srd033 R6.5 contract. Shutdown
// closes the listeners before it waits for connections already in flight, and
// Stop deregisters the server either way, so a drain the budget cuts short
// still leaves a server that has stopped serving: the command succeeds and the
// outcome is reported rather than raised.
func TestServerStopIncompleteDrainSucceeds(t *testing.T) {
	t.Parallel()
	state, name := launchDrainServer(t, "slow_drain", context.DeadlineExceeded)

	result := stopCommand(state, name).Execute()

	require.Equal(t, core.Signal("ServerStopped"), result.Signal, result.Output)
	output := decodeStopOutput(t, result.Output)
	require.Equal(t, DrainBudgetExpired, output["connection_drain"])
	require.Equal(t, "stopped", output["status"])
	_, err := state.runtime(name)
	require.ErrorContains(t, err, "is not launched")
}

// TestServerStopReportsDrainOutcome is the srd033 R6.10 contract: the field is
// present in both outcomes, differing in its value and not in its shape, so a
// machine reading the stop output can always branch on it.
func TestServerStopReportsDrainOutcome(t *testing.T) {
	t.Parallel()
	completedState, completedName := launchDrainServer(t, "clean_drain", nil)
	expiredState, expiredName := launchDrainServer(t, "expired_drain", context.DeadlineExceeded)

	completed := stopRESTServer(t, completedState, completedName)
	expired := stopRESTServer(t, expiredState, expiredName)

	require.Equal(t, DrainCompleted, completed["connection_drain"])
	require.Equal(t, DrainBudgetExpired, expired["connection_drain"])
	require.Equal(t, outputKeys(completed), outputKeys(expired))
}

// TestServerStopNonDrainFaultsFail keeps the failure paths honest (srd033
// R6.5): a server that was never launched, and a shutdown failing for any
// reason other than the expired budget, stay command failures.
func TestServerStopNonDrainFaultsFail(t *testing.T) {
	t.Parallel()
	missing := stopCommand(NewServerState(), "never_launched").Execute()
	require.Equal(t, core.Signal("CommandError"), missing.Signal, missing.Output)
	require.Contains(t, missing.Output, "is not launched")

	state, name := launchDrainServer(t, "broken_stop", errors.New("listener close refused"))
	result := stopCommand(state, name).Execute()

	require.Equal(t, core.Signal("CommandError"), result.Signal, result.Output)
	require.Contains(t, result.Output, "listener close refused")
}

// launchDrainServer launches a real server and replaces its shutdown seam with
// the outcome under test. The seam is written and read from this goroutine
// alone, so a proof presents each outcome without racing a real drain.
func launchDrainServer(t *testing.T, name string, shutdownErr error) (*ServerState, string) {
	t.Helper()
	state := NewServerState()
	launchRESTServerWithState(t, state, namedControlServer(name), restdef.LimitProfile{})
	runtime, err := state.runtime(name)
	require.NoError(t, err)
	live := runtime.shutdown
	runtime.shutdown = func(ctx context.Context) error {
		_ = live(ctx)
		return shutdownErr
	}
	t.Cleanup(func() { _, _ = state.Stop(name) })
	return state, name
}

func decodeStopOutput(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var output map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(raw), &output))
	return output
}

func outputKeys(output map[string]interface{}) []string {
	keys := make([]string, 0, len(output))
	for key := range output {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
