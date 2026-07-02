// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

const (
	monitorViewMachine = "machine_spec"
	monitorViewState   = "current_state"
	monitorViewTools   = "tools"
	monitorViewMetrics = "metrics"
	monitorViewEvents  = "events"
)

type monitorField[T any] struct {
	name   string
	schema map[string]interface{}
	value  func(T) interface{}
}

func monitorObjectView[T any](fields []monitorField[T], item T) map[string]interface{} {
	out := map[string]interface{}{}
	for _, field := range fields {
		out[field.name] = field.value(item)
	}
	return out
}

func monitorObjectListView[T any](items []T, fields []monitorField[T]) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		out = append(out, monitorObjectView(fields, item))
	}
	return out
}

func monitorObjectMapView[T any](items map[string]T, fields []monitorField[T]) map[string]interface{} {
	out := map[string]interface{}{}
	for name, item := range items {
		out[name] = monitorObjectView(fields, item)
	}
	return out
}

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
	return monitorObjectView(monitorMachineFields(), machine)
}

func monitorTransitions(transitions []core.TransitionSpec) []map[string]interface{} {
	return monitorObjectListView(transitions, transitionFields())
}

func monitorStateView(snapshot monitor.Snapshot) map[string]interface{} {
	return map[string]interface{}{
		"run":         runSnapshotView(snapshot.Run),
		"diagnostics": diagnosticViews(snapshot.Diagnostics),
		"errors":      recentErrorViews(snapshot.RecentErrors),
	}
}

func monitorToolsView(tools []catalog.ToolDef) map[string]interface{} {
	return map[string]interface{}{"tools": monitorObjectListView(tools, toolFields())}
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
	return monitorObjectView(runSnapshotFields(), run)
}

func sampleViews(samples []monitor.MetricSample) []map[string]interface{} {
	return monitorObjectListView(samples, sampleFields())
}

func runEventViews(events []monitor.RunEvent) []map[string]interface{} {
	return monitorObjectListView(events, runEventFields())
}

func diagnosticViews(items []monitor.Diagnostic) []map[string]interface{} {
	return monitorObjectListView(items, diagnosticFields())
}

func recentErrorViews(items []monitor.RecentError) []map[string]interface{} {
	return monitorObjectListView(items, recentErrorFields())
}

func toolAggregateViews(items map[string]monitor.ToolAggregate) map[string]interface{} {
	return monitorObjectMapView(items, toolAggregateFields())
}

func metricAggregateViews(items map[string]monitor.MetricAggregate) map[string]interface{} {
	return monitorObjectMapView(items, metricAggregateFields())
}

func metricSchemaViews(items map[string]monitor.MetricSchema) map[string]interface{} {
	return monitorObjectMapView(items, metricSchemaFields())
}

func metricConfigView(cfg core.MetricConfig) map[string]interface{} {
	return map[string]interface{}{
		"instruments": metricInstrumentViews(cfg.Instruments),
		"attributes":  metricAttributeViews(cfg.Attributes),
		"disabled":    cfg.Disabled,
	}
}

func metricInstrumentViews(items []core.MetricInstrument) []map[string]interface{} {
	return monitorObjectListView(items, metricInstrumentFields())
}

func metricAttributeViews(items []core.MetricAttribute) []map[string]interface{} {
	return monitorObjectListView(items, metricAttributeFields())
}

func monitorMachineFields() []monitorField[*core.MachineSpec] {
	return []monitorField[*core.MachineSpec]{
		{"name", monitorSchemaString(), func(m *core.MachineSpec) interface{} { return m.Name }},
		{"states", monitorSchemaArray(monitorSchemaString()), func(m *core.MachineSpec) interface{} { return m.States.Names() }},
		{"signals", monitorSchemaArray(monitorSchemaString()), func(m *core.MachineSpec) interface{} { return m.Signals.Names() }},
		{"terminal_states", monitorSchemaArray(monitorSchemaString()), func(m *core.MachineSpec) interface{} { return m.TerminalStates }},
		{"metric_labels", monitorSchemaMap(monitorSchemaString()), func(m *core.MachineSpec) interface{} { return safeLabels(m.MetricLabels) }},
		{"transitions", monitorSchemaArray(transitionSchema()), func(m *core.MachineSpec) interface{} { return monitorTransitions(m.Transitions) }},
	}
}

