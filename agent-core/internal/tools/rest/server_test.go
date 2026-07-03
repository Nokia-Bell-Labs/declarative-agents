// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestRESTServer_LaunchRegistersRoutes(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, controlServer(), LimitProfile{})
	defer stopRESTServer(t, state, "control")

	result := getJSON(t, baseURL+"/health")
	require.Equal(t, "ok", result["status"])
	require.Equal(t, "control", getJSON(t, baseURL+"/metadata")["server"])
}

func TestRESTServer_AwaitInboundSignals(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, controlServer(), LimitProfile{})
	defer stopRESTServer(t, state, "control")

	postStatus(t, baseURL+"/approve/123", `{}`, http.StatusAccepted)
	requireAwaitSignal(t, state, "control", "Approved")

	postStatus(t, baseURL+"/domain?signal=DomainEventReceived", `{}`, http.StatusAccepted)
	requireAwaitSignal(t, state, "control", "DomainEventReceived")

	postStatus(t, baseURL+"/domain?signal=Hidden", `{}`, http.StatusBadRequest)
	requireAwaitSignal(t, state, "control", "AwaitTimedOut")
}

func TestRESTServer_LifecycleControlEnqueuesSignals(t *testing.T) {
	t.Parallel()

	requireLifecycleControlEnqueuesSignal(t)
}

func TestRESTServer_ControlQueueAndReadPolicyConformance(t *testing.T) {
	t.Parallel()

	t.Run("lifecycle control enqueues signal", requireLifecycleControlEnqueuesSignal)
	t.Run("drop oldest keeps newest event", requireDropOldestQueuePolicy)
	t.Run("unsupported queue and drain policies fail validation", requireUnsupportedQueueAndDrainPoliciesRejected)
	t.Run("unsupported read policy rejected", requireUnsupportedReadPolicyRejected)
}

func requireLifecycleControlEnqueuesSignal(t *testing.T) {
	t.Helper()

	state, baseURL := launchRESTServer(t, lifecycleControlServer(), LimitProfile{})
	defer stopRESTServer(t, state, "lifecycle")

	postStatus(t, baseURL+"/lifecycle/exit", `{"reason":"operator"}`, http.StatusAccepted)
	event, signal, err := state.AwaitAny(AwaitAnyOptions{
		Sources: []AwaitSource{{Server: "lifecycle", Routes: []string{"exit"}}},
		Timeout: time.Second,
	})

	require.NoError(t, err)
	require.Equal(t, "ExitRequested", signal)
	require.Equal(t, "operator", event.Payload["reason"])
	require.Equal(t, "exit", event.Route)
}

func TestRESTAwaitEvent_MultiSourceFanIn(t *testing.T) {
	t.Parallel()

	state := NewServerState()
	_, _ = launchRESTServerWithState(t, state, namedControlServer("first"), LimitProfile{})
	defer stopRESTServer(t, state, "first")
	_, secondURL := launchRESTServerWithState(t, state, namedControlServer("second"), LimitProfile{})
	defer stopRESTServer(t, state, "second")

	postStatus(t, secondURL+"/approve/123", `{}`, http.StatusAccepted)
	event, signal, err := state.AwaitAny(AwaitAnyOptions{
		Sources: []AwaitSource{{Server: "first"}, {Server: "second"}},
		Timeout: time.Second,
	})
	require.NoError(t, err)
	require.Equal(t, "Approved", signal)
	require.Equal(t, "second", event.Source)
	require.Equal(t, "approve", event.Route)
}

func TestRESTAwaitEvent_SourceFiltersPreserveUnrelatedEvents(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, controlServer(), LimitProfile{})
	defer stopRESTServer(t, state, "control")

	postStatus(t, baseURL+"/domain?signal=DomainEventReceived", `{}`, http.StatusAccepted)
	postStatus(t, baseURL+"/approve/123", `{}`, http.StatusAccepted)
	event, signal, err := state.AwaitAny(AwaitAnyOptions{
		Sources: []AwaitSource{{
			Server: "control", Routes: []string{"approve"}, Signals: []string{"Approved"},
		}},
		Timeout: time.Second,
	})
	require.NoError(t, err)
	require.Equal(t, "Approved", signal)
	require.Equal(t, "approve", event.Route)

	preserved, preservedSignal, err := state.Await("control")
	require.NoError(t, err)
	require.Equal(t, "DomainEventReceived", preservedSignal)
	require.Equal(t, "domain", preserved.Route)
}

