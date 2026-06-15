// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import "strings"

func (r *serverRuntime) monitorOpenAPI() map[string]interface{} {
	paths := map[string]interface{}{}
	for name, endpoint := range r.def.Server.Endpoints {
		if endpoint.MonitorView == "" || endpoint.MonitorView == "openapi" {
			continue
		}
		paths[endpoint.Path] = map[string]interface{}{
			strings.ToLower(endpoint.Method): map[string]interface{}{
				"operationId": monitorOperationID(name),
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "Cached monitor state",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": monitorResponseSchema(endpoint.MonitorView),
							},
						},
					},
				},
			},
		}
	}
	return map[string]interface{}{
		"openapi": "3.0.3",
		"info": map[string]interface{}{
			"title": "Agent Core Monitor API", "version": "v1",
		},
		"paths": paths,
	}
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
