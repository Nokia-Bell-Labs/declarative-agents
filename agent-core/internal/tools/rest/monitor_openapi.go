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
		return monitorReadOperation(name, endpoint)
	case monitorControlEndpoint(endpoint):
		return monitorControlOperation(name, endpoint)
	default:
		return nil
	}
}

func monitorReadOperation(name string, endpoint Endpoint) map[string]interface{} {
	if endpoint.Binding == bindingStreamEvents {
		return monitorStreamOperation(name)
	}
	return map[string]interface{}{
		"operationId": monitorOperationID(name),
		"responses":   monitorResponses("200", "Cached monitor state", monitorResponseSchema(endpoint.MonitorView)),
	}
}

func monitorStreamOperation(name string) map[string]interface{} {
	return map[string]interface{}{
		"operationId": monitorOperationID(name),
		"responses":   monitorStreamResponses(),
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

func monitorStreamResponses() map[string]interface{} {
	return map[string]interface{}{
		"200": map[string]interface{}{
			"description": "Cached monitor event stream",
			"content": map[string]interface{}{
				"text/event-stream": map[string]interface{}{},
			},
		},
	}
}

func monitorRequestBody(schema map[string]interface{}) map[string]interface{} {
	if len(schema) == 0 {
		schema = monitorSchemaObject(nil)
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
		return monitorMachineSchema()
	case monitorViewState:
		return monitorStateSchema()
	case monitorViewTools:
		return monitorToolsSchema()
	case monitorViewMetrics:
		return monitorMetricsSchema()
	case monitorViewEvents:
		return monitorEventsSchema()
	default:
		return monitorSchemaObject(map[string]map[string]interface{}{"data": monitorSchemaObject(nil)})
	}
}

func monitorMachineSchema() map[string]interface{} {
	return monitorSchemaObjectFromFields(monitorMachineFields())
}

func monitorStateSchema() map[string]interface{} {
	return monitorSchemaObject(map[string]map[string]interface{}{
		"run":         runSchema(),
		"diagnostics": monitorSchemaArray(diagnosticSchema()),
		"errors":      monitorSchemaArray(recentErrorSchema()),
	})
}

func monitorToolsSchema() map[string]interface{} {
	return monitorSchemaObject(map[string]map[string]interface{}{
		"tools": monitorSchemaArray(toolSchema()),
	})
}

func monitorMetricsSchema() map[string]interface{} {
	return monitorSchemaObject(map[string]map[string]interface{}{
		"tools":          monitorSchemaMap(toolAggregateSchema()),
		"metrics":        monitorSchemaMap(metricAggregateSchema()),
		"schemas":        monitorSchemaMap(metricSchema()),
		"recent_samples": monitorSchemaArray(sampleSchema()),
		"diagnostics":    monitorSchemaArray(diagnosticSchema()),
	})
}

func monitorEventsSchema() map[string]interface{} {
	return monitorSchemaObject(map[string]map[string]interface{}{
		"recent_events": monitorSchemaArray(runEventSchema()),
	})
}

func transitionSchema() map[string]interface{} {
	return monitorSchemaObjectFromFields(transitionFields())
}

func runSchema() map[string]interface{} {
	return monitorSchemaObjectFromFields(runSnapshotFields())
}

func diagnosticSchema() map[string]interface{} {
	return monitorSchemaObjectFromFields(diagnosticFields())
}

func recentErrorSchema() map[string]interface{} {
	return monitorSchemaObjectFromFields(recentErrorFields())
}

func toolSchema() map[string]interface{} {
	return monitorSchemaObjectFromFields(toolFields())
}

func metricConfigSchema() map[string]interface{} {
	return monitorSchemaObject(map[string]map[string]interface{}{
		"instruments": monitorSchemaArray(metricInstrumentSchema()),
		"attributes":  monitorSchemaArray(metricAttributeSchema()),
		"disabled":    monitorSchemaBoolean(),
	})
}

func metricInstrumentSchema() map[string]interface{} {
	return monitorSchemaObjectFromFields(metricInstrumentFields())
}

func metricAttributeSchema() map[string]interface{} {
	return monitorSchemaObjectFromFields(metricAttributeFields())
}

func relationshipSchema() map[string]interface{} {
	return monitorSchemaObject(map[string]map[string]interface{}{
		"before": monitorSchemaArray(monitorSchemaString()),
		"after":  monitorSchemaArray(monitorSchemaString()),
		"overlaps": monitorSchemaArray(monitorSchemaObject(map[string]map[string]interface{}{
			"tool": monitorSchemaString(), "difference": monitorSchemaString(),
		})),
	})
}

func toolAggregateSchema() map[string]interface{} {
	return monitorSchemaObjectFromFields(toolAggregateFields())
}

func metricAggregateSchema() map[string]interface{} {
	return monitorSchemaObjectFromFields(metricAggregateFields())
}

func metricSchema() map[string]interface{} {
	return monitorSchemaObjectFromFields(metricSchemaFields())
}

func sampleSchema() map[string]interface{} {
	return monitorSchemaObjectFromFields(sampleFields())
}

func runEventSchema() map[string]interface{} {
	return monitorSchemaObjectFromFields(runEventFields())
}

func monitorSchemaObjectFromFields[T any](fields []monitorField[T]) map[string]interface{} {
	properties := map[string]interface{}{}
	for _, field := range fields {
		properties[field.name] = field.schema
	}
	return map[string]interface{}{"type": "object", "properties": properties}
}

func monitorSchemaObject(fields map[string]map[string]interface{}) map[string]interface{} {
	properties := map[string]interface{}{}
	for field, schema := range fields {
		properties[field] = schema
	}
	return map[string]interface{}{"type": "object", "properties": properties}
}

func monitorSchemaArray(item map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"type": "array", "items": item}
}

func monitorSchemaMap(value map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"type": "object", "additionalProperties": value}
}

func monitorSchemaString() map[string]interface{} {
	return map[string]interface{}{"type": "string"}
}

func monitorSchemaDateTime() map[string]interface{} {
	return map[string]interface{}{"type": "string", "format": "date-time"}
}

func monitorSchemaInteger() map[string]interface{} {
	return map[string]interface{}{"type": "integer"}
}

func monitorSchemaNumber() map[string]interface{} {
	return map[string]interface{}{"type": "number"}
}

func monitorSchemaBoolean() map[string]interface{} {
	return map[string]interface{}{"type": "boolean"}
}