func TestRESTAwaitEvent_Timeout(t *testing.T) {
	t.Parallel()

	state, _ := launchRESTServer(t, namedControlServer("timeout"), LimitProfile{})
	defer stopRESTServer(t, state, "timeout")

	_, signal, err := state.AwaitAny(AwaitAnyOptions{
		Sources: []AwaitSource{{Server: "timeout"}}, Timeout: 10 * time.Millisecond,
	})
	require.NoError(t, err)
	require.Equal(t, "AwaitTimedOut", signal)
}

func TestRESTAwaitEvent_ServerStopped(t *testing.T) {
	t.Parallel()

	state, _ := launchRESTServer(t, namedControlServer("stopped"), LimitProfile{})
	results := make(chan core.Result, 1)
	go func() { results <- awaitAnyResult(state, AwaitSource{Server: "stopped"}) }()
	time.Sleep(20 * time.Millisecond)
	stopRESTServer(t, state, "stopped")
	require.Equal(t, core.Signal("ServerStopped"), (<-results).Signal)
}

func TestRESTAwaitEvent_StoppedSourceCommandError(t *testing.T) {
	t.Parallel()

	state, _ := launchRESTServer(t, namedControlServer("stopped_error"), LimitProfile{})
	source := AwaitSource{Server: "stopped_error", StoppedBehavior: StoppedSourceCommandError}
	results := make(chan core.Result, 1)
	go func() { results <- awaitAnyResult(state, source) }()
	time.Sleep(20 * time.Millisecond)
	stopRESTServer(t, state, "stopped_error")
	require.Equal(t, core.Signal("CommandError"), (<-results).Signal)
}

func TestRESTAwaitEvent_FactoryBuildsConfiguredCommand(t *testing.T) {
	t.Parallel()

	state := NewServerState()
	collection := NewCollection()
	require.NoError(t, collection.Add(Definition{Servers: map[string]Server{"control": controlServer()}}))
	_, baseURL := launchRESTServerWithState(t, state, controlServer(), LimitProfile{})
	defer stopRESTServer(t, state, "control")

	def := requireRESTToolDef(t, InitAwaitEvent)
	def.Config = map[string]interface{}{"sources": []interface{}{
		map[string]interface{}{"server": "control", "routes": []interface{}{"approve"}},
	}}
	command := requireRESTCommand(t, def, collection, state)
	postStatus(t, baseURL+"/approve/123", `{}`, http.StatusAccepted)
	result := command.Execute()

	require.Equal(t, core.Signal("Approved"), result.Signal, result.Output)
	require.Contains(t, result.Output, `"source":"control"`)
	require.Contains(t, result.Output, `"route":"approve"`)
	require.Equal(t, core.ToolDone, command.Undo(core.Result{}).Signal)
}

func TestRESTAwaitEvent_RejectsUnsupportedReadPolicy(t *testing.T) {
	t.Parallel()

	requireUnsupportedReadPolicyRejected(t)
}

func requireUnsupportedReadPolicyRejected(t *testing.T) {
	t.Helper()

	collection := NewCollection()
	require.NoError(t, collection.Add(Definition{Servers: map[string]Server{"control": controlServer()}}))
	def := requireRESTToolDef(t, InitAwaitEvent)
	def.Config = map[string]interface{}{
		"sources":     []interface{}{map[string]interface{}{"server": "control"}},
		"read_policy": "round_robin",
	}
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{Definitions: collection, ServerState: NewServerState()})
	factory, ok := br.Resolve(def.Init)
	require.True(t, ok)

	_, err := factory(def, nil)
	require.ErrorContains(t, err, "read_policy")
}

