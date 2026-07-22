// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestControlProfileExitReachesSucceededBeforeDeferredShutdown(t *testing.T) {
	t.Parallel()
	var cancelled bool
	shutdown := newDeferredShutdown(func() { cancelled = true })

	result := runExitMachine(t, exitMachineCase{
		machinePath: profilePathFromTest(t, "../testdata/conformance/control/machine.yaml"),
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

func TestDocumentationCuratorExitReachesDoneBeforeDeferredShutdown(t *testing.T) {
	t.Parallel()
	var cancelled bool
	shutdown := newDeferredShutdown(func() { cancelled = true })

	result := runExitMachine(t, exitMachineCase{
		machinePath:   profilePathFromTest(t, "knowledge-manager/documentation-curator/machine.yaml"),
		launch:        "launch_documentation",
		secondLaunch:  "launch_curator_control",
		monitorLaunch: "launch_monitor_rest",
		monitorStop:   "stop_monitor_rest",
		docsStop:      "stop_documentation",
		await:         "await_curator_control",
		terminal:      "Done",
		shutdown:      shutdown,
	})

	require.Equal(t, core.StatusSucceeded, result.Status)
	require.Equal(t, core.State("Done"), result.FinalState)
	requireExitEvent(t, result)
	require.False(t, cancelled, "shutdown must wait until after Loop returns")
	shutdown.Apply()
	require.True(t, cancelled)
}

func TestApprovalLifecycleProfileSuspendsThroughCheckpointPort(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	profilePath := profilePathFromTest(t, "../testdata/conformance/lifecycle/approval/profile.yaml")

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

	cp, err := resolveCheckpoint(runtimeConfig{}, core.MachineSpec{})

	require.NoError(t, err)
	require.IsType(t, core.NoopCheckpoint{}, cp)
}

func TestResolveCheckpointWithDoltDSNOpensDoltBackend(t *testing.T) {
	t.Parallel()

	// A --dolt-dsn value routes to the Dolt adapter over the registered "dolt"
	// (MySQL-wire) driver; an unparseable DSN surfaces as a typed ErrDolt.
	_, err := resolveCheckpoint(runtimeConfig{DoltDSN: "not-a-valid-dsn"}, core.MachineSpec{})

	require.ErrorIs(t, err, core.ErrDolt)
}

func TestResumeWithoutPersistentBackendReportsNoCheckpoint(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	clearAgentFlags()
	flagProfile = profilePathFromTest(t, "../testdata/conformance/lifecycle/approval/profile.yaml")
	flagResumeCheckpoint = "missing"

	_, err := captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.ErrorIs(t, err, core.ErrNoCheckpoint)
}