func transitionFields() []monitorField[core.TransitionSpec] {
	return []monitorField[core.TransitionSpec]{
		{"state", monitorSchemaString(), func(t core.TransitionSpec) interface{} { return t.State }},
		{"signal", monitorSchemaString(), func(t core.TransitionSpec) interface{} { return t.Signal }},
		{"next", monitorSchemaString(), func(t core.TransitionSpec) interface{} { return t.Next }},
		{"action", monitorSchemaString(), func(t core.TransitionSpec) interface{} { return t.Action }},
		{"metric_labels", monitorSchemaMap(monitorSchemaString()), func(t core.TransitionSpec) interface{} { return safeLabels(t.MetricLabels) }},
	}
}

func runSnapshotFields() []monitorField[monitor.RunSnapshot] {
	return []monitorField[monitor.RunSnapshot]{
		{"run_id", monitorSchemaString(), func(r monitor.RunSnapshot) interface{} { return r.RunID }},
		{"status", monitorSchemaString(), func(r monitor.RunSnapshot) interface{} { return r.Status }},
		{"state", monitorSchemaString(), func(r monitor.RunSnapshot) interface{} { return r.State }},
		{"signal", monitorSchemaString(), func(r monitor.RunSnapshot) interface{} { return r.Signal }},
		{"iteration", monitorSchemaInteger(), func(r monitor.RunSnapshot) interface{} { return r.Iteration }},
		{"updated_at", monitorSchemaDateTime(), func(r monitor.RunSnapshot) interface{} { return r.UpdatedAt }},
	}
}

func toolFields() []monitorField[catalog.ToolDef] {
	return []monitorField[catalog.ToolDef]{
		{"name", monitorSchemaString(), func(t catalog.ToolDef) interface{} { return t.Name }},
		{"category", monitorSchemaString(), func(t catalog.ToolDef) interface{} { return t.Category }},
		{"visibility", monitorSchemaString(), func(t catalog.ToolDef) interface{} { return t.Visibility }},
		{"emits", monitorSchemaArray(monitorSchemaString()), func(t catalog.ToolDef) interface{} { return t.Emits }},
		{"metrics", metricConfigSchema(), func(t catalog.ToolDef) interface{} { return metricConfigView(t.Metrics) }},
		{"relationships", relationshipSchema(), func(t catalog.ToolDef) interface{} { return t.Relationships }},
	}
}

func sampleFields() []monitorField[monitor.MetricSample] {
	return []monitorField[monitor.MetricSample]{
		{"name", monitorSchemaString(), func(s monitor.MetricSample) interface{} { return s.Name }},
		{"kind", monitorSchemaString(), func(s monitor.MetricSample) interface{} { return s.Kind }},
		{"unit", monitorSchemaString(), func(s monitor.MetricSample) interface{} { return s.Unit }},
		{"description", monitorSchemaString(), func(s monitor.MetricSample) interface{} { return s.Description }},
		{"value", monitorSchemaNumber(), func(s monitor.MetricSample) interface{} { return s.Value }},
		{"tool_name", monitorSchemaString(), func(s monitor.MetricSample) interface{} { return s.ToolName }},
		{"run_id", monitorSchemaString(), func(s monitor.MetricSample) interface{} { return s.RunID }},
		{"state", monitorSchemaString(), func(s monitor.MetricSample) interface{} { return s.State }},
		{"signal", monitorSchemaString(), func(s monitor.MetricSample) interface{} { return s.Signal }},
		{"status", monitorSchemaString(), func(s monitor.MetricSample) interface{} { return s.Status }},
		{"attributes", monitorSchemaMap(monitorSchemaString()), func(s monitor.MetricSample) interface{} { return safeLabels(s.Attributes) }},
		{"timestamp", monitorSchemaDateTime(), func(s monitor.MetricSample) interface{} { return s.Timestamp }},
	}
}