func TestRESTAwaitEvent_FactoryBuildsStagedFanIn(t *testing.T) {
	t.Parallel()

	state := NewServerState()
	collection := stagedFanInCollection(t)
	launchRESTServerCommand(t, collection, state, "first")
	defer stopRESTServer(t, state, "first")
	secondURL := launchRESTServerCommand(t, collection, state, "second")
	defer stopRESTServer(t, state, "second")

	postStatus(t, secondURL+"/approve/123", `{}`, http.StatusAccepted)
	firstAwait := awaitEventCommand(t, collection, state, "first", "second")
	result := firstAwait.Execute()
	requireAwaitEventOutput(t, result, "second", "SecondApproved")
	require.Equal(t, core.ToolDone, firstAwait.Undo(core.Result{}).Signal)

	thirdURL := launchRESTServerCommand(t, collection, state, "third")
	defer stopRESTServer(t, state, "third")
	postStatus(t, thirdURL+"/approve/456", `{}`, http.StatusAccepted)
	secondAwait := awaitEventCommand(t, collection, state, "first", "second", "third")
	result = secondAwait.Execute()
	requireAwaitEventOutput(t, result, "third", "ThirdApproved")
	require.Equal(t, core.ToolDone, secondAwait.Undo(core.Result{}).Signal)
}

func TestRESTServer_RejectsUndeclaredQueryAndHeader(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, validationServer(), LimitProfile{MaxRequestBytes: 128})
	defer stopRESTServer(t, state, "validation")

	postStatus(t, baseURL+"/approve/1?unexpected=value", `{}`, http.StatusBadRequest)
	requestStatusWithHeaders(t, http.MethodPost, baseURL+"/approve/1", `{}`, map[string]string{
		"X-Undeclared-Secret": "secret-value",
	}, http.StatusBadRequest)
	postStatus(t, baseURL+"/approve/abc", `{}`, http.StatusBadRequest)
	requestStatusWithHeaders(t, http.MethodPost, baseURL+"/approve/1", `{}`, map[string]string{
		"X-Approval-Token": "wrong-type",
	}, http.StatusBadRequest)

	requireAwaitSignal(t, state, "validation", "AwaitTimedOut")
}

func TestRESTServer_RedactsAwaitAndStreamOutput(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, redactionServer(), LimitProfile{})
	defer stopRESTServer(t, state, "redaction")

	requestStatusWithHeaders(t, http.MethodPost, baseURL+"/approve/123?token=query-secret",
		`{"secret":"body-secret"}`, map[string]string{"Authorization": "header-secret"}, http.StatusAccepted)
	await := awaitCommand(state, "redaction").Execute().Output
	require.NotContains(t, await, "query-secret")
	require.NotContains(t, await, "header-secret")
	require.NotContains(t, await, "body-secret")
	require.Contains(t, await, "[REDACTED]")

	requestStatusWithHeaders(t, http.MethodPost, baseURL+"/approve/456?token=query-secret",
		`{"secret":"body-secret"}`, map[string]string{"Authorization": "header-secret"}, http.StatusAccepted)
	stream := requestBody(t, http.MethodGet, baseURL+"/events", "", http.StatusOK)
	require.NotContains(t, stream, "query-secret")
	require.NotContains(t, stream, "header-secret")
	require.NotContains(t, stream, "body-secret")
	require.Contains(t, stream, "[REDACTED]")
}

func TestRESTServer_RedactsHandlerResponses(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, redactionServer(), LimitProfile{})
	defer stopRESTServer(t, state, "redaction")

	result := postJSON(t, baseURL+"/handle-secret", `{"secret":"body-secret"}`, http.StatusOK)
	require.Equal(t, "[REDACTED]", result["secret"])
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
		results := make(chan core.Result, 1)
		go func() { results <- awaitCommand(state, "blocking").Execute() }()
		time.Sleep(20 * time.Millisecond)
		require.Equal(t, "stopped", stopRESTServer(t, state, "blocking")["status"])
		require.Equal(t, core.Signal("ServerStopped"), (<-results).Signal)
	})
}

func TestRESTServer_QueueOverflowPolicies(t *testing.T) {
	t.Parallel()

	t.Run("drop oldest keeps newest event", requireDropOldestQueuePolicy)
	t.Run("unsupported queue and drain policies fail validation", requireUnsupportedQueueAndDrainPoliciesRejected)
}

