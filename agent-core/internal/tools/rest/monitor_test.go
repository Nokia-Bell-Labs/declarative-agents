// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
)

func TestMonitorREST_ReadOnlyCachedState(t *testing.T) {
	t.Parallel()

	state, baseURL := launchMonitorRESTServer(t, "monitor", seededMonitorState())
	defer stopRESTServer(t, state, "monitor")

	current := getJSON(t, baseURL+"/monitor/state")
	require.Equal(t, "running", current["run"].(map[string]interface{})["status"])
	require.Equal(t, "agent", current["run"].(map[string]interface{})["run_id"])
	requireJSONOmitsGoMonitorFields(t, requestBody(t, http.MethodGet, baseURL+"/monitor/state", "", http.StatusOK))
	require.Len(t, getJSON(t, baseURL+"/monitor/events")["recent_events"], 1)

	requireAwaitSignal(t, state, "monitor", "AwaitTimedOut")
}

func TestMonitorREST_OpenAPIRedaction(t *testing.T) {
	t.Parallel()

	state, baseURL := launchMonitorRESTServer(t, "monitor_openapi", seededMonitorState())
	defer stopRESTServer(t, state, "monitor_openapi")

	body := requestBody(t, http.MethodGet, baseURL+"/monitor/openapi", "", http.StatusOK)
	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &doc))
	require.Equal(t, "3.0.3", doc["openapi"])
	require.NotContains(t, body, "prompt")
	require.NotContains(t, body, "full_output")
	require.NotContains(t, body, "RunID")
	require.NotContains(t, body, "ToolName")
	require.Contains(t, body, "run_id")
	require.Contains(t, body, "tool_name")
	requireMonitorOpenAPIPaths(t, doc)
	control := monitorOpenAPIOperation(t, doc, "/monitor/control/exit", "post")
	require.Equal(t, "monitorControlExit", control["operationId"])
	require.Contains(t, control, "requestBody")
	require.NotContains(t, control, "monitor_view")
	requireMonitorOpenAPISchemaTypes(t, doc, control)
	requireMonitorOpenAPIMatchesRuntimeViews(t, doc, baseURL)
}

func TestMonitorREST_SnapshotEndpoints(t *testing.T) {
	t.Parallel()

	state, baseURL := launchMonitorRESTServer(t, "monitor_snapshot", seededMonitorState())
	defer stopRESTServer(t, state, "monitor_snapshot")

	machine := getJSON(t, baseURL+"/monitor/machine")
	require.Equal(t, "monitor-machine", machine["name"])
	require.Contains(t, machine["metric_labels"], "profile")

	tools := getJSON(t, baseURL+"/monitor/tools")
	require.Len(t, tools["tools"], 1)

	metrics := getJSON(t, baseURL+"/monitor/metrics")
	require.Contains(t, metrics["metrics"], "dispatch_count")
	require.NotContains(t, metrics, "secret")
	body := requestBody(t, http.MethodGet, baseURL+"/monitor/metrics", "", http.StatusOK)
	require.NotContains(t, body, "synthetic-token")
	requireJSONOmitsGoMonitorFields(t, body)
}

func TestMonitorREST_EventStreamCachedUpdates(t *testing.T) {
	t.Parallel()

	state, baseURL := launchMonitorRESTServer(t, "monitor_stream", seededMonitorState())
	defer stopRESTServer(t, state, "monitor_stream")

	body := requestBody(t, http.MethodGet, baseURL+"/monitor/events/stream", "", http.StatusOK)
	require.Contains(t, body, "event: run_event")
	require.Contains(t, body, "event: metric_sample")
	require.NotContains(t, body, "request_id")
	requireJSONOmitsGoMonitorFields(t, body)
	requireQueueEmpty(t, state, "monitor_stream")
}

func TestMonitorREST_FailureDoesNotMutateState(t *testing.T) {
	t.Parallel()

	monitorState := seededMonitorState()
	state, baseURL := launchMonitorRESTServer(t, "monitor_failure", monitorState)
	defer stopRESTServer(t, state, "monitor_failure")

	before := monitorState.Store.Snapshot()
	requestStatus(t, http.MethodGet, baseURL+"/monitor/broken", "", http.StatusInternalServerError)
	after := monitorState.Store.Snapshot()
	require.Equal(t, len(before.RecentEvents), len(after.RecentEvents))
	requireAwaitSignal(t, state, "monitor_failure", "AwaitTimedOut")
}

