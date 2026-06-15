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
		"run":         snapshot.Run,
		"diagnostics": snapshot.Diagnostics,
		"errors":      snapshot.RecentErrors,
	}
}

func monitorToolsView(tools []catalog.ToolDef) map[string]interface{} {
	views := make([]map[string]interface{}, 0, len(tools))
	for _, tool := range tools {
		views = append(views, map[string]interface{}{
			"name": tool.Name, "category": tool.Category,
			"visibility": tool.Visibility, "emits": tool.Emits,
			"metrics": tool.Metrics, "relationships": tool.Relationships,
		})
	}
	return map[string]interface{}{"tools": views}
}

func monitorMetricsView(snapshot monitor.Snapshot) map[string]interface{} {
	return map[string]interface{}{
		"tools":          snapshot.Tools,
		"metrics":        snapshot.Metrics,
		"schemas":        snapshot.Schemas,
		"recent_samples": safeSamples(snapshot.RecentSamples),
		"diagnostics":    snapshot.Diagnostics,
	}
}

func monitorEventsView(snapshot monitor.Snapshot) map[string]interface{} {
	return map[string]interface{}{"recent_events": snapshot.RecentEvents}
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
	for _, event := range snapshot.RecentEvents {
		writeMonitorSSE(w, flusher, "run_event", event)
	}
	for _, sample := range safeSamples(snapshot.RecentSamples) {
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

func safeSamples(samples []monitor.MetricSample) []monitor.MetricSample {
	out := make([]monitor.MetricSample, 0, len(samples))
	for _, sample := range samples {
		sample.Attributes = safeLabels(sample.Attributes)
		out = append(out, sample)
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