func requireDropOldestQueuePolicy(t *testing.T) {
	t.Helper()

	server := namedControlServer("drop_oldest")
	server.Queue = QueueConfig{Name: "drop_oldest", Capacity: 1, Overflow: queueOverflowDropOldest, Timeout: "20ms"}
	state, baseURL := launchRESTServer(t, server, LimitProfile{})

	postStatus(t, baseURL+"/approve/old", `{}`, http.StatusAccepted)
	postStatus(t, baseURL+"/approve/new", `{}`, http.StatusAccepted)

	event, signal, err := state.Await("drop_oldest")
	require.NoError(t, err)
	require.Equal(t, "Approved", signal)
	require.Equal(t, "new", event.Payload["id"])
	require.Equal(t, float64(1), stopRESTServer(t, state, "drop_oldest")["dropped_events"])
}

func requireUnsupportedQueueAndDrainPoliciesRejected(t *testing.T) {
	t.Helper()

	server := namedControlServer("invalid")
	server.Queue.Overflow = "silently_drop"
	require.ErrorContains(t, ValidateDefinition(Definition{Version: "v1", Servers: map[string]Server{"invalid": server}}), "overflow")
	server.Queue.Overflow = queueOverflowReject
	for _, policy := range []string{"mystery", "reject_new", "drop_queued", "fail_queued"} {
		server.Shutdown.DrainPolicy = policy
		require.ErrorContains(t, ValidateDefinition(Definition{Version: "v1", Servers: map[string]Server{"invalid": server}}), "drain_policy")
	}
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

func TestRESTServer_RequestValidationFailures(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, validationServer(), LimitProfile{MaxRequestBytes: 12})
	defer stopRESTServer(t, state, "validation")

	postStatus(t, baseURL+"/approve/1", `{}`, http.StatusAccepted)
	postStatus(t, baseURL+"/approve/2", `{}`, http.StatusTooManyRequests)
	requestStatus(t, http.MethodGet, baseURL+"/approve/3", "", http.StatusMethodNotAllowed)
	postStatus(t, baseURL+"/typed", `{"name": 42}`, http.StatusBadRequest)
	postStatus(t, baseURL+"/typed", `{"name":"too large"}`, http.StatusRequestEntityTooLarge)
	postStatus(t, baseURL+"/handler", `{}`, http.StatusInternalServerError)

	requireAwaitSignal(t, state, "validation", "Approved")
	requireAwaitSignal(t, state, "validation", "AwaitTimedOut")
}

func TestRESTServer_InvokeHandlerBindings(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, handlerServer(), LimitProfile{})
	defer stopRESTServer(t, state, "handler")

	result := postJSON(t, baseURL+"/handle", `{"name":"alice"}`, http.StatusOK)
	require.Equal(t, true, result["handled"])
	require.Equal(t, "alice", result["name"])

	postStatus(t, baseURL+"/handle-signal", `{}`, http.StatusOK)
	requireAwaitSignal(t, state, "handler", "Handled")
}

func TestRESTServer_StreamEvents(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, streamServer(), LimitProfile{})
	defer stopRESTServer(t, state, "stream")

	postStatus(t, baseURL+"/approve/123", `{}`, http.StatusAccepted)
	body := requestBody(t, http.MethodGet, baseURL+"/events", "", http.StatusOK)
	require.Contains(t, body, "event: message")
	require.Contains(t, body, `"signal":"Approved"`)
	require.Contains(t, body, `"route":"approve"`)
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

func launchRESTServer(t *testing.T, server Server, limits LimitProfile) (*ServerState, string) {
	t.Helper()
	state := NewServerState()
	_, baseURL := launchRESTServerWithState(t, state, server, limits)
	return state, baseURL
}

func launchRESTServerWithState(
	t *testing.T,
	state *ServerState,
	server Server,
	limits LimitProfile,
) (map[string]interface{}, string) {
	t.Helper()
	def := ServerDefinition{Name: serverName(server), Server: server, Limits: limits}
	return launchRESTServerDefinition(t, state, def)
}