func TestMonitorREST_FactoryUsesLiveMonitorState(t *testing.T) {
	t.Parallel()

	monitorState, rec := liveMonitorState()
	state, baseURL := launchMonitorRESTServerFromFactory(t, "monitor_live", monitorState)
	defer stopRESTServer(t, state, "monitor_live")

	_ = rec.RecordMetric(context.Background(), monitor.MetricSample{
		Name: "filesystem.bytes_read", Kind: monitor.InstrumentHistogram, Unit: "By",
		Value: 42, ToolName: "file_read", Status: "success",
		Attributes: map[string]string{"profile": "monitor"},
	})

	metrics := getJSON(t, baseURL+"/monitor/metrics")
	require.Contains(t, metrics["metrics"], "filesystem.bytes_read")
	requireMonitorSample(t, metrics["recent_samples"].([]interface{}), "filesystem.bytes_read")

	tools := getJSON(t, baseURL+"/monitor/tools")
	requireToolMetricDeclaration(t, tools["tools"].([]interface{}), "filesystem.bytes_read")

	stream := requestBody(t, http.MethodGet, baseURL+"/monitor/events/stream", "", http.StatusOK)
	require.Contains(t, stream, "event: metric_sample")
	require.Contains(t, stream, "filesystem.bytes_read")
	requireQueueEmpty(t, state, "monitor_live")
}

func launchMonitorRESTServer(
	t *testing.T,
	name string,
	monitorState MonitorState,
) (*ServerState, string) {
	t.Helper()
	state := NewServerState()
	server := monitorServer(name)
	def := ServerDefinition{Name: name, Server: server, Monitor: monitorState}
	_, baseURL := launchRESTServerDefinition(t, state, def)
	return state, baseURL
}

func launchMonitorRESTServerFromFactory(
	t *testing.T,
	name string,
	monitorState MonitorState,
) (*ServerState, string) {
	t.Helper()
	state := NewServerState()
	collection := NewCollection()
	require.NoError(t, collection.Add(Definition{Servers: map[string]Server{name: monitorServer(name)}}))
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{Definitions: collection, ServerState: state, Monitor: monitorState})
	factory, ok := br.Resolve(InitServerLaunch)
	require.True(t, ok)
	builder, err := factory(monitorLaunchTool(name), nil)
	require.NoError(t, err)
	result := builder.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("ServerLaunched"), result.Signal, result.Output)
	return state, "http://" + decodedLaunchAddress(t, result.Output)
}

func monitorLaunchTool(name string) catalog.ToolDef {
	return catalog.ToolDef{
		Name: "launch_monitor_rest", Type: "builtin", Init: InitServerLaunch,
		Description: "Launch monitor REST test server.",
		Config:      map[string]interface{}{"rest_ref": name},
	}
}

func liveMonitorState() (MonitorState, *monitor.Recorder) {
	store := monitor.NewStore(monitor.Limits{Events: 5, Samples: 5})
	rec := monitor.NewRecorder(store, nil)
	_ = rec.RecordRun(context.Background(), monitor.RunSnapshot{
		RunID: "agent", Status: "running", State: "Serving", Iteration: 1,
	})
	_ = rec.RecordEvent(context.Background(), monitor.RunEvent{
		Iteration: 1, CommandName: "launch_monitor_rest", Signal: "ServerLaunched",
		FromState: "Launching", ToState: "Serving",
	})
	return MonitorState{Store: store, Machine: monitorMachineSpec(), Tools: monitorToolDefs()}, rec
}

func seededMonitorState() MonitorState {
	store := monitor.NewStore(monitor.Limits{Events: 5, Samples: 5})
	rec := monitor.NewRecorder(store, nil)
	_ = rec.RecordRun(context.Background(), monitor.RunSnapshot{
		RunID: "agent", Status: "running", State: "Serving", Iteration: 2,
	})
	_ = rec.RecordEvent(context.Background(), monitor.RunEvent{
		Iteration: 2, CommandName: "file_read", Signal: string(core.ToolDone),
		FromState: "Serving", ToState: "Serving",
	})
	_ = rec.RecordMetric(context.Background(), monitor.MetricSample{
		Name: "dispatch_count", Kind: monitor.InstrumentCounter, Unit: "{dispatch}",
		Value: 1, ToolName: "file_read", Status: "success",
		Attributes: map[string]string{"profile": "monitor", "credential": "synthetic-token", "request_id": "unsafe"},
		Timestamp:  time.Now(),
	})
	return MonitorState{Store: store, Machine: monitorMachineSpec(), Tools: monitorToolDefs()}
}

