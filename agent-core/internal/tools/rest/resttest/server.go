// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package resttest

import (
	"testing"

	"github.com/stretchr/testify/require"

	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
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
func ControlServer() toolrest.Server {
	return NamedControlServer("control")
}

// NamedControlServer returns a loopback control server with a named queue.
func NamedControlServer(name string) toolrest.Server {
	return toolrest.Server{
		Address:  "127.0.0.1:0",
		Queue:    toolrest.QueueConfig{Name: name, Capacity: 8, Timeout: "20ms"},
		Shutdown: toolrest.ShutdownConfig{Timeout: "200ms", DrainPolicy: "drain_then_stop"},
		Endpoints: map[string]toolrest.Endpoint{
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

func namedActionEndpoint() toolrest.Endpoint {
	return toolrest.Endpoint{
		Method: "POST", Path: "/action", Binding: bindingDynamicSignal,
		AllowedSignals: []string{"ExperimentRequested", "Shutdown"},
		SignalField:    "body.type",
		SignalMapping: map[string]string{
			"launch_eval": "ExperimentRequested",
			"shutdown":    "Shutdown",
		},
		Request: toolrest.RequestBinding{BodySchema: BodySchemaWithRequired("type")},
	}
}

// HandlerServer is the invoke_handler loopback fixture.
func HandlerServer() toolrest.Server {
	return toolrest.Server{
		Address: "127.0.0.1:0",
		Queue:   toolrest.QueueConfig{Name: "handler", Capacity: 8, Timeout: "20ms"},
		Endpoints: map[string]toolrest.Endpoint{
			"handle": {
				Method: "POST", Path: "/handle", Binding: bindingInvokeHandler,
				Request:  toolrest.RequestBinding{BodySchema: BodySchemaWithRequired("name")},
				Response: toolrest.ResponseMapping{Output: map[string]string{"handled": "true", "name": "$.name"}},
			},
			"handle_signal": {
				Method: "POST", Path: "/handle-signal", Binding: bindingInvokeHandler,
				Signal: "Handled", Response: toolrest.ResponseMapping{Output: map[string]string{"accepted": "true"}},
			},
		},
	}
}

// StreamServer is the stream_events loopback fixture.
func StreamServer() toolrest.Server {
	server := NamedControlServer("stream")
	server.Endpoints["events"] = toolrest.Endpoint{Method: "GET", Path: "/events", Binding: bindingStreamEvents}
	return server
}

// LifecycleControlServer is the lifecycle-control loopback fixture.
func LifecycleControlServer() toolrest.Server {
	return toolrest.Server{
		Address:  "127.0.0.1:0",
		Queue:    toolrest.QueueConfig{Name: "lifecycle", Capacity: 8, Timeout: "20ms", Overflow: queueOverflowReject},
		Shutdown: toolrest.ShutdownConfig{Timeout: "200ms", DrainPolicy: "drain_then_stop"},
		Endpoints: map[string]toolrest.Endpoint{
			"exit": lifecycleExitEndpoint(),
		},
	}
}

func lifecycleExitEndpoint() toolrest.Endpoint {
	return toolrest.Endpoint{
		Method: "POST", Path: "/lifecycle/exit", Binding: bindingLifecycleControl,
		LifecycleControl: toolrest.LifecycleControl{
			Action: "enqueue_signal", Signal: "ExitRequested",
			TargetSchema: BodySchemaWithRequired("reason"),
		},
		Request:  toolrest.RequestBinding{BodySchema: BodySchemaWithRequired("reason")},
		Response: toolrest.ResponseMapping{Output: map[string]string{"accepted": "true"}},
	}
}

// SignalEndpoint builds an emit_signal endpoint.
func SignalEndpoint(method, path, signal string) toolrest.Endpoint {
	return toolrest.Endpoint{Method: method, Path: path, Binding: bindingEmitSignal, Signal: signal}
}

// DynamicEndpoint builds an emit_dynamic_signal endpoint.
func DynamicEndpoint(method, path string) toolrest.Endpoint {
	return toolrest.Endpoint{
		Method: method, Path: path, Binding: bindingDynamicSignal,
		AllowedSignals: []string{"DomainEventReceived"},
		Request: toolrest.RequestBinding{Query: map[string]interface{}{
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
	require.NoError(t, collection.Add(toolrest.Definition{Servers: map[string]toolrest.Server{
		"first":  namedSignalServer("first", "FirstApproved"),
		"second": namedSignalServer("second", "SecondApproved"),
		"third":  namedSignalServer("third", "ThirdApproved"),
	}}))
	return collection
}

func namedSignalServer(name, signal string) toolrest.Server {
	server := NamedControlServer(name)
	approve := server.Endpoints["approve"]
	approve.Signal = signal
	server.Endpoints["approve"] = approve
	return server
}
