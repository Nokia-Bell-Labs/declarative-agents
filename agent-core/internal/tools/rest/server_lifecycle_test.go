// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestRESTServer_LaunchRegistersRoutes(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, controlServer(), LimitProfile{})
	defer stopRESTServer(t, state, "control")

	result := getJSON(t, baseURL+"/health")
	require.Equal(t, "ok", result["status"])
	require.Equal(t, "control", getJSON(t, baseURL+"/metadata")["server"])
}

func TestRESTServer_DuplicateLaunchReleasesNewListener(t *testing.T) {
	t.Parallel()
	state := NewServerState()
	first := monitorServer("duplicate")
	_, err := state.Launch(ServerDefinition{Name: "duplicate", Server: first})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = state.Stop("duplicate") })

	for range 10 {
		reservation, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		address := reservation.Addr().String()
		require.NoError(t, reservation.Close())
		duplicate := monitorServer("duplicate")
		duplicate.Address = address

		_, err = state.Launch(ServerDefinition{Name: "duplicate", Server: duplicate})
		require.ErrorContains(t, err, `REST server "duplicate" is already launched`)

		rebound, err := net.Listen("tcp", address)
		require.NoError(t, err, "duplicate launch leaked listener at %s", address)
		require.NoError(t, rebound.Close())
	}
}

func TestRESTServer_ControlQueueAndReadPolicyConformance(t *testing.T) {
	t.Parallel()

	t.Run("lifecycle control enqueues signal", requireLifecycleControlEnqueuesSignal)
	t.Run("drop oldest keeps newest event", requireDropOldestQueuePolicy)
	t.Run("unsupported queue and drain policies fail validation", requireUnsupportedQueueAndDrainPoliciesRejected)
	t.Run("unsupported read policy rejected", requireUnsupportedReadPolicyRejected)
}

func TestRESTServer_StopDrainsAndUnblocks(t *testing.T) {
	t.Parallel()

	t.Run("drains queued events", func(t *testing.T) {
		state, baseURL := launchRESTServer(t, controlServer(), LimitProfile{})
		postStatus(t, baseURL+"/approve/1", `{}`, http.StatusAccepted)
		postStatus(t, baseURL+"/approve/2", `{}`, http.StatusAccepted)
		result := stopRESTServer(t, state, "control")
		require.Equal(t, float64(2), result["drained_events"])
		require.Equal(t, float64(0), result["dropped_events"])
		require.Equal(t, "drain_then_stop", result["drain_policy"])
		require.Equal(t, "drained", result["queue_outcome"])
	})

	t.Run("unblocks await", func(t *testing.T) {
		server := namedControlServer("blocking")
		server.Queue.Timeout = "1s"
		state, _ := launchRESTServer(t, server, LimitProfile{})
		results := startRESTAwait(t, func() core.Result {
			return awaitCommand(state, "blocking").Execute()
		})
		requireAwaitBlocked(t, results)
		require.Equal(t, "stopped", stopRESTServer(t, state, "blocking")["status"])
		require.Equal(t, core.Signal("ServerStopped"), requireRESTResult(t, results).Signal)
	})
}

func TestRESTAwaitCommandSupportsDispatchCancellation(t *testing.T) {
	t.Parallel()
	server := namedControlServer("context_await")
	server.Queue.Timeout = "30s"
	state, _ := launchRESTServer(t, server, LimitProfile{})
	defer stopRESTServer(t, state, "context_await")
	command := awaitCommand(state, "context_await")
	_, ok := command.(core.ContextCommand)
	require.True(t, ok)

	result := core.SafeExecute(command, time.Millisecond)

	require.Equal(t, core.CommandError, result.Signal)
	require.ErrorContains(t, result.Err, "timeout executing")
}

func TestRESTServer_QueueOverflowPolicies(t *testing.T) {
	t.Parallel()

	t.Run("drop oldest keeps newest event", requireDropOldestQueuePolicy)
	t.Run("unsupported queue and drain policies fail validation", requireUnsupportedQueueAndDrainPoliciesRejected)
}

func TestRESTServer_ShutdownConfigValidation(t *testing.T) {
	t.Parallel()

	for _, policy := range []string{"", "drain", "drain_then_stop"} {
		server := shutdownValidationServer("valid_shutdown")
		server.Shutdown.DrainPolicy = policy
		server.Shutdown.UnblockAwaitSignal = "ServerStopped"
		err := ValidateDefinition(Definition{Version: "v1", Servers: map[string]Server{"valid_shutdown": server}})
		require.NoError(t, err)
	}

	tests := []struct {
		name     string
		mutate   func(*ShutdownConfig)
		contains string
	}{
		{name: "drain timeout", mutate: func(cfg *ShutdownConfig) { cfg.DrainTimeout = "1s" }, contains: "drain_timeout"},
		{name: "stop listeners false", mutate: func(cfg *ShutdownConfig) { cfg.StopListeners = boolPointer(false) }, contains: "stop_listeners"},
		{name: "queue on shutdown", mutate: func(cfg *ShutdownConfig) { cfg.QueueOnShutdown = "drop" }, contains: "queue_on_shutdown"},
		{name: "unblock await signal", mutate: func(cfg *ShutdownConfig) { cfg.UnblockAwaitSignal = "StoppedCustom" }, contains: "unblock_await_signal"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := shutdownValidationServer("invalid_shutdown")
			tc.mutate(&server.Shutdown)
			err := ValidateDefinition(Definition{Version: "v1", Servers: map[string]Server{"invalid_shutdown": server}})
			require.ErrorContains(t, err, tc.contains)
		})
	}
}

func TestRESTServer_StreamEventsUnblocksOnStop(t *testing.T) {
	t.Parallel()

	server := streamServer()
	server.Queue.Timeout = "1s"
	state, baseURL := launchRESTServer(t, server, LimitProfile{})
	bodyC := make(chan string, 1)
	errC := make(chan error, 1)
	go streamResponse(baseURL+"/events", bodyC, errC)
	requireActiveStreams(t, state, "stream", 1)

	start := time.Now()
	result := stopRESTServer(t, state, "stream")
	require.Less(t, time.Since(start), 500*time.Millisecond)
	require.Equal(t, "stopped", result["status"])

	select {
	case err := <-errC:
		require.NoError(t, err)
		body := <-bodyC
		require.Contains(t, body, "event: server_stopped")
		require.Contains(t, body, `"signal":"ServerStopped"`)
	case <-time.After(500 * time.Millisecond):
		require.Fail(t, "stream did not unblock after server stop")
	}
}