func monitorMachineSpec() *core.MachineSpec {
	return &core.MachineSpec{
		Name: "monitor-machine", InitialState: "Serving",
		States:         core.StateSpecsFromNames("Serving", "Stopped"),
		Signals:        core.SignalSpecsFromNames("Seed", "ServerLaunched"),
		TerminalStates: []string{"Stopped"},
		MetricLabels:   core.MetricLabels{"profile": "monitor", "path": "/tmp/unsafe"},
		Transitions: []core.TransitionSpec{{
			State: "Serving", Signal: "Seed", Next: "Serving", Action: "launch_monitor_rest",
			MetricLabels: core.MetricLabels{"route": "monitor"},
		}},
	}
}

func requireMonitorSample(t *testing.T, samples []interface{}, name string) {
	t.Helper()
	for _, item := range samples {
		sample, _ := item.(map[string]interface{})
		if sample["name"] == name {
			require.Contains(t, sample, "tool_name")
			require.Contains(t, sample, "attributes")
			return
		}
	}
	require.Failf(t, "missing monitor sample", "sample %q not found in %#v", name, samples)
}

func requireToolMetricDeclaration(t *testing.T, tools []interface{}, metric string) {
	t.Helper()
	for _, item := range tools {
		tool, _ := item.(map[string]interface{})
		metrics, _ := tool["metrics"].(map[string]interface{})
		instruments, _ := metrics["instruments"].([]interface{})
		if metricDeclared(instruments, metric) {
			return
		}
	}
	require.Failf(t, "missing tool metric declaration", "metric %q not found in %#v", metric, tools)
}

func metricDeclared(instruments []interface{}, metric string) bool {
	for _, item := range instruments {
		instrument, _ := item.(map[string]interface{})
		if instrument["name"] == metric {
			return true
		}
	}
	return false
}

func requireJSONOmitsGoMonitorFields(t *testing.T, body string) {
	t.Helper()
	for _, field := range []string{"RunID", "ToolName", "UpdatedAt", "CommandName", "FromState", "ToState"} {
		require.NotContains(t, body, `"`+field+`"`)
	}
}

func requireQueueEmpty(t *testing.T, state *ServerState, name string) {
	t.Helper()
	runtime, err := state.runtime(name)
	require.NoError(t, err)
	require.Len(t, runtime.queue, 0)
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	require.Empty(t, runtime.pending)
}

func monitorToolDefs() []catalog.ToolDef {
	return []catalog.ToolDef{{
		Name: "file_read", Category: "filesystem", Visibility: "public",
		Emits: []string{string(core.ToolDone), string(core.CommandError)},
		Metrics: core.MetricConfig{Instruments: []core.MetricInstrument{{
			Name: "filesystem.bytes_read", Kind: "histogram", Unit: "By",
			Description: "Bytes read by filesystem reads.", ValueSource: "bytes_read",
		}}},
	}}
}

func monitorServer(name string) Server {
	return Server{
		Address: "127.0.0.1:0",
		Queue:   QueueConfig{Name: name, Capacity: 8, Timeout: "20ms"},
		Endpoints: map[string]Endpoint{
			"monitor_machine": {Method: "GET", Path: "/monitor/machine", Binding: bindingReadState, MonitorView: monitorViewMachine},
			"monitor_state":   {Method: "GET", Path: "/monitor/state", Binding: bindingReadState, MonitorView: monitorViewState},
			"monitor_tools":   {Method: "GET", Path: "/monitor/tools", Binding: bindingReadState, MonitorView: monitorViewTools},
			"monitor_metrics": {Method: "GET", Path: "/monitor/metrics", Binding: bindingReadState, MonitorView: monitorViewMetrics},
			"monitor_events":  {Method: "GET", Path: "/monitor/events", Binding: bindingReadState, MonitorView: monitorViewEvents},
			"monitor_stream":  {Method: "GET", Path: "/monitor/events/stream", Binding: bindingStreamEvents, MonitorView: monitorViewEvents},
			"monitor_openapi": {Method: "GET", Path: "/monitor/openapi", Binding: bindingStaticMetadata, MonitorView: "openapi"},
			"control_exit": {
				Method: "POST", Path: "/monitor/control/exit",
				Binding: bindingEmitSignal, Signal: "ExitRequested",
				Request: RequestBinding{BodySchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"reason": map[string]interface{}{"type": "string"},
					},
				}},
			},
			"monitor_broken": {Method: "GET", Path: "/monitor/broken", Binding: bindingReadState, MonitorView: "broken"},
		},
	}
}

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
		"/monitor/machine", "/monitor/state", "/monitor/tools", "/monitor/metrics",
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
