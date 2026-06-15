// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import "strings"

func (r *serverRuntime) monitorOpenAPI() map[string]interface{} {
	paths := map[string]interface{}{}
	for name, endpoint := range r.def.Server.Endpoints {
		operation := monitorEndpointOperation(name, endpoint)
		if operation == nil {
			continue
		}
		addMonitorPathOperation(paths, endpoint, operation)
	}
	return map[string]interface{}{
		"openapi": "3.0.3",
		"info": map[string]interface{}{
			"title": "Agent Core Monitor API", "version": "v1",
		},
		"paths": paths,
	}
}

func monitorEndpointOperation(name string, endpoint Endpoint) map[string]interface{} {
	switch {
	case endpoint.MonitorView != "" && endpoint.MonitorView != "openapi":
		return monitorReadOperation(name, endpoint.MonitorView)
	case monitorControlEndpoint(endpoint):
		return monitorControlOperation(name, endpoint)
	default:
		return nil
	}
}

func monitorReadOperation(name, view string) map[string]interface{} {
	return map[string]interface{}{
		"operationId": monitorOperationID(name),
		"responses":   monitorResponses("200", "Cached monitor state", monitorResponseSchema(view)),
	}
}

func monitorControlOperation(name string, endpoint Endpoint) map[string]interface{} {
	return map[string]interface{}{
		"operationId": monitorOperationID(name),
		"requestBody": monitorRequestBody(endpoint.Request.BodySchema),
		"responses":   monitorResponses("202", "Control request accepted", monitorControlResponseSchema()),
	}
}

func addMonitorPathOperation(paths map[string]interface{}, endpoint Endpoint, operation map[string]interface{}) {
	pathItem, _ := paths[endpoint.Path].(map[string]interface{})
	if pathItem == nil {
		pathItem = map[string]interface{}{}
		paths[endpoint.Path] = pathItem
	}
	pathItem[strings.ToLower(endpoint.Method)] = operation
}

func monitorOperationID(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_' || r == ' '
	})
	for i := range parts {
		if parts[i] == "" {
			continue
		}
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return "monitor" + strings.Join(parts, "")
}

func monitorControlEndpoint(endpoint Endpoint) bool {
	return endpoint.Binding == bindingEmitSignal && strings.HasPrefix(endpoint.Path, "/monitor/control/")
}

func monitorResponses(status, description string, schema map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		status: map[string]interface{}{
			"description": description,
			"content": map[string]interface{}{
				"application/json": map[string]interface{}{"schema": schema},
			},
		},
	}
}

func monitorRequestBody(schema map[string]interface{}) map[string]interface{} {
	if len(schema) == 0 {
		schema = schemaObject()
	}
	return map[string]interface{}{
		"required": false,
		"content": map[string]interface{}{
			"application/json": map[string]interface{}{"schema": schema},
		},
	}
}

func monitorControlResponseSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"accepted": map[string]interface{}{"type": "boolean"},
			"signal":   map[string]interface{}{"type": "string"},
		},
	}
}

func monitorResponseSchema(view string) map[string]interface{} {
	switch view {
	case monitorViewMachine:
		return schemaObject("name", "states", "signals", "terminal_states", "metric_labels", "transitions")
	case monitorViewState:
		return schemaObject(
			"run", "run_id", "status", "state", "signal", "iteration", "updated_at",
			"diagnostics", "stage", "message", "metric", "tool_name", "timestamp",
			"errors", "command_name",
		)
	case monitorViewTools:
		return schemaObject("tools", "name", "category", "visibility", "emits", "metrics", "instruments", "attributes", "relationships")
	case monitorViewMetrics:
		return schemaObject(
			"tools", "metrics", "schemas", "recent_samples", "diagnostics",
			"tool_name", "run_id", "state", "signal", "status", "updated_at", "last_value",
		)
	case monitorViewEvents:
		return schemaObject("recent_events", "command_name", "from_state", "to_state", "duration_ms", "tokens_in", "tokens_out")
	default:
		return schemaObject("data")
	}
}

func schemaObject(fields ...string) map[string]interface{} {
	properties := map[string]interface{}{}
	for _, field := range fields {
		properties[field] = map[string]interface{}{"type": "object"}
	}
	return map[string]interface{}{"type": "object", "properties": properties}
}
