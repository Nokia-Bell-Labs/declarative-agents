// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"net/http"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	"github.com/stretchr/testify/require"
)

func skipIfShortRESTLaunch(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration-grade: production REST server launch binds a real loopback listener")
	}
}

func requireLifecycleControlEnqueuesSignal(t *testing.T) {
	t.Helper()

	state, baseURL := launchRESTServer(t, lifecycleControlServer(), restdef.LimitProfile{})
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

func requireUnsupportedReadPolicyRejected(t *testing.T) {
	t.Helper()

	collection := NewCollection()
	require.NoError(t, collection.Add(restdef.Definition{Servers: map[string]restdef.Server{"control": controlServer()}}))
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

func startRESTAwait(t *testing.T, await func() core.Result) <-chan core.Result {
	t.Helper()
	if testing.Short() {
		t.Skip("integration-grade: multi-second wall-clock wait")
	}
	started := make(chan struct{})
	results := make(chan core.Result, 1)
	go func() {
		close(started)
		results <- await()
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out starting REST await")
	}
	return results
}

func requireAwaitBlocked(t *testing.T, results <-chan core.Result) {
	t.Helper()
	select {
	case result := <-results:
		t.Fatalf("await returned before server stop: signal=%s output=%s", result.Signal, result.Output)
	default:
	}
}

func requireRESTResult(t *testing.T, results <-chan core.Result) core.Result {
	t.Helper()
	if testing.Short() {
		t.Skip("integration-grade: multi-second wall-clock wait")
	}
	select {
	case result := <-results:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for REST command result")
		return core.Result{}
	}
}

func requireDropOldestQueuePolicy(t *testing.T) {
	t.Helper()

	server := namedControlServer("drop_oldest")
	server.Queue = restdef.QueueConfig{Name: "drop_oldest", Capacity: 1, Overflow: queueOverflowDropOldest, Timeout: "20ms"}
	state, baseURL := launchRESTServer(t, server, restdef.LimitProfile{})

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
	require.ErrorContains(t, ValidateDefinition(restdef.Definition{Version: "v1", Servers: map[string]restdef.Server{"invalid": server}}), "overflow")
	server.Queue.Overflow = queueOverflowReject
	for _, policy := range []string{"mystery", "reject_new", "drop_queued", "fail_queued"} {
		server.Shutdown.DrainPolicy = policy
		require.ErrorContains(t, ValidateDefinition(restdef.Definition{Version: "v1", Servers: map[string]restdef.Server{"invalid": server}}), "drain_policy")
	}
}

func controlServer() restdef.Server {
	return namedControlServer("control")
}

func namedControlServer(name string) restdef.Server {
	return restdef.Server{
		Address:  "127.0.0.1:0",
		Queue:    restdef.QueueConfig{Name: name, Capacity: 8, Timeout: "20ms"},
		Shutdown: restdef.ShutdownConfig{Timeout: "200ms", DrainPolicy: "drain_then_stop"},
		Endpoints: map[string]restdef.Endpoint{
			"approve": signalEndpoint("POST", "/approve/{id}", "Approved"),
			"domain":  dynamicEndpoint("POST", "/domain"),
			"action": {
				Method: "POST", Path: "/action", Binding: bindingDynamicSignal,
				AllowedSignals: []string{"ExperimentRequested", "Shutdown"},
				SignalField:    "body.type",
				SignalMapping: map[string]string{
					"launch_eval": "ExperimentRequested",
					"shutdown":    "Shutdown",
				},
				Request: restdef.RequestBinding{BodySchema: bodySchemaWithRequired("type")},
			},
			"health":   {Method: "GET", Path: "/health", Binding: bindingHealth},
			"metadata": {Method: "GET", Path: "/metadata", Binding: bindingStaticMetadata},
		},
	}
}

func stagedFanInCollection(t *testing.T) Collection {
	t.Helper()
	collection := NewCollection()
	require.NoError(t, collection.Add(restdef.Definition{Servers: map[string]restdef.Server{
		"first":  namedSignalServer("first", "FirstApproved"),
		"second": namedSignalServer("second", "SecondApproved"),
		"third":  namedSignalServer("third", "ThirdApproved"),
	}}))
	return collection
}

func namedSignalServer(name, signal string) restdef.Server {
	server := namedControlServer(name)
	approve := server.Endpoints["approve"]
	approve.Signal = signal
	server.Endpoints["approve"] = approve
	return server
}

func validationServer() restdef.Server {
	server := namedControlServer("validation")
	server.Queue = restdef.QueueConfig{Name: "validation", Capacity: 1, Timeout: "20ms"}
	approve := server.Endpoints["approve"]
	approve.Request.Path = map[string]interface{}{"id": map[string]interface{}{"type": "integer"}}
	approve.Request.Headers = map[string]interface{}{"x-approval-token": map[string]interface{}{"type": "integer"}}
	server.Endpoints["approve"] = approve
	server.Endpoints["typed"] = restdef.Endpoint{
		Method: "POST", Path: "/typed", Binding: bindingEmitSignal, Signal: "Typed",
		Request: restdef.RequestBinding{BodySchema: bodySchemaWithRequired("name")},
	}
	server.Endpoints["handler"] = restdef.Endpoint{
		Method: "POST", Path: "/handler", Binding: "invoke_handler",
	}
	return server
}

func shutdownValidationServer(name string) restdef.Server {
	server := namedControlServer(name)
	approve := server.Endpoints["approve"]
	approve.Request.Path = map[string]interface{}{"id": map[string]interface{}{"type": "string"}}
	server.Endpoints["approve"] = approve
	return server
}

func lifecycleControlServer() restdef.Server {
	return restdef.Server{
		Address:  "127.0.0.1:0",
		Queue:    restdef.QueueConfig{Name: "lifecycle", Capacity: 8, Timeout: "20ms", Overflow: queueOverflowReject},
		Shutdown: restdef.ShutdownConfig{Timeout: "200ms", DrainPolicy: "drain_then_stop"},
		Endpoints: map[string]restdef.Endpoint{
			"exit": {
				Method: "POST", Path: "/lifecycle/exit", Binding: bindingLifecycleControl,
				LifecycleControl: restdef.LifecycleControl{
					Action: "enqueue_signal", Signal: "ExitRequested",
					TargetSchema: bodySchemaWithRequired("reason"),
				},
				Request:  restdef.RequestBinding{BodySchema: bodySchemaWithRequired("reason")},
				Response: restdef.ResponseMapping{Output: map[string]string{"accepted": "true"}},
			},
		},
	}
}

// bareLifecycleServer declares no exit route, so agent-core's canonical
// lifecycle-exit injection is what makes POST /api/lifecycle/exit answer.
func bareLifecycleServer(name string) restdef.Server {
	return restdef.Server{
		Address:  "127.0.0.1:0",
		Queue:    restdef.QueueConfig{Name: name, Capacity: 8, Timeout: "20ms", Overflow: queueOverflowReject},
		Shutdown: restdef.ShutdownConfig{Timeout: "200ms", DrainPolicy: "drain_then_stop"},
		Endpoints: map[string]restdef.Endpoint{
			"health": {Method: "GET", Path: "/health", Binding: bindingHealth},
		},
	}
}

func handlerServer() restdef.Server {
	return restdef.Server{
		Address: "127.0.0.1:0",
		Queue:   restdef.QueueConfig{Name: "handler", Capacity: 8, Timeout: "20ms"},
		Endpoints: map[string]restdef.Endpoint{
			"handle": {
				Method: "POST", Path: "/handle", Binding: bindingInvokeHandler,
				Request:  restdef.RequestBinding{BodySchema: bodySchemaWithRequired("name")},
				Response: restdef.ResponseMapping{Output: map[string]string{"handled": "true", "name": "$.name"}},
			},
			"handle_signal": {
				Method: "POST", Path: "/handle-signal", Binding: bindingInvokeHandler,
				Signal: "Handled", Response: restdef.ResponseMapping{Output: map[string]string{"accepted": "true"}},
			},
		},
	}
}

func streamServer() restdef.Server {
	server := namedControlServer("stream")
	server.Endpoints["events"] = restdef.Endpoint{Method: "GET", Path: "/events", Binding: bindingStreamEvents}
	return server
}

func signalEndpoint(method, path, signal string) restdef.Endpoint {
	return restdef.Endpoint{Method: method, Path: path, Binding: bindingEmitSignal, Signal: signal}
}

func dynamicEndpoint(method, path string) restdef.Endpoint {
	return restdef.Endpoint{
		Method: method, Path: path, Binding: bindingDynamicSignal,
		AllowedSignals: []string{"DomainEventReceived"},
		Request: restdef.RequestBinding{Query: map[string]interface{}{
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

func redactionServer() restdef.Server {
	server := namedControlServer("redaction")
	server.Endpoints["approve"] = redactionEndpoint()
	server.Endpoints["events"] = restdef.Endpoint{Method: "GET", Path: "/events", Binding: bindingStreamEvents}
	server.Endpoints["handle_secret"] = restdef.Endpoint{
		Method: "POST", Path: "/handle-secret", Binding: bindingInvokeHandler,
		Request:  restdef.RequestBinding{BodySchema: bodySchemaWithRequired("secret")},
		Response: restdef.ResponseMapping{Output: map[string]string{"secret": "$.secret"}, Redact: []string{"body.secret"}},
	}
	return server
}

func redactionEndpoint() restdef.Endpoint {
	return restdef.Endpoint{
		Method: "POST", Path: "/approve/{id}", Binding: bindingEmitSignal, Signal: "Approved",
		Request: restdef.RequestBinding{
			Path:       map[string]interface{}{"id": map[string]interface{}{"type": "string"}},
			Query:      map[string]interface{}{"token": map[string]interface{}{"type": "string"}},
			Headers:    map[string]interface{}{"authorization": map[string]interface{}{"type": "string"}},
			BodySchema: bodySchemaWithRequired("secret"),
		},
		Response: restdef.ResponseMapping{Redact: []string{"query.token", "headers.authorization", "body.secret"}},
	}
}

func serverName(server restdef.Server) string {
	if server.Queue.Name != "" {
		return server.Queue.Name
	}
	return "control"
}