func launchRESTServerDefinition(
	t *testing.T,
	state *ServerState,
	def ServerDefinition,
) (map[string]interface{}, string) {
	t.Helper()
	result := ServerBuilder{
		ToolName: "rest_server_launch", Init: InitServerLaunch, Server: def, State: state,
	}.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("ServerLaunched"), result.Signal)
	var output map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	return output, "http://" + output["address"].(string)
}

func decodedLaunchAddress(t *testing.T, output string) string {
	t.Helper()
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(output), &decoded))
	return decoded["address"].(string)
}

func stopRESTServer(t *testing.T, state *ServerState, name string) map[string]interface{} {
	t.Helper()
	result := stopCommand(state, name).Execute()
	require.Equal(t, core.Signal("ServerStopped"), result.Signal, result.Output)
	var output map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	return output
}

func requireAwaitSignal(t *testing.T, state *ServerState, name, signal string) {
	t.Helper()
	result := awaitCommand(state, name).Execute()
	require.Equal(t, core.Signal(signal), result.Signal, result.Output)
}

func awaitCommand(state *ServerState, name string) core.Command {
	return ServerBuilder{
		ToolName: "rest_server_await", Init: InitServerAwait,
		Server: ServerDefinition{Name: name, Server: namedControlServer(name)}, State: state,
	}.Build(core.Result{})
}

func stopCommand(state *ServerState, name string) core.Command {
	return ServerBuilder{
		ToolName: "rest_server_stop", Init: InitServerStop,
		Server: ServerDefinition{Name: name, Server: namedControlServer(name)}, State: state,
	}.Build(core.Result{})
}

func awaitAnyResult(state *ServerState, source AwaitSource) core.Result {
	event, signal, err := state.AwaitAny(AwaitAnyOptions{
		Sources: []AwaitSource{source}, Timeout: time.Second,
	})
	output := map[string]interface{}{"source": event.Source}
	if err != nil {
		output["error"] = err.Error()
	}
	return core.Result{Signal: core.Signal(signal), Output: jsonOutput(output)}
}

func requireRESTToolDef(t *testing.T, init string) catalog.ToolDef {
	t.Helper()
	defs, err := catalog.LoadToolDefs(restDeclarationsPath(t))
	require.NoError(t, err)
	for _, def := range defs {
		if def.Init == init {
			return def
		}
	}
	require.Failf(t, "missing REST tool declaration", "init %q", init)
	return catalog.ToolDef{}
}

func requireRESTCommand(
	t *testing.T,
	def catalog.ToolDef,
	collection Collection,
	state *ServerState,
) core.Command {
	t.Helper()
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{Definitions: collection, ServerState: state})
	factory, ok := br.Resolve(def.Init)
	require.True(t, ok)
	builder, err := factory(def, nil)
	require.NoError(t, err)
	return builder.Build(core.Result{})
}

func launchRESTServerCommand(t *testing.T, collection Collection, state *ServerState, name string) string {
	t.Helper()
	def := requireRESTToolDef(t, InitServerLaunch)
	def.Name = "launch_" + name
	def.Config = map[string]interface{}{"rest_ref": name}
	result := requireRESTCommand(t, def, collection, state).Execute()
	require.Equal(t, core.Signal("ServerLaunched"), result.Signal, result.Output)
	return "http://" + decodedLaunchAddress(t, result.Output)
}

func awaitEventCommand(t *testing.T, collection Collection, state *ServerState, names ...string) core.Command {
	t.Helper()
	def := requireRESTToolDef(t, InitAwaitEvent)
	def.Config = map[string]interface{}{"sources": awaitEventSources(names)}
	return requireRESTCommand(t, def, collection, state)
}

func awaitEventSources(names []string) []interface{} {
	sources := make([]interface{}, 0, len(names))
	for _, name := range names {
		sources = append(sources, map[string]interface{}{"server": name})
	}
	return sources
}

func requireAwaitEventOutput(t *testing.T, result core.Result, source, signal string) {
	t.Helper()
	require.Equal(t, core.Signal(signal), result.Signal, result.Output)
	require.Contains(t, result.Output, `"source":"`+source+`"`)
	require.Contains(t, result.Output, `"signal":"`+signal+`"`)
}

