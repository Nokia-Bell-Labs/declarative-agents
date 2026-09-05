// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func monitorOpenAPIOperation(t *testing.T, doc map[string]interface{}, path string, method string) map[string]interface{} {
	t.Helper()
	paths, _ := doc["paths"].(map[string]interface{})
	pathItem, _ := paths[path].(map[string]interface{})
	require.NotNil(t, pathItem, "path %s should be documented", path)
	operation, _ := pathItem[method].(map[string]interface{})
	require.NotNil(t, operation, "%s %s should be documented", method, path)
	return operation
}

func requireMonitorOpenAPIPaths(t *testing.T, doc map[string]interface{}) {
	t.Helper()
	paths, _ := doc["paths"].(map[string]interface{})
	for _, path := range []string{
		"/monitor/machine", "/monitor/machines", "/monitor/state", "/monitor/tools", "/monitor/metrics",
		"/monitor/events", "/monitor/events/stream", "/monitor/control/exit",
	} {
		require.Contains(t, paths, path)
	}
}

func requireMonitorOpenAPISchemaTypes(
	t *testing.T,
	doc map[string]interface{},
	control map[string]interface{},
) {
	t.Helper()
	requireMonitorStateOpenAPISchema(t, doc)
	requireMonitorDeclaredMachinesOpenAPISchema(t, doc)
	requireMonitorDeclaredToolsOpenAPISchema(t, doc)
	requireMonitorMetricsOpenAPISchema(t, doc)
	requireMonitorEventsOpenAPISchema(t, doc)
	requireMonitorStreamOpenAPIContent(t, doc)
	requireMonitorControlOpenAPISchema(t, doc, control)
}

func requireMonitorOpenAPIMatchesRuntimeViews(t *testing.T, doc map[string]interface{}, baseURL string) {
	t.Helper()
	for _, path := range []string{"/monitor/machine", "/monitor/state", "/monitor/metrics", "/monitor/events"} {
		schema := monitorOpenAPIResponseSchema(t, doc, path, "get", "200")
		requireSchemaCoversRuntimeValue(t, schema, getJSON(t, baseURL+path))
	}
	schema := monitorOpenAPIResponseSchema(t, doc, "/monitor/machines", "get", "200")
	body := requestBody(t, http.MethodGet, baseURL+"/monitor/machines", "", http.StatusOK)
	var machines []interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &machines))
	requireSchemaCoversRuntimeValue(t, schema, machines)
}

func requireSchemaCoversRuntimeValue(t *testing.T, schema map[string]interface{}, value interface{}) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]interface{}:
		if _, ok := schema["additionalProperties"]; ok {
			return
		}
		props, _ := schema["properties"].(map[string]interface{})
		for name, item := range typed {
			child, _ := props[name].(map[string]interface{})
			require.NotNil(t, child, "schema should cover runtime field %q", name)
			requireSchemaCoversRuntimeValue(t, child, item)
		}
	case []interface{}:
		if len(typed) == 0 {
			return
		}
		requireSchemaCoversRuntimeValue(t, schemaItems(t, schema), typed[0])
	}
}

func requireMonitorStateOpenAPISchema(t *testing.T, doc map[string]interface{}) {
	t.Helper()
	stateSchema := monitorOpenAPIResponseSchema(t, doc, "/monitor/state", "get", "200")
	requireSchemaType(t, schemaProperty(t, stateSchema, "run", "run_id"), "string")
	requireSchemaType(t, schemaProperty(t, stateSchema, "run", "state"), "string")
	requireSchemaType(t, schemaProperty(t, stateSchema, "run", "status"), "string")
	requireSchemaType(t, schemaProperty(t, stateSchema, "run", "iteration"), "integer")
	requireSchemaFormat(t, schemaProperty(t, stateSchema, "run", "updated_at"), "date-time")
	requireSchemaType(t, schemaProperty(t, stateSchema, "diagnostics"), "array")
}

func requireMonitorDeclaredMachinesOpenAPISchema(t *testing.T, doc map[string]interface{}) {
	t.Helper()
	schema := monitorOpenAPIResponseSchema(t, doc, "/monitor/machines", "get", "200")
	requireSchemaType(t, schema, "array")
	machine := schemaItems(t, schema)
	requireSchemaType(t, schemaProperty(t, machine, "name"), "string")
	requireSchemaType(t, schemaProperty(t, machine, "initial_state"), "string")
	requireSchemaType(t, schemaProperty(t, machine, "signals"), "array")
	requireSchemaType(t, schemaProperty(t, machine, "terminal_states"), "array")
	requireSchemaType(t, schemaProperty(t, machine, "transitions"), "array")
	state := schemaItems(t, schemaProperty(t, machine, "states"))
	requireSchemaType(t, schemaProperty(t, state, "name"), "string")
	requireSchemaType(t, schemaProperty(t, state, "tags"), "array")
	viewTag := schemaItems(t, schemaProperty(t, machine, "view_tags"))
	requireSchemaType(t, schemaProperty(t, viewTag, "tag"), "string")
	requireSchemaType(t, schemaProperty(t, viewTag, "label"), "string")
}

func requireMonitorDeclaredToolsOpenAPISchema(t *testing.T, doc map[string]interface{}) {
	t.Helper()
	schema := monitorOpenAPIResponseSchema(t, doc, "/monitor/tools/declared", "get", "200")
	requireSchemaType(t, schema, "array")
	word := schemaItems(t, schema)
	requireSchemaType(t, schemaProperty(t, word, "name"), "string")
	requireSchemaType(t, schemaProperty(t, word, "description"), "string")
	require.Equal(t, true, word["additionalProperties"])
}

