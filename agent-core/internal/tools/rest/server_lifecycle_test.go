// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
	"github.com/stretchr/testify/require"
)

func TestRESTServer_LaunchRegistersRoutes(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, controlServer(), restdef.LimitProfile{})
	defer stopRESTServer(t, state, "control")

	result := getJSON(t, baseURL+"/health")
	require.Equal(t, "ok", result["status"])
	require.Equal(t, "control", getJSON(t, baseURL+"/metadata")["server"])
}

func TestRESTServer_DuplicateLaunchReleasesNewListener(t *testing.T) {
	if testing.Short() {
		t.Skip("integration-grade: binds real loopback listeners")
	}
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

func TestRESTServer_LaunchReceiptStopsOnlyOwnedListener(t *testing.T) {
	if testing.Short() {
		t.Skip("integration-grade: production REST server launch binds a real loopback listener")
	}
	t.Parallel()
	server := namedControlServer("receipt_control")
	state := NewServerState()
	builder := ServerBuilder{
		ToolName: "launch_control_alias", Init: InitServerLaunch,
		Server: ServerDefinition{Name: "receipt_control", Server: server}, State: state,
	}
	result := builder.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("ServerLaunched"), result.Signal, result.Output)
	require.Equal(t, "launch_control_alias", result.CommandName)
	receipt, err := decodeServerLaunchReceipt(result.Receipt)
	require.NoError(t, err)
	require.Equal(t, serverLaunchReceiptStrategy, receipt.Strategy)
	require.Equal(t, "launch_control_alias", receipt.Declaration)
	require.Equal(t, "receipt_control", receipt.Server)
	require.NotEmpty(t, receipt.Address)
	require.NotEmpty(t, receipt.Ownership)
	require.Equal(t, "ok", getJSON(t, "http://"+receipt.Address+"/health")["status"])

	undoResult := builder.BuildReverser().Undo(result)
	require.Equal(t, core.Signal("ServerStopped"), undoResult.Signal, undoResult.Output)
	require.Equal(t, "launch_control_alias", undoResult.CommandName)
	_, err = state.runtime("receipt_control")
	require.ErrorContains(t, err, "is not launched")
}

func TestRESTServer_LaunchUndoInFreshProcessIsAlreadyCompensated(t *testing.T) {
	if testing.Short() {
		t.Skip("integration-grade: production REST server launch binds a real loopback listener")
	}
	t.Parallel()
	server := namedControlServer("fresh_receipt")
	liveState := NewServerState()
	liveBuilder := ServerBuilder{
		ToolName: "launch_fresh_alias", Init: InitServerLaunch,
		Server: ServerDefinition{Name: "fresh_receipt", Server: server}, State: liveState,
	}
	result := liveBuilder.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("ServerLaunched"), result.Signal, result.Output)
	t.Cleanup(func() { _, _ = liveState.Stop("fresh_receipt") })
	receipt, err := decodeServerLaunchReceipt(result.Receipt)
	require.NoError(t, err)

	freshBuilder := liveBuilder
	freshBuilder.State = NewServerState()
	undoResult := freshBuilder.BuildReverser().Undo(result)
	require.Equal(t, core.Signal("ServerStopped"), undoResult.Signal, undoResult.Output)
	var output map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(undoResult.Output), &output))
	require.Equal(t, "already_compensated", output["status"])
	require.Equal(t, "launch_fresh_alias", undoResult.CommandName)

	// The fresh process-local state cannot claim or stop the listener that
	// remains owned by the originating process.
	require.Equal(t, "ok", getJSON(t, "http://"+receipt.Address+"/health")["status"])
}

