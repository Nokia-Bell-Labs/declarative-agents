// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/checkpoint"
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

func TestResumeWithoutPersistentBackendReportsNoCheckpoint(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	clearAgentFlags()
	flagProfile = profilePathFromTest(t, "lifecycle/profile.yaml")
	checkpointCfg.ResumeCheckpoint = "missing"

	_, err := captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.ErrorIs(t, err, core.ErrNoCheckpoint)
}

func TestResumeLoadOverridesRequestSeed(t *testing.T) {
	cp := &core.InMemoryCheckpoint{}
	require.NoError(t, cp.Save(core.Position{CurrentState: "Start"}, nil))
	params := terminalLoopParams()
	params.Checkpoint = cp
	params.InitialSignal = core.Seed
	params.InitialResult = core.Result{Signal: core.Seed, Output: "new request bytes"}
	params.Table = core.TransitionTable{
		{State: "Start", Signal: core.Seed}:     {NextState: "WrongSeedPath"},
		{State: "Start", Signal: core.Approved}: {NextState: "Resumed"},
	}
	params.IsTerminal = func(state core.State) bool {
		return state == "WrongSeedPath" || state == "Resumed"
	}
	params.Hooks.TerminalStatus = func(core.State) core.RunStatus { return core.StatusSucceeded }

	result, err := runOrResume(runtimeConfig{
		Checkpoint: checkpoint.Config{ResumeCheckpoint: "run-1", ResumeSignal: string(core.Approved)},
	}, resumeDeps{Params: params, State: &agentState{}, Ctx: context.Background()})

	require.NoError(t, err)
	require.Equal(t, core.State("Resumed"), result.FinalState)
	require.Equal(t, core.StatusSucceeded, result.Status)
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
		runtimeConfig{Checkpoint: checkpoint.Config{ResumeCheckpoint: "run-1"}},
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
