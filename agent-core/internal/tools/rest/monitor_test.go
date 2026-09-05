// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	"github.com/stretchr/testify/require"
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

func TestMonitorREST_DeclaredMachines(t *testing.T) {
	t.Parallel()

	monitorState := seededMonitorState()
	root := *monitorState.Machine
	child := core.MachineSpec{
		Name:         "chatbot-turn",
		InitialState: "AwaitingRequest",
		ViewTags: []core.ViewTag{
			{Tag: "intake", Label: "Intake"},
			{Tag: "answer", Label: "Answer composition"},
		},
		States: core.StateSpecs{
			{Name: "AwaitingRequest", Tags: []string{"intake"}},
			{Name: "Done", RunStatus: core.StatusSucceeded, Tags: []string{"answer"}},
		},
		Signals:        core.SignalSpecsFromNames("Seed"),
		TerminalStates: []string{"Done"},
		Transitions: []core.TransitionSpec{{
			State: "AwaitingRequest", Signal: "Seed", Next: "Done",
		}},
	}
	monitorState.DeclaredMachines = []core.MachineSpec{root, child}
	beforeStore := monitorState.Store.Snapshot()
	beforeMachines, err := json.Marshal(monitorState.DeclaredMachines)
	require.NoError(t, err)

	state, baseURL := launchMonitorRESTServer(t, "monitor_declared", monitorState)
	defer stopRESTServer(t, state, "monitor_declared")

	body := requestBody(t, http.MethodGet, baseURL+"/monitor/machines", "", http.StatusOK)
	var machines []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &machines))
	require.Len(t, machines, 2)
	require.Equal(t, "monitor-machine", machines[0]["name"])
	require.Equal(t, "chatbot-turn", machines[1]["name"])
	require.Equal(t, "AwaitingRequest", machines[1]["initial_state"])
	require.NotContains(t, machines[1], "run")
	require.NotContains(t, machines[1], "current_state")
	viewTags := machines[1]["view_tags"].([]interface{})
	require.Equal(t, "intake", viewTags[0].(map[string]interface{})["tag"])
	states := machines[1]["states"].([]interface{})
	require.Equal(t, []interface{}{"intake"}, states[0].(map[string]interface{})["tags"])

	rootView := getJSON(t, baseURL+"/monitor/machine")
	require.Equal(t, "monitor-machine", rootView["name"])
	require.Equal(t, []interface{}{"Serving", "Stopped"}, rootView["states"])
	require.NotContains(t, rootView, "initial_state")
	require.NotContains(t, rootView, "view_tags")

	require.Equal(t, beforeStore, monitorState.Store.Snapshot())
	afterMachines, err := json.Marshal(monitorState.DeclaredMachines)
	require.NoError(t, err)
	require.JSONEq(t, string(beforeMachines), string(afterMachines))
	requireQueueEmpty(t, state, "monitor_declared")
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
	beforeMachine, err := json.Marshal(monitorState.Machine)
	require.NoError(t, err)
	beforeTools, err := json.Marshal(monitorState.Tools)
	require.NoError(t, err)

	body := requestBody(t, http.MethodGet, baseURL+"/monitor/broken", "", http.StatusInternalServerError)
	var failure map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &failure))
	require.Len(t, failure, 3)
	require.Equal(t, "monitor_broken", failure["endpoint"])
	require.Equal(t, "monitor_view", failure["failure_stage"])
	require.Contains(t, failure["message"], `monitor view "broken"`)
	require.NotContains(t, body, "synthetic-token")
	require.NotContains(t, body, "/tmp/unsafe")
	require.NotContains(t, body, "request_id")

	after := monitorState.Store.Snapshot()
	require.Equal(t, before, after)
	afterMachine, err := json.Marshal(monitorState.Machine)
	require.NoError(t, err)
	require.JSONEq(t, string(beforeMachine), string(afterMachine))
	afterTools, err := json.Marshal(monitorState.Tools)
	require.NoError(t, err)
	require.JSONEq(t, string(beforeTools), string(afterTools))
	requireQueueEmpty(t, state, "monitor_failure")
	requireAwaitSignal(t, state, "monitor_failure", "AwaitTimedOut")
}

func TestMonitorREST_FactoryUsesLiveMonitorState(t *testing.T) {
	t.Parallel()

	monitorState, rec := liveMonitorState(t)
	state, baseURL := launchMonitorRESTServerFromFactory(t, "monitor_live", monitorState)
	defer stopRESTServer(t, state, "monitor_live")

	_ = rec.RecordMetric(context.Background(), monitor.MetricSample{
		Name: "filesystem.bytes_read", Kind: monitor.InstrumentHistogram, Unit: "By",
		Value: 42, ToolName: "file_read", RunID: "agent", State: "Serving",
		Signal: string(core.ToolDone), Status: "success",
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

func liveMonitorState(t *testing.T) (MonitorState, *monitor.Recorder) {
	t.Helper()
	store := monitor.NewStore(monitor.Limits{Events: 5, Samples: 5})
	rec, err := monitor.NewRecorderWithConfig(store, nil, monitor.RecorderConfig{
		GlobalAttributes: []monitor.AttributePolicy{{Name: "profile", AllowedValues: []string{"monitor"}}},
		Envelope: monitor.EnvelopePolicy{
			RunID: "agent", ToolNames: []string{"file_read"},
			States: []string{"Serving"}, Signals: []string{string(core.ToolDone)},
		},
		Bindings: []monitor.MetricBinding{{
			ToolName: "file_read",
			Schema: monitor.MetricSchema{
				Name: "filesystem.bytes_read", Kind: monitor.InstrumentHistogram, Unit: "By",
			},
		}},
	})
	require.NoError(t, err)
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
		Value: 1, ToolName: "file_read", RunID: "agent", State: "Serving",
		Signal: string(core.ToolDone), Status: "success",
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

func monitorServer(name string) restdef.Server {
	return restdef.Server{
		Address: "127.0.0.1:0",
		Queue:   restdef.QueueConfig{Name: name, Capacity: 8, Timeout: "20ms"},
		Endpoints: map[string]restdef.Endpoint{
			"monitor_machine":  {Method: "GET", Path: "/monitor/machine", Binding: bindingReadState, MonitorView: monitorViewMachine},
			"monitor_machines": {Method: "GET", Path: "/monitor/machines", Binding: bindingReadState, MonitorView: monitorViewDeclaredMachines},
			"monitor_state":    {Method: "GET", Path: "/monitor/state", Binding: bindingReadState, MonitorView: monitorViewState},
			"monitor_tools":    {Method: "GET", Path: "/monitor/tools", Binding: bindingReadState, MonitorView: monitorViewTools},
			"monitor_metrics":  {Method: "GET", Path: "/monitor/metrics", Binding: bindingReadState, MonitorView: monitorViewMetrics},
			"monitor_events":   {Method: "GET", Path: "/monitor/events", Binding: bindingReadState, MonitorView: monitorViewEvents},
			"monitor_stream":   {Method: "GET", Path: "/monitor/events/stream", Binding: bindingStreamEvents, MonitorView: monitorViewEvents},
			"monitor_openapi":  {Method: "GET", Path: "/monitor/openapi", Binding: bindingStaticMetadata, MonitorView: "openapi"},
			"control_exit": {
				Method: "POST", Path: "/monitor/control/exit",
				Binding: bindingEmitSignal, Signal: "ExitRequested",
				Request: restdef.RequestBinding{BodySchema: map[string]interface{}{
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