func postStatus(t *testing.T, url, body string, want int) {
	t.Helper()
	requestStatus(t, http.MethodPost, url, body, want)
}

func requestStatus(t *testing.T, method, url, body string, want int) {
	t.Helper()
	requestStatusWithHeaders(t, method, url, body, nil, want)
}

func requestStatusWithHeaders(
	t *testing.T,
	method string,
	url string,
	body string,
	headers map[string]string,
	want int,
) {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	require.Equal(t, want, resp.StatusCode)
}

func requestBody(t *testing.T, method, url, body string, want int) string {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, want, resp.StatusCode, string(data))
	return string(data)
}

func postJSON(t *testing.T, url, body string, want int) map[string]interface{} {
	t.Helper()
	data := requestBody(t, http.MethodPost, url, body, want)
	var output map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(data), &output))
	return output
}

func getJSON(t *testing.T, url string) map[string]interface{} {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	var output map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&output))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	return output
}

func streamResponse(url string, bodyC chan<- string, errC chan<- error) {
	resp, err := http.Get(url)
	if err != nil {
		errC <- err
		return
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		errC <- err
		return
	}
	bodyC <- string(data)
	errC <- nil
}

func requireActiveStreams(t *testing.T, state *ServerState, name string, want int) {
	t.Helper()
	require.Eventually(t, func() bool {
		runtime, err := state.runtime(name)
		if err != nil {
			return false
		}
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		return runtime.activeStreams == want
	}, 500*time.Millisecond, 10*time.Millisecond)
}

func controlServer() Server {
	return namedControlServer("control")
}

func namedControlServer(name string) Server {
	return Server{
		Address:  "127.0.0.1:0",
		Queue:    QueueConfig{Name: name, Capacity: 8, Timeout: "20ms"},
		Shutdown: ShutdownConfig{Timeout: "200ms", DrainPolicy: "drain_then_stop"},
		Endpoints: map[string]Endpoint{
			"approve":  signalEndpoint("POST", "/approve/{id}", "Approved"),
			"domain":   dynamicEndpoint("POST", "/domain"),
			"health":   {Method: "GET", Path: "/health", Binding: bindingHealth},
			"metadata": {Method: "GET", Path: "/metadata", Binding: bindingStaticMetadata},
		},
	}
}

func stagedFanInCollection(t *testing.T) Collection {
	t.Helper()
	collection := NewCollection()
	require.NoError(t, collection.Add(Definition{Servers: map[string]Server{
		"first":  namedSignalServer("first", "FirstApproved"),
		"second": namedSignalServer("second", "SecondApproved"),
		"third":  namedSignalServer("third", "ThirdApproved"),
	}}))
	return collection
}

func namedSignalServer(name, signal string) Server {
	server := namedControlServer(name)
	approve := server.Endpoints["approve"]
	approve.Signal = signal
	server.Endpoints["approve"] = approve
	return server
}

func validationServer() Server {
	server := namedControlServer("validation")
	server.Queue = QueueConfig{Name: "validation", Capacity: 1, Timeout: "20ms"}
	approve := server.Endpoints["approve"]
	approve.Request.Path = map[string]interface{}{"id": map[string]interface{}{"type": "integer"}}
	approve.Request.Headers = map[string]interface{}{"x-approval-token": map[string]interface{}{"type": "integer"}}
	server.Endpoints["approve"] = approve
	server.Endpoints["typed"] = Endpoint{
		Method: "POST", Path: "/typed", Binding: bindingEmitSignal, Signal: "Typed",
		Request: RequestBinding{BodySchema: bodySchemaWithRequired("name")},
	}
	server.Endpoints["handler"] = Endpoint{
		Method: "POST", Path: "/handler", Binding: "invoke_handler",
	}
	return server
}

func shutdownValidationServer(name string) Server {
	server := namedControlServer(name)
	approve := server.Endpoints["approve"]
	approve.Request.Path = map[string]interface{}{"id": map[string]interface{}{"type": "string"}}
	server.Endpoints["approve"] = approve
	return server
}