func TestRESTServer_LaunchUndoRejectsInvalidOrUnownedReceipts(t *testing.T) {
	if testing.Short() {
		t.Skip("integration-grade: production REST server launch binds a real loopback listener")
	}
	t.Parallel()
	server := namedControlServer("strict_receipt")
	state := NewServerState()
	builder := ServerBuilder{
		ToolName: "launch_strict_alias", Init: InitServerLaunch,
		Server: ServerDefinition{Name: "strict_receipt", Server: server}, State: state,
	}
	result := builder.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("ServerLaunched"), result.Signal, result.Output)
	t.Cleanup(func() { _, _ = state.Stop("strict_receipt") })
	receipt, err := decodeServerLaunchReceipt(result.Receipt)
	require.NoError(t, err)
	baseURL := "http://" + receipt.Address

	tests := []struct {
		name        string
		commandName string
		receipt     string
		want        string
	}{
		{name: "malformed", receipt: "{", want: "decode REST server launch receipt"},
		{
			name: "wrong strategy",
			receipt: launchReceiptWith(t, receipt, func(value *serverLaunchReceipt) {
				value.Strategy = "server_shutdown"
			}),
			want: "strategy",
		},
		{
			name: "wrong declaration",
			receipt: launchReceiptWith(t, receipt, func(value *serverLaunchReceipt) {
				value.Declaration = "other_launch_alias"
			}),
			want: "declaration",
		},
		{
			name: "wrong server",
			receipt: launchReceiptWith(t, receipt, func(value *serverLaunchReceipt) {
				value.Server = "other_server"
			}),
			want: "configured server",
		},
		{
			name: "wrong address",
			receipt: launchReceiptWith(t, receipt, func(value *serverLaunchReceipt) {
				value.Address = "127.0.0.1:1"
			}),
			want: "listener address",
		},
		{
			name: "wrong ownership",
			receipt: launchReceiptWith(t, receipt, func(value *serverLaunchReceipt) {
				value.Ownership = strings.Repeat("0", serverOwnershipBytes*2)
			}),
			want: "not owned by the launch receipt",
		},
		{
			name: "wrong persisted command alias", commandName: "other_launch_alias",
			receipt: result.Receipt, want: "does not match declaration",
		},
		{
			name:    "unknown field",
			receipt: strings.TrimSuffix(result.Receipt, "}") + `,"unexpected":true}`,
			want:    "unknown field",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prior := core.Result{CommandName: test.commandName, Receipt: test.receipt}
			undoResult := builder.BuildReverser().Undo(prior)
			require.Equal(t, core.CommandError, undoResult.Signal, undoResult.Output)
			require.ErrorContains(t, undoResult.Err, test.want)
			require.Equal(t, "ok", getJSON(t, baseURL+"/health")["status"])
		})
	}

	runtime, err := state.runtime("strict_receipt")
	require.NoError(t, err)
	runtime.owned = false
	undoResult := builder.BuildReverser().Undo(result)
	require.Equal(t, core.CommandError, undoResult.Signal, undoResult.Output)
	require.ErrorContains(t, undoResult.Err, "not process-owned")
	require.Equal(t, "ok", getJSON(t, baseURL+"/health")["status"])
	runtime.owned = true
}

func launchReceiptWith(
	t *testing.T,
	receipt serverLaunchReceipt,
	mutate func(*serverLaunchReceipt),
) string {
	t.Helper()
	mutate(&receipt)
	data, err := json.Marshal(receipt)
	require.NoError(t, err)
	return string(data)
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
		state, baseURL := launchRESTServer(t, controlServer(), restdef.LimitProfile{})
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
		server.Shutdown.UnblockAwaitSignal = "StoppedCustom"
		state, _ := launchRESTServer(t, server, restdef.LimitProfile{})
		runtime, err := state.runtime("blocking")
		require.NoError(t, err)
		results := startRESTAwait(t, func() core.Result {
			result := runtime.awaitMatching(
				context.Background(),
				awaitFilter{server: "blocking"},
				StoppedSourceEmitServerStopped,
			)
			return core.Result{Signal: core.Signal(result.signal), Err: result.err}
		})
		requireAwaitBlocked(t, results)
		require.Equal(t, "stopped", stopRESTServer(t, state, "blocking")["status"])
		require.Equal(t, core.Signal("StoppedCustom"), requireRESTResult(t, results).Signal)
	})
}

