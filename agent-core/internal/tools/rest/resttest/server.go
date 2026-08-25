// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package resttest

import (
	"testing"

	"github.com/stretchr/testify/require"

	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
)

const (
	bindingEmitSignal       = "emit_signal"
	bindingDynamicSignal    = "emit_dynamic_signal"
	bindingHealth           = "health"
	bindingStaticMetadata   = "static_metadata"
	bindingLifecycleControl = "lifecycle_control"
	bindingInvokeHandler    = "invoke_handler"
	bindingStreamEvents     = "stream_events"
	queueOverflowReject     = "reject"
)

// ControlServer is the shared loopback control-server fixture.
func ControlServer() restdef.Server {
	return NamedControlServer("control")
}

// NamedControlServer returns a loopback control server with a named queue.
func NamedControlServer(name string) restdef.Server {
	return restdef.Server{
		Address:  "127.0.0.1:0",
		Queue:    restdef.QueueConfig{Name: name, Capacity: 8, Timeout: "20ms"},
		Shutdown: restdef.ShutdownConfig{Timeout: "200ms", DrainPolicy: "drain_then_stop"},
		Endpoints: map[string]restdef.Endpoint{
			"approve": SignalEndpoint("POST", "/approve/{id}", "Approved"),
			"domain":  DynamicEndpoint("POST", "/domain"),
			"action":  namedActionEndpoint(),
			"health":  {Method: "GET", Path: "/health", Binding: bindingHealth},
			"metadata": {
				Method: "GET", Path: "/metadata", Binding: bindingStaticMetadata,
			},
		},
	}
}

func namedActionEndpoint() restdef.Endpoint {
	return restdef.Endpoint{
		Method: "POST", Path: "/action", Binding: bindingDynamicSignal,
		AllowedSignals: []string{"ExperimentRequested", "Shutdown"},
		SignalField:    "body.type",
		SignalMapping: map[string]string{
			"launch_eval": "ExperimentRequested",
			"shutdown":    "Shutdown",
		},
		Request: restdef.RequestBinding{BodySchema: BodySchemaWithRequired("type")},
	}
}

// HandlerServer is the invoke_handler loopback fixture.
func HandlerServer() restdef.Server {
	return restdef.Server{
		Address: "127.0.0.1:0",
		Queue:   restdef.QueueConfig{Name: "handler", Capacity: 8, Timeout: "20ms"},
		Endpoints: map[string]restdef.Endpoint{
			"handle": {
				Method: "POST", Path: "/handle", Binding: bindingInvokeHandler,
				Request:  restdef.RequestBinding{BodySchema: BodySchemaWithRequired("name")},
				Response: restdef.ResponseMapping{Output: map[string]string{"handled": "true", "name": "$.name"}},
			},
			"handle_signal": {
				Method: "POST", Path: "/handle-signal", Binding: bindingInvokeHandler,
				Signal: "Handled", Response: restdef.ResponseMapping{Output: map[string]string{"accepted": "true"}},
			},
		},
	}
}

// StreamServer is the stream_events loopback fixture.
func StreamServer() restdef.Server {
	server := NamedControlServer("stream")
	server.Endpoints["events"] = restdef.Endpoint{Method: "GET", Path: "/events", Binding: bindingStreamEvents}
	return server
}

// LifecycleControlServer is the lifecycle-control loopback fixture.
func LifecycleControlServer() restdef.Server {
	return restdef.Server{
		Address:  "127.0.0.1:0",
		Queue:    restdef.QueueConfig{Name: "lifecycle", Capacity: 8, Timeout: "20ms", Overflow: queueOverflowReject},
		Shutdown: restdef.ShutdownConfig{Timeout: "200ms", DrainPolicy: "drain_then_stop"},
		Endpoints: map[string]restdef.Endpoint{
			"exit": lifecycleExitEndpoint(),
		},
	}
}

func lifecycleExitEndpoint() restdef.Endpoint {
	return restdef.Endpoint{
		Method: "POST", Path: "/lifecycle/exit", Binding: bindingLifecycleControl,
		LifecycleControl: restdef.LifecycleControl{
			Action: "enqueue_signal", Signal: "ExitRequested",
			TargetSchema: BodySchemaWithRequired("reason"),
		},
		Request:  restdef.RequestBinding{BodySchema: BodySchemaWithRequired("reason")},
		Response: restdef.ResponseMapping{Output: map[string]string{"accepted": "true"}},
	}
}

// SignalEndpoint builds an emit_signal endpoint.
func SignalEndpoint(method, path, signal string) restdef.Endpoint {
	return restdef.Endpoint{Method: method, Path: path, Binding: bindingEmitSignal, Signal: signal}
}

// DynamicEndpoint builds an emit_dynamic_signal endpoint.
func DynamicEndpoint(method, path string) restdef.Endpoint {
	return restdef.Endpoint{
		Method: method, Path: path, Binding: bindingDynamicSignal,
		AllowedSignals: []string{"DomainEventReceived"},
		Request: restdef.RequestBinding{Query: map[string]interface{}{
			"signal": map[string]interface{}{"type": "string"},
		}},
	}
}

// BodySchemaWithRequired is an object schema that requires one string field.
func BodySchemaWithRequired(field string) map[string]interface{} {
	return map[string]interface{}{
		"type": "object", "required": []interface{}{field},
		"properties": map[string]interface{}{field: map[string]interface{}{"type": "string"}},
	}
}

// StagedFanInCollection is three named signal servers for await-any tests.
func StagedFanInCollection(t *testing.T) toolrest.Collection {
	t.Helper()
	collection := toolrest.NewCollection()
	require.NoError(t, collection.Add(restdef.Definition{Servers: map[string]restdef.Server{
		"first":  namedSignalServer("first", "FirstApproved"),
		"second": namedSignalServer("second", "SecondApproved"),
		"third":  namedSignalServer("third", "ThirdApproved"),
	}}))
	return collection
}

func namedSignalServer(name, signal string) restdef.Server {
	server := NamedControlServer(name)
	approve := server.Endpoints["approve"]
	approve.Signal = signal
	server.Endpoints["approve"] = approve
	return server
}