func runEventFields() []monitorField[monitor.RunEvent] {
	return []monitorField[monitor.RunEvent]{
		{"iteration", monitorSchemaInteger(), func(e monitor.RunEvent) interface{} { return e.Iteration }},
		{"timestamp", monitorSchemaDateTime(), func(e monitor.RunEvent) interface{} { return e.Timestamp }},
		{"command_name", monitorSchemaString(), func(e monitor.RunEvent) interface{} { return e.CommandName }},
		{"signal", monitorSchemaString(), func(e monitor.RunEvent) interface{} { return e.Signal }},
		{"from_state", monitorSchemaString(), func(e monitor.RunEvent) interface{} { return e.FromState }},
		{"to_state", monitorSchemaString(), func(e monitor.RunEvent) interface{} { return e.ToState }},
		{"duration_ms", monitorSchemaNumber(), func(e monitor.RunEvent) interface{} { return e.Duration.Milliseconds() }},
		{"tokens_in", monitorSchemaInteger(), func(e monitor.RunEvent) interface{} { return e.TokensIn }},
		{"tokens_out", monitorSchemaInteger(), func(e monitor.RunEvent) interface{} { return e.TokensOut }},
	}
}

func diagnosticFields() []monitorField[monitor.Diagnostic] {
	return []monitorField[monitor.Diagnostic]{
		{"stage", monitorSchemaString(), func(d monitor.Diagnostic) interface{} { return d.Stage }},
		{"message", monitorSchemaString(), func(d monitor.Diagnostic) interface{} { return d.Message }},
		{"metric", monitorSchemaString(), func(d monitor.Diagnostic) interface{} { return d.Metric }},
		{"tool_name", monitorSchemaString(), func(d monitor.Diagnostic) interface{} { return d.ToolName }},
		{"timestamp", monitorSchemaDateTime(), func(d monitor.Diagnostic) interface{} { return d.Timestamp }},
	}
}

func recentErrorFields() []monitorField[monitor.RecentError] {
	return []monitorField[monitor.RecentError]{
		{"stage", monitorSchemaString(), func(e monitor.RecentError) interface{} { return e.Stage }},
		{"message", monitorSchemaString(), func(e monitor.RecentError) interface{} { return e.Message }},
		{"command_name", monitorSchemaString(), func(e monitor.RecentError) interface{} { return e.CommandName }},
		{"timestamp", monitorSchemaDateTime(), func(e monitor.RecentError) interface{} { return e.Timestamp }},
	}
}

func toolAggregateFields() []monitorField[monitor.ToolAggregate] {
	return []monitorField[monitor.ToolAggregate]{
		{"tool_name", monitorSchemaString(), func(a monitor.ToolAggregate) interface{} { return a.ToolName }},
		{"dispatches", monitorSchemaInteger(), func(a monitor.ToolAggregate) interface{} { return a.Dispatches }},
		{"successes", monitorSchemaInteger(), func(a monitor.ToolAggregate) interface{} { return a.Successes }},
		{"failures", monitorSchemaInteger(), func(a monitor.ToolAggregate) interface{} { return a.Failures }},
		{"samples", monitorSchemaInteger(), func(a monitor.ToolAggregate) interface{} { return a.Samples }},
		{"total_duration_ms", monitorSchemaNumber(), func(a monitor.ToolAggregate) interface{} { return a.TotalDuration.Milliseconds() }},
		{"last_signal", monitorSchemaString(), func(a monitor.ToolAggregate) interface{} { return a.LastSignal }},
		{"last_status", monitorSchemaString(), func(a monitor.ToolAggregate) interface{} { return a.LastStatus }},
		{"updated_at", monitorSchemaDateTime(), func(a monitor.ToolAggregate) interface{} { return a.UpdatedAt }},
	}
}