func TestRESTServer_StopPersistsRelaunchCompensation(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, controlServer(), restdef.LimitProfile{})
	postStatus(t, baseURL+"/approve/1", `{}`, http.StatusAccepted)
	postStatus(t, baseURL+"/approve/2", `{}`, http.StatusAccepted)
	builder := ServerBuilder{
		ToolName: "stop_control", Init: InitServerStop,
		Server: ServerDefinition{Name: "control", Server: controlServer()}, State: state,
	}

	result := builder.Build(core.Result{}).Execute()

	require.Equal(t, core.Signal("ServerStopped"), result.Signal, result.Output)
	require.NotEmpty(t, result.Receipt)
	compensation, ok, err := undo.DecodeBoundaryReceipt(result.Receipt)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "server_shutdown_or_user_action_compensation", compensation.Strategy)
	require.Equal(t, "control", compensation.Data["rest_ref"])
	details := compensation.Data["compensation"].(map[string]interface{})
	require.Equal(t, float64(2), details["drained_events"])
	require.Equal(t, []string{"machine_owned_server_relaunch"}, compensation.Requires)

	undoResult := builder.BuildReverser().Undo(core.Result{Receipt: result.Receipt})
	require.Equal(t, core.CompensationRequired, undoResult.Signal)
	require.NoError(t, undoResult.Err)
	require.Contains(t, undoResult.Output, `MachineSpec must relaunch server "control"`)
	require.Contains(t, undoResult.Output, "drained 2 queued events")
}

func TestRESTAwaitCommandSupportsDispatchCancellation(t *testing.T) {
	t.Parallel()
	server := namedControlServer("context_await")
	server.Queue.Timeout = "30s"
	state, _ := launchRESTServer(t, server, restdef.LimitProfile{})
	defer stopRESTServer(t, state, "context_await")
	command := awaitCommand(state, "context_await")
	_, ok := command.(core.ContextCommand)
	require.True(t, ok)

	result := core.SafeExecuteContext(context.Background(), command, time.Millisecond)

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

	for _, policy := range []string{"", "drain_then_stop"} {
		server := shutdownValidationServer("valid_shutdown")
		server.Shutdown.DrainPolicy = policy
		server.Shutdown.UnblockAwaitSignal = "StoppedCustom"
		err := ValidateDefinition(restdef.Definition{Version: "v1", Servers: map[string]restdef.Server{"valid_shutdown": server}})
		require.NoError(t, err)
	}

	tests := []struct {
		name     string
		mutate   func(*restdef.ShutdownConfig)
		contains string
	}{
		{name: "inert drain policy", mutate: func(cfg *restdef.ShutdownConfig) { cfg.DrainPolicy = "drain" }, contains: "not implemented"},
		{name: "drain timeout", mutate: func(cfg *restdef.ShutdownConfig) { cfg.DrainTimeout = "1s" }, contains: "drain_timeout"},
		{name: "stop listeners false", mutate: func(cfg *restdef.ShutdownConfig) { cfg.StopListeners = boolPointer(false) }, contains: "stop_listeners"},
		{name: "queue on shutdown", mutate: func(cfg *restdef.ShutdownConfig) { cfg.QueueOnShutdown = "drop" }, contains: "queue_on_shutdown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := shutdownValidationServer("invalid_shutdown")
			tc.mutate(&server.Shutdown)
			err := ValidateDefinition(restdef.Definition{Version: "v1", Servers: map[string]restdef.Server{"invalid_shutdown": server}})
			require.ErrorContains(t, err, tc.contains)
		})
	}
}

func TestRESTServer_InjectsCanonicalLifecycleExit(t *testing.T) {
	t.Parallel()

	// A server that declares no exit route still answers the canonical path,
	// and the enqueued event matches a control await filtering by the reserved
	// route name, so injection alone drives the agent to exit (GH-1264).
	state, baseURL := launchRESTServer(t, bareLifecycleServer("inject_default"), restdef.LimitProfile{})
	defer stopRESTServer(t, state, "inject_default")

	result := postJSON(t, baseURL+"/api/lifecycle/exit", `{"reason":"operator"}`, http.StatusAccepted)
	require.Equal(t, true, result["accepted"])
	require.Equal(t, "ExitRequested", result["signal"])

	event, signal, err := state.AwaitAny(AwaitAnyOptions{
		Sources: []AwaitSource{{
			Server: "inject_default", Routes: []string{"exit"}, Signals: []string{"ExitRequested"},
		}},
		Timeout: time.Second,
	})
	require.NoError(t, err)
	require.Equal(t, "ExitRequested", signal)
	require.Equal(t, "exit", event.Route)
	require.Equal(t, "operator", event.Payload["reason"])
}

