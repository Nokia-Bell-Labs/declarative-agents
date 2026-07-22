// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
	"time"
)

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

func startRESTAwait(t *testing.T, await func() core.Result) <-chan core.Result {
	t.Helper()
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