func requireMonitorMetricsOpenAPISchema(t *testing.T, doc map[string]interface{}) {
	t.Helper()
	metricsSchema := monitorOpenAPIResponseSchema(t, doc, "/monitor/metrics", "get", "200")
	requireSchemaMap(t, schemaProperty(t, metricsSchema, "tools"))
	requireSchemaMap(t, schemaProperty(t, metricsSchema, "metrics"))
	requireSchemaMap(t, schemaProperty(t, metricsSchema, "schemas"))
	sampleSchema := schemaItems(t, schemaProperty(t, metricsSchema, "recent_samples"))
	requireSchemaType(t, schemaProperty(t, sampleSchema, "value"), "number")
	requireSchemaFormat(t, schemaProperty(t, sampleSchema, "timestamp"), "date-time")
}

func requireMonitorEventsOpenAPISchema(t *testing.T, doc map[string]interface{}) {
	t.Helper()
	eventsSchema := monitorOpenAPIResponseSchema(t, doc, "/monitor/events", "get", "200")
	eventSchema := schemaItems(t, schemaProperty(t, eventsSchema, "recent_events"))
	requireSchemaType(t, schemaProperty(t, eventSchema, "iteration"), "integer")
	requireSchemaType(t, schemaProperty(t, eventSchema, "duration_ms"), "number")
	requireSchemaType(t, schemaProperty(t, eventSchema, "tokens_in"), "integer")
	requireSchemaFormat(t, schemaProperty(t, eventSchema, "timestamp"), "date-time")
}

func requireMonitorStreamOpenAPIContent(t *testing.T, doc map[string]interface{}) {
	t.Helper()
	streamContent := monitorOpenAPIResponseContent(t, doc, "/monitor/events/stream", "get", "200")
	require.Contains(t, streamContent, "text/event-stream")
	require.NotContains(t, streamContent, "application/json")
	eventsContent := monitorOpenAPIResponseContent(t, doc, "/monitor/events", "get", "200")
	require.Contains(t, eventsContent, "application/json")
	require.NotContains(t, eventsContent, "text/event-stream")
}

func requireMonitorControlOpenAPISchema(
	t *testing.T,
	doc map[string]interface{},
	control map[string]interface{},
) {
	t.Helper()
	requireSchemaType(t, monitorOpenAPIRequestSchema(t, control, "reason"), "string")
	controlSchema := monitorOpenAPIResponseSchema(t, doc, "/monitor/control/exit", "post", "202")
	requireSchemaType(t, schemaProperty(t, controlSchema, "accepted"), "boolean")
	requireSchemaType(t, schemaProperty(t, controlSchema, "signal"), "string")
}

func monitorOpenAPIResponseSchema(
	t *testing.T,
	doc map[string]interface{},
	path string,
	method string,
	status string,
) map[string]interface{} {
	t.Helper()
	operation := monitorOpenAPIOperation(t, doc, path, method)
	responses, _ := operation["responses"].(map[string]interface{})
	response, _ := responses[status].(map[string]interface{})
	content, _ := response["content"].(map[string]interface{})
	jsonContent, _ := content["application/json"].(map[string]interface{})
	schema, _ := jsonContent["schema"].(map[string]interface{})
	require.NotNil(t, schema, "%s %s response %s should have schema", method, path, status)
	return schema
}

func monitorOpenAPIResponseContent(
	t *testing.T,
	doc map[string]interface{},
	path string,
	method string,
	status string,
) map[string]interface{} {
	t.Helper()
	operation := monitorOpenAPIOperation(t, doc, path, method)
	responses, _ := operation["responses"].(map[string]interface{})
	response, _ := responses[status].(map[string]interface{})
	content, _ := response["content"].(map[string]interface{})
	require.NotNil(t, content, "%s %s response %s should have content", method, path, status)
	return content
}

func monitorOpenAPIRequestSchema(t *testing.T, operation map[string]interface{}, field string) map[string]interface{} {
	t.Helper()
	requestBody, _ := operation["requestBody"].(map[string]interface{})
	content, _ := requestBody["content"].(map[string]interface{})
	jsonContent, _ := content["application/json"].(map[string]interface{})
	schema, _ := jsonContent["schema"].(map[string]interface{})
	return schemaProperty(t, schema, field)
}

func schemaProperty(t *testing.T, schema map[string]interface{}, fields ...string) map[string]interface{} {
	t.Helper()
	current := schema
	for _, field := range fields {
		properties, _ := current["properties"].(map[string]interface{})
		next, _ := properties[field].(map[string]interface{})
		require.NotNil(t, next, "schema property %s should exist", field)
		current = next
	}
	return current
}

func schemaItems(t *testing.T, schema map[string]interface{}) map[string]interface{} {
	t.Helper()
	items, _ := schema["items"].(map[string]interface{})
	require.NotNil(t, items, "schema should define array items")
	return items
}

func requireSchemaMap(t *testing.T, schema map[string]interface{}) {
	t.Helper()
	requireSchemaType(t, schema, "object")
	require.Contains(t, schema, "additionalProperties")
}

func requireSchemaType(t *testing.T, schema map[string]interface{}, want string) {
	t.Helper()
	require.Equal(t, want, schema["type"])
}

func requireSchemaFormat(t *testing.T, schema map[string]interface{}, want string) {
	t.Helper()
	requireSchemaType(t, schema, "string")
	require.Equal(t, want, schema["format"])
}
