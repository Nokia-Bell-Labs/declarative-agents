// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
)

const (
	monitorViewMachine = "machine_spec"
	monitorViewState   = "current_state"
	monitorViewTools   = "tools"
	monitorViewMetrics = "metrics"
	monitorViewEvents  = "events"
)

func (r *serverRuntime) monitorView(route, view string) (map[string]interface{}, error) {
	switch view {
	case monitorViewMachine:
		return monitorMachineView(r.def.Monitor.Machine), nil
	case monitorViewState:
		return monitorStateView(r.monitorSnapshot()), nil
	case monitorViewTools:
		return monitorToolsView(r.def.Monitor.Tools), nil
	case monitorViewMetrics:
		return monitorMetricsView(r.monitorSnapshot()), nil
	case monitorViewEvents:
		return monitorEventsView(r.monitorSnapshot()), nil
	default:
		return nil, fmt.Errorf("monitor view %q is not configured for route %q", view, route)
	}
}

func (r *serverRuntime) writeReadState(w http.ResponseWriter, name string, endpoint Endpoint) {
	if endpoint.MonitorView == "" {
		writeJSON(w, http.StatusOK, r.stateOutput())
		return
	}
	output, err := r.monitorView(name, endpoint.MonitorView)
	if err != nil {
		writeMonitorError(w, name, err)
		return
	}
	writeJSON(w, http.StatusOK, output)
}

func (r *serverRuntime) writeStaticMetadata(w http.ResponseWriter, endpoint Endpoint) {
	if endpoint.MonitorView == "openapi" {
		writeJSON(w, http.StatusOK, r.monitorOpenAPI())
		return
	}
	writeJSON(w, http.StatusOK, r.metadataOutput())
}

func (r *serverRuntime) monitorSnapshot() monitor.Snapshot {
	if r.def.Monitor.Store == nil {
		return monitor.Snapshot{}
	}
	return r.def.Monitor.Store.Snapshot()
}

func monitorMachineView(machine *core.MachineSpec) map[string]interface{} {
	if machine == nil {
		return map[string]interface{}{"machine": nil}
	}
	return map[string]interface{}{
		"name":            machine.Name,
		"states":          machine.States.Names(),
		"signals":         machine.Signals.Names(),
		"terminal_states": machine.TerminalStates,
		"metric_labels":   safeLabels(machine.MetricLabels),
		"transitions":     monitorTransitions(machine.Transitions),
	}
}

func monitorTransitions(transitions []core.TransitionSpec) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(transitions))
	for _, tr := range transitions {
		out = append(out, map[string]interface{}{
			"state": tr.State, "signal": tr.Signal, "next": tr.Next,
			"action": tr.Action, "metric_labels": safeLabels(tr.MetricLabels),
		})
	}
	return out
}

func monitorStateView(snapshot monitor.Snapshot) map[string]interface{} {
	return map[string]interface{}{
		"run":         runSnapshotView(snapshot.Run),
		"diagnostics": diagnosticViews(snapshot.Diagnostics),
		"errors":      recentErrorViews(snapshot.RecentErrors),
	}
}

func monitorToolsView(tools []catalog.ToolDef) map[string]interface{} {
	views := make([]map[string]interface{}, 0, len(tools))
	for _, tool := range tools {
		views = append(views, map[string]interface{}{
			"name": tool.Name, "category": tool.Category,
			"visibility": tool.Visibility, "emits": tool.Emits,
			"metrics": metricConfigView(tool.Metrics), "relationships": tool.Relationships,
		})
	}
	return map[string]interface{}{"tools": views}
}

func monitorMetricsView(snapshot monitor.Snapshot) map[string]interface{} {
	return map[string]interface{}{
		"tools":          toolAggregateViews(snapshot.Tools),
		"metrics":        metricAggregateViews(snapshot.Metrics),
		"schemas":        metricSchemaViews(snapshot.Schemas),
		"recent_samples": sampleViews(snapshot.RecentSamples),
		"diagnostics":    diagnosticViews(snapshot.Diagnostics),
	}
}

func monitorEventsView(snapshot monitor.Snapshot) map[string]interface{} {
	return map[string]interface{}{"recent_events": runEventViews(snapshot.RecentEvents)}
}

func (r *serverRuntime) streamMonitorEvents(w http.ResponseWriter) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	snapshot := r.monitorSnapshot()
	for _, event := range runEventViews(snapshot.RecentEvents) {
		writeMonitorSSE(w, flusher, "run_event", event)
	}
	for _, sample := range sampleViews(snapshot.RecentSamples) {
		writeMonitorSSE(w, flusher, "metric_sample", sample)
	}
}

func writeMonitorSSE(w http.ResponseWriter, flusher http.Flusher, eventType string, value interface{}) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
	flusher.Flush()
}

