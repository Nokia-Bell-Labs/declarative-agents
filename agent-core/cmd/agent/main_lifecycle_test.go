// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
)

type finalizedLoadCheckpoint struct{}

func (finalizedLoadCheckpoint) Save(core.Position, core.Execution) error { return nil }

func (finalizedLoadCheckpoint) Load() (core.Position, core.Execution, error) {
	position := core.Position{
		CurrentState: "Done",
		Snapshot: core.AgentSnapshot{
			State: "Done", Iteration: 4, TokensIn: 10, TokensOut: 5, TotalCost: 0.25,
		},
	}
	return position, nil, fmt.Errorf("%w: test run", core.ErrCheckpointFinalized)
}

func TestControlProfileExitReachesSucceededBeforeDeferredShutdown(t *testing.T) {
	t.Parallel()
	var cancelled bool
	shutdown := newDeferredShutdown(func() { cancelled = true })

	result := runExitMachine(t, exitMachineCase{
		machinePath: profilePathFromTest(t, "control/machine.yaml"),
		launch:      "launch_agent_control",
		await:       "await_agent_control",
		terminal:    "Succeeded",
		shutdown:    shutdown,
	})

	require.Equal(t, core.StatusSucceeded, result.Status)
	require.Equal(t, core.State("Succeeded"), result.FinalState)
	requireExitEvent(t, result)
	require.False(t, cancelled, "shutdown must wait until after Loop returns")
	shutdown.Apply()
	require.True(t, cancelled)
}

func TestApprovalLifecycleProfileSuspendsThroughCheckpointPort(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	profilePath := profilePathFromTest(t, "lifecycle/profile.yaml")

	clearAgentFlags()
	flagProfile = profilePath
	// No --dolt-dsn: persistence defaults to NoopCheckpoint, so the run still
	// suspends at the approval gate without a persistent backend. Round-trip
	// persistence via Dolt is covered by TestDoltCheckpointSuspendResumeRoundTrip.
	firstStderr, err := captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.NoError(t, err)
	require.Contains(t, firstStderr, "terminal state: suspended")
}

func TestResolveCheckpointDefaultsToNoop(t *testing.T) {
	t.Parallel()

	cp, err := resolveCheckpoint(runtimeConfig{}, core.MachineSpec{}, "run-test")

	require.NoError(t, err)
	require.IsType(t, core.NoopCheckpoint{}, cp.Checkpoint)
}

func TestResolveCheckpointWithDoltDSNOpensDoltBackend(t *testing.T) {
	t.Parallel()

	// A --dolt-dsn value routes to the Dolt adapter over the registered "dolt"
	// (MySQL-wire) driver; an unparseable DSN surfaces as a typed ErrDolt.
	_, err := resolveCheckpoint(runtimeConfig{DoltDSN: "not-a-valid-dsn"}, core.MachineSpec{}, "run-test")

	require.ErrorIs(t, err, core.ErrDolt)
}

func TestResumeWithoutPersistentBackendReportsNoCheckpoint(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	clearAgentFlags()
	flagProfile = profilePathFromTest(t, "lifecycle/profile.yaml")
	flagResumeCheckpoint = "missing"

	_, err := captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.ErrorIs(t, err, core.ErrNoCheckpoint)
}

func TestResumeRunReturnsFinalizedOutcomeWithoutDomainRestoreOrLoop(t *testing.T) {
	t.Parallel()
	params := core.LoopParams{
		Checkpoint: finalizedLoadCheckpoint{},
		IsTerminal: func(state core.State) bool { return state == "Done" },
		Hooks: core.LoopHooks{
			TerminalStatus: func(core.State) core.RunStatus { return core.StatusSucceeded },
		},
	}

	result, err := resumeRun(
		runtimeConfig{ResumeCheckpoint: "run-1"},
		resumeDeps{Params: params, Ctx: context.Background()},
	)

	require.NoError(t, err)
	require.Equal(t, core.StatusSucceeded, result.Status)
	require.Equal(t, core.State("Done"), result.FinalState)
	require.Equal(t, 4, result.Iterations)
	require.Equal(t, 10, result.TokensIn)
	require.Equal(t, 5, result.TokensOut)
	require.Equal(t, 0.25, result.TotalCost)
}