func lifecycleControlServer() Server {
	return Server{
		Address:  "127.0.0.1:0",
		Queue:    QueueConfig{Name: "lifecycle", Capacity: 8, Timeout: "20ms", Overflow: queueOverflowReject},
		Shutdown: ShutdownConfig{Timeout: "200ms", DrainPolicy: "drain"},
		Endpoints: map[string]Endpoint{
			"exit": {
				Method: "POST", Path: "/lifecycle/exit", Binding: bindingLifecycleControl,
				LifecycleControl: LifecycleControl{
					Action: "exit", Signal: "ExitRequested",
					TargetSchema: bodySchemaWithRequired("reason"),
				},
				Request:  RequestBinding{BodySchema: bodySchemaWithRequired("reason")},
				Response: ResponseMapping{Output: map[string]string{"accepted": "true"}},
			},
		},
	}
}

func handlerServer() Server {
	return Server{
		Address: "127.0.0.1:0",
		Queue:   QueueConfig{Name: "handler", Capacity: 8, Timeout: "20ms"},
		Endpoints: map[string]Endpoint{
			"handle": {
				Method: "POST", Path: "/handle", Binding: bindingInvokeHandler,
				Request:  RequestBinding{BodySchema: bodySchemaWithRequired("name")},
				Response: ResponseMapping{Output: map[string]string{"handled": "true", "name": "$.name"}},
			},
			"handle_signal": {
				Method: "POST", Path: "/handle-signal", Binding: bindingInvokeHandler,
				Signal: "Handled", Response: ResponseMapping{Output: map[string]string{"accepted": "true"}},
			},
		},
	}
}

func streamServer() Server {
	server := namedControlServer("stream")
	server.Endpoints["events"] = Endpoint{Method: "GET", Path: "/events", Binding: bindingStreamEvents}
	return server
}

func signalEndpoint(method, path, signal string) Endpoint {
	return Endpoint{Method: method, Path: path, Binding: bindingEmitSignal, Signal: signal}
}

func dynamicEndpoint(method, path string) Endpoint {
	return Endpoint{
		Method: method, Path: path, Binding: bindingDynamicSignal,
		AllowedSignals: []string{"DomainEventReceived"},
		Request: RequestBinding{Query: map[string]interface{}{
			"signal": map[string]interface{}{"type": "string"},
		}},
	}
}

func bodySchemaWithRequired(field string) map[string]interface{} {
	return map[string]interface{}{
		"type": "object", "required": []interface{}{field},
		"properties": map[string]interface{}{field: map[string]interface{}{"type": "string"}},
	}
}

func boolPointer(value bool) *bool {
	return &value
}

func redactionServer() Server {
	server := namedControlServer("redaction")
	server.Endpoints["approve"] = redactionEndpoint()
	server.Endpoints["events"] = Endpoint{Method: "GET", Path: "/events", Binding: bindingStreamEvents}
	server.Endpoints["handle_secret"] = Endpoint{
		Method: "POST", Path: "/handle-secret", Binding: bindingInvokeHandler,
		Request:  RequestBinding{BodySchema: bodySchemaWithRequired("secret")},
		Response: ResponseMapping{Output: map[string]string{"secret": "$.secret"}, Redact: []string{"body.secret"}},
	}
	return server
}

func redactionEndpoint() Endpoint {
	return Endpoint{
		Method: "POST", Path: "/approve/{id}", Binding: bindingEmitSignal, Signal: "Approved",
		Request: RequestBinding{
			Path:       map[string]interface{}{"id": map[string]interface{}{"type": "string"}},
			Query:      map[string]interface{}{"token": map[string]interface{}{"type": "string"}},
			Headers:    map[string]interface{}{"authorization": map[string]interface{}{"type": "string"}},
			BodySchema: bodySchemaWithRequired("secret"),
		},
		Response: ResponseMapping{Redact: []string{"query.token", "headers.authorization", "body.secret"}},
	}
}

func serverName(server Server) string {
	if server.Queue.Name != "" {
		return server.Queue.Name
	}
	return "control"
}