func runSnapshotView(run monitor.RunSnapshot) map[string]interface{} {
	return map[string]interface{}{
		"run_id": run.RunID, "status": run.Status, "state": run.State,
		"signal": run.Signal, "iteration": run.Iteration, "updated_at": run.UpdatedAt,
	}
}

func sampleViews(samples []monitor.MetricSample) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(samples))
	for _, sample := range samples {
		out = append(out, map[string]interface{}{
			"name": sample.Name, "kind": sample.Kind, "unit": sample.Unit,
			"description": sample.Description, "value": sample.Value,
			"tool_name": sample.ToolName, "run_id": sample.RunID,
			"state": sample.State, "signal": sample.Signal, "status": sample.Status,
			"attributes": safeLabels(sample.Attributes), "timestamp": sample.Timestamp,
		})
	}
	return out
}

func runEventViews(events []monitor.RunEvent) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(events))
	for _, event := range events {
		out = append(out, map[string]interface{}{
			"iteration": event.Iteration, "timestamp": event.Timestamp,
			"command_name": event.CommandName, "signal": event.Signal,
			"from_state": event.FromState, "to_state": event.ToState,
			"duration_ms": event.Duration.Milliseconds(),
			"tokens_in":   event.TokensIn, "tokens_out": event.TokensOut,
		})
	}
	return out
}

func diagnosticViews(items []monitor.Diagnostic) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]interface{}{
			"stage": item.Stage, "message": item.Message, "metric": item.Metric,
			"tool_name": item.ToolName, "timestamp": item.Timestamp,
		})
	}
	return out
}

func recentErrorViews(items []monitor.RecentError) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]interface{}{
			"stage": item.Stage, "message": item.Message,
			"command_name": item.CommandName, "timestamp": item.Timestamp,
		})
	}
	return out
}

func toolAggregateViews(items map[string]monitor.ToolAggregate) map[string]interface{} {
	out := map[string]interface{}{}
	for name, item := range items {
		out[name] = map[string]interface{}{
			"tool_name": item.ToolName, "dispatches": item.Dispatches,
			"successes": item.Successes, "failures": item.Failures,
			"samples": item.Samples, "total_duration_ms": item.TotalDuration.Milliseconds(),
			"last_signal": item.LastSignal, "last_status": item.LastStatus,
			"updated_at": item.UpdatedAt,
		}
	}
	return out
}

func metricAggregateViews(items map[string]monitor.MetricAggregate) map[string]interface{} {
	out := map[string]interface{}{}
	for name, item := range items {
		out[name] = map[string]interface{}{
			"name": item.Name, "kind": item.Kind, "unit": item.Unit,
			"count": item.Count, "sum": item.Sum, "min": item.Min,
			"max": item.Max, "last_value": item.LastValue, "updated_at": item.UpdatedAt,
		}
	}
	return out
}

func metricSchemaViews(items map[string]monitor.MetricSchema) map[string]interface{} {
	out := map[string]interface{}{}
	for name, item := range items {
		out[name] = map[string]interface{}{
			"name": item.Name, "kind": item.Kind, "unit": item.Unit,
			"description": item.Description, "attributes": item.Attributes,
		}
	}
	return out
}

func metricConfigView(cfg core.MetricConfig) map[string]interface{} {
	return map[string]interface{}{
		"instruments": metricInstrumentViews(cfg.Instruments),
		"attributes":  metricAttributeViews(cfg.Attributes),
		"disabled":    cfg.Disabled,
	}
}

func metricInstrumentViews(items []core.MetricInstrument) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]interface{}{
			"name": item.Name, "kind": item.Kind, "unit": item.Unit,
			"description": item.Description, "value_source": item.ValueSource,
			"attributes": item.Attributes, "buckets": item.Buckets,
		})
	}
	return out
}

func metricAttributeViews(items []core.MetricAttribute) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]interface{}{
			"name": item.Name, "source": item.Source, "cardinality": item.Cardinality,
			"allowed_values": item.AllowedValues, "redaction": item.Redaction,
		})
	}
	return out
}

func safeLabels(labels map[string]string) map[string]string {
	out := map[string]string{}
	for name, value := range labels {
		if safeMonitorLabel(name, value) {
			out[name] = value
		}
	}
	return out
}

func safeMonitorLabel(name, value string) bool {
	if name == "" || value == "" {
		return false
	}
	combined := strings.ToLower(name + " " + value)
	for _, bad := range unsafeMonitorTerms {
		if strings.Contains(combined, bad) {
			return false
		}
	}
	return true
}

var unsafeMonitorTerms = []string{
	"prompt", "secret", "token", "authorization", "full_output",
	"request_id", "timestamp", "stack_trace", "command_output", "url",
	"path", "user_text",
}

func writeMonitorError(w http.ResponseWriter, route string, err error) {
	writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
		"endpoint": route, "failure_stage": "monitor_view", "message": err.Error(),
	})
}