func metricAggregateFields() []monitorField[monitor.MetricAggregate] {
	return []monitorField[monitor.MetricAggregate]{
		{"name", monitorSchemaString(), func(a monitor.MetricAggregate) interface{} { return a.Name }},
		{"kind", monitorSchemaString(), func(a monitor.MetricAggregate) interface{} { return a.Kind }},
		{"unit", monitorSchemaString(), func(a monitor.MetricAggregate) interface{} { return a.Unit }},
		{"count", monitorSchemaInteger(), func(a monitor.MetricAggregate) interface{} { return a.Count }},
		{"sum", monitorSchemaNumber(), func(a monitor.MetricAggregate) interface{} { return a.Sum }},
		{"min", monitorSchemaNumber(), func(a monitor.MetricAggregate) interface{} { return a.Min }},
		{"max", monitorSchemaNumber(), func(a monitor.MetricAggregate) interface{} { return a.Max }},
		{"last_value", monitorSchemaNumber(), func(a monitor.MetricAggregate) interface{} { return a.LastValue }},
		{"updated_at", monitorSchemaDateTime(), func(a monitor.MetricAggregate) interface{} { return a.UpdatedAt }},
	}
}

func metricSchemaFields() []monitorField[monitor.MetricSchema] {
	return []monitorField[monitor.MetricSchema]{
		{"name", monitorSchemaString(), func(s monitor.MetricSchema) interface{} { return s.Name }},
		{"kind", monitorSchemaString(), func(s monitor.MetricSchema) interface{} { return s.Kind }},
		{"unit", monitorSchemaString(), func(s monitor.MetricSchema) interface{} { return s.Unit }},
		{"description", monitorSchemaString(), func(s monitor.MetricSchema) interface{} { return s.Description }},
		{"attributes", monitorSchemaArray(monitorSchemaString()), func(s monitor.MetricSchema) interface{} { return s.Attributes }},
	}
}

func metricInstrumentFields() []monitorField[core.MetricInstrument] {
	return []monitorField[core.MetricInstrument]{
		{"name", monitorSchemaString(), func(i core.MetricInstrument) interface{} { return i.Name }},
		{"kind", monitorSchemaString(), func(i core.MetricInstrument) interface{} { return i.Kind }},
		{"unit", monitorSchemaString(), func(i core.MetricInstrument) interface{} { return i.Unit }},
		{"description", monitorSchemaString(), func(i core.MetricInstrument) interface{} { return i.Description }},
		{"value_source", monitorSchemaString(), func(i core.MetricInstrument) interface{} { return i.ValueSource }},
		{"attributes", monitorSchemaArray(monitorSchemaString()), func(i core.MetricInstrument) interface{} { return i.Attributes }},
		{"buckets", monitorSchemaArray(monitorSchemaNumber()), func(i core.MetricInstrument) interface{} { return i.Buckets }},
	}
}

func metricAttributeFields() []monitorField[core.MetricAttribute] {
	return []monitorField[core.MetricAttribute]{
		{"name", monitorSchemaString(), func(a core.MetricAttribute) interface{} { return a.Name }},
		{"source", monitorSchemaString(), func(a core.MetricAttribute) interface{} { return a.Source }},
		{"cardinality", monitorSchemaString(), func(a core.MetricAttribute) interface{} { return a.Cardinality }},
		{"allowed_values", monitorSchemaArray(monitorSchemaString()), func(a core.MetricAttribute) interface{} { return a.AllowedValues }},
		{"redaction", monitorSchemaString(), func(a core.MetricAttribute) interface{} { return a.Redaction }},
	}
}

func safeLabels(labels map[string]string) map[string]string {
	out := map[string]string{}
	for name, value := range labels {
		if safeRedactionLabel(name, value) {
			out[name] = value
		}
	}
	return out
}

func writeMonitorError(w http.ResponseWriter, route string, err error) {
	writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
		"endpoint": route, "failure_stage": "monitor_view", "message": err.Error(),
	})
}