func TestRESTServer_LifecycleExitInjectionIdempotent(t *testing.T) {
	t.Parallel()

	// A profile that already declares the canonical path launches without a
	// route conflict and adds no duplicate: injection fills the gap, it does not
	// override the profile's own exit declaration.
	server := bareLifecycleServer("inject_declared")
	server.Endpoints["exit"] = restdef.Endpoint{
		Method: "POST", Path: "/api/lifecycle/exit", Binding: bindingEmitSignal,
		Signal:   "ExitRequested",
		Response: restdef.ResponseMapping{Output: map[string]string{"accepted": "true"}},
	}
	state := NewServerState()
	output, baseURL := launchRESTServerWithState(t, state, server, restdef.LimitProfile{})
	defer stopRESTServer(t, state, "inject_declared")

	require.Equal(t, float64(2), output["route_count"])
	postStatus(t, baseURL+"/api/lifecycle/exit", `{"reason":"operator"}`, http.StatusAccepted)
}

func TestRESTServer_LifecycleExitOptOut(t *testing.T) {
	t.Parallel()

	// Opt-out suppresses injection, so the canonical path is not served while
	// the server's own routes still answer.
	server := bareLifecycleServer("inject_optout")
	server.LifecycleExit.Disabled = true
	state, baseURL := launchRESTServer(t, server, restdef.LimitProfile{})
	defer stopRESTServer(t, state, "inject_optout")

	postStatus(t, baseURL+"/api/lifecycle/exit", `{"reason":"operator"}`, http.StatusNotFound)
	require.Equal(t, "ok", getJSON(t, baseURL+"/health")["status"])
}

func TestRESTServerQueueNameAndPayloadShapeAreEnforced(t *testing.T) {
	t.Parallel()
	t.Run("payload shape rejects event before enqueue", func(t *testing.T) {
		server := namedControlServer("shaped")
		server.LifecycleExit.Disabled = true
		endpoint := server.Endpoints["approve"]
		endpoint.Queue.PayloadShape = map[string]interface{}{
			"type": "object", "required": []interface{}{"operator"},
		}
		server.Endpoints["approve"] = endpoint
		state, baseURL := launchRESTServer(t, server, restdef.LimitProfile{})
		defer stopRESTServer(t, state, "shaped")
		postStatus(t, baseURL+"/approve/1", `{}`, http.StatusBadRequest)
	})

	t.Run("queue name reaches event output", func(t *testing.T) {
		server := namedControlServer("named_queue")
		server.LifecycleExit.Disabled = true
		endpoint := server.Endpoints["approve"]
		endpoint.Queue.Name = "approvals"
		server.Endpoints["approve"] = endpoint
		state, baseURL := launchRESTServer(t, server, restdef.LimitProfile{})
		defer stopRESTServer(t, state, "named_queue")
		postStatus(t, baseURL+"/approve/1", `{}`, http.StatusAccepted)
		event, signal, err := state.Await("named_queue")
		require.NoError(t, err)
		require.Equal(t, "Approved", signal)
		require.Equal(t, "approvals", event.Queue)
	})
}

func TestLifecycleControlActionNamesSignalEnqueueModes(t *testing.T) {
	t.Parallel()
	endpoint := restdef.Endpoint{LifecycleControl: restdef.LifecycleControl{
		Action: "enqueue_signal", Signal: "ExitRequested",
	}}
	require.NoError(t, validateLifecycleControlEndpoint("exit", endpoint))
	require.Equal(t, "ExitRequested", lifecycleSignal(endpoint))

	for _, action := range []string{"exit", "pause", "rollback_request", "resume"} {
		err := validateLifecycleControlEndpoint("legacy", restdef.Endpoint{
			LifecycleControl: restdef.LifecycleControl{Action: action, Signal: "Requested"},
		})
		require.ErrorContains(t, err, "unsupported action")
	}
}

func TestRESTServer_StreamEventsUnblocksOnStop(t *testing.T) {
	t.Parallel()

	server := streamServer()
	server.Queue.Timeout = "1s"
	state, baseURL := launchRESTServer(t, server, restdef.LimitProfile{})
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
