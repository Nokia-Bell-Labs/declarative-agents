// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package monitor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	obsmonitor "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

const (
	ViewMachine          = "machine_spec"
	ViewDeclaredMachines = "declared_machines"
	ViewState            = "current_state"
	ViewTools            = "tools"
	ViewMetrics          = "metrics"
	ViewEvents           = "events"
	ViewCommandState     = "command_state"
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

func monitorView(s Surface, route, view string) (interface{}, error) {
	switch view {
	case ViewMachine:
		return monitorMachineView(s.Machine()), nil
	case ViewDeclaredMachines:
		return monitorDeclaredMachinesView(s.DeclaredMachines()), nil
	case ViewState:
		return monitorStateView(s.Snapshot()), nil
	case ViewTools:
		return monitorToolsView(s.Tools()), nil
	case ViewMetrics:
		return monitorMetricsView(s.Snapshot()), nil
	case ViewEvents:
		return monitorEventsView(s.Snapshot()), nil
	default:
		return nil, fmt.Errorf("monitor view %q is not configured for route %q", view, route)
	}
}

// WriteReadState serves a cached monitor view or the generic server state.
func WriteReadState(s Surface, w http.ResponseWriter, name, view string, labels []string) {
	if view == "" {
		s.WriteJSON(w, http.StatusOK, s.StateOutput())
		return
	}
	if view == ViewCommandState {
		s.WriteJSON(w, http.StatusOK, commandStateView(s, labels))
		return
	}
	output, err := monitorView(s, name, view)
	if err != nil {
		writeMonitorError(s, w, name, err)
		return
	}
	s.WriteJSON(w, http.StatusOK, output)
}

// monitorCommandStateView assembles the opt-in command_state response: a map of
// the endpoint's declared labels to entry objects. An absent step renders null,
// a matched step whose output cannot cross the srd038 redaction boundary or
// exceeds the configured response size renders an explicit unavailable entry,
// and a present step carries its redaction-cleared output and run envelope
// (srd033-monitor-rest-api R7.2, R7.3, R7.4, R7.5).
func commandStateView(s Surface, labels []string) map[string]interface{} {
	entries := map[string]interface{}{}
	source := s.CommandState()
	maxBytes := s.MaxResponseBytes()
	for _, label := range labels {
		if source == nil {
			entries[label] = nil
			continue
		}
		entry, found := source.LookupCommandState(label)
		switch {
		case !found:
			entries[label] = nil
		case !entry.Available:
			entries[label] = commandStateUnavailable("output_unavailable")
		case maxBytes > 0 && len(entry.Output) > maxBytes:
			entries[label] = commandStateUnavailable("exceeds_response_limit")
		default:
			entries[label] = commandStateEntryView(entry)
		}
	}
	return map[string]interface{}{"labels": entries}
}

func commandStateEntryView(entry core.MonitorCommandStateEntry) map[string]interface{} {
	return map[string]interface{}{
		"available":  true,
		"output":     rawJSONOrString(entry.Output),
		"state":      entry.State,
		"signal":     entry.Signal,
		"iteration":  entry.Iteration,
		"updated_at": entry.UpdatedAt,
	}
}

func commandStateUnavailable(reason string) map[string]interface{} {
	return map[string]interface{}{"available": false, "reason": reason}
}

// rawJSONOrString embeds valid JSON output verbatim so a reader receives
// structured data, and falls back to the raw string when the output is not JSON.
func rawJSONOrString(output string) interface{} {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil
	}
	if json.Valid([]byte(trimmed)) {
		return json.RawMessage(trimmed)
	}
	return output
}

// WriteStaticMetadata serves OpenAPI or generic server metadata.
func WriteStaticMetadata(s Surface, w http.ResponseWriter, view string) {
	if view == "openapi" {
		s.WriteJSON(w, http.StatusOK, OpenAPI(s))
		return
	}
	s.WriteJSON(w, http.StatusOK, s.MetadataOutput())
}

func monitorMachineView(machine *core.MachineSpec) map[string]interface{} {
	if machine == nil {
		return map[string]interface{}{"machine": nil}
	}
	return monitorObjectView(monitorMachineFields(), machine)
}

func monitorDeclaredMachinesView(machines []core.MachineSpec) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(machines))
	for i := range machines {
		out = append(out, monitorDeclaredMachineView(&machines[i]))
	}
	return out
}

func monitorDeclaredMachineView(machine *core.MachineSpec) map[string]interface{} {
	out := map[string]interface{}{
		"name":            machine.Name,
		"initial_state":   machine.InitialState,
		"states":          monitorDeclaredStates(machine.States),
		"signals":         machine.Signals.Names(),
		"terminal_states": machine.TerminalStates,
		"transitions":     monitorTransitions(machine.Transitions),
	}
	if len(machine.ViewTags) > 0 {
		out["view_tags"] = monitorObjectListView(machine.ViewTags, viewTagFields())
	}
	return out
}

func monitorDeclaredStates(states core.StateSpecs) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(states))
	for _, state := range states {
		item := map[string]interface{}{"name": state.Name}
		if len(state.Tags) > 0 {
			item["tags"] = state.Tags
		}
		out = append(out, item)
	}
	return out
}

func monitorTransitions(transitions []core.TransitionSpec) []map[string]interface{} {
	return monitorObjectListView(transitions, transitionFields())
}

func monitorStateView(snapshot obsmonitor.Snapshot) map[string]interface{} {
	return map[string]interface{}{
		"run":         runSnapshotView(snapshot.Run),
		"diagnostics": diagnosticViews(snapshot.Diagnostics),
		"errors":      recentErrorViews(snapshot.RecentErrors),
	}
}

func monitorToolsView(tools []catalog.ToolDef) map[string]interface{} {
	return map[string]interface{}{"tools": monitorObjectListView(tools, toolFields())}
}

func monitorMetricsView(snapshot obsmonitor.Snapshot) map[string]interface{} {
	return map[string]interface{}{
		"tools":          toolAggregateViews(snapshot.Tools),
		"metrics":        metricAggregateViews(snapshot.Metrics),
		"schemas":        metricSchemaViews(snapshot.Schemas),
		"recent_samples": sampleViews(snapshot.RecentSamples),
		"diagnostics":    diagnosticViews(snapshot.Diagnostics),
	}
}

func monitorEventsView(snapshot obsmonitor.Snapshot) map[string]interface{} {
	return map[string]interface{}{"recent_events": runEventViews(snapshot.RecentEvents)}
}

// StreamEvents writes cached run events and metric samples as SSE.
func StreamEvents(s Surface, w http.ResponseWriter) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	snapshot := s.Snapshot()
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

func runSnapshotView(run obsmonitor.RunSnapshot) map[string]interface{} {
	return monitorObjectView(runSnapshotFields(), run)
}

func sampleViews(samples []obsmonitor.MetricSample) []map[string]interface{} {
	return monitorObjectListView(samples, sampleFields())
}

func runEventViews(events []obsmonitor.RunEvent) []map[string]interface{} {
	return monitorObjectListView(events, runEventFields())
}

func diagnosticViews(items []obsmonitor.Diagnostic) []map[string]interface{} {
	return monitorObjectListView(items, diagnosticFields())
}

func recentErrorViews(items []obsmonitor.RecentError) []map[string]interface{} {
	return monitorObjectListView(items, recentErrorFields())
}

func toolAggregateViews(items map[string]obsmonitor.ToolAggregate) map[string]interface{} {
	return monitorObjectMapView(items, toolAggregateFields())
}

func metricAggregateViews(items map[string]obsmonitor.MetricAggregate) map[string]interface{} {
	return monitorObjectMapView(items, metricAggregateFields())
}

func metricSchemaViews(items map[string]obsmonitor.MetricSchema) map[string]interface{} {
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

func viewTagFields() []monitorField[core.ViewTag] {
	return []monitorField[core.ViewTag]{
		{"tag", monitorSchemaString(), func(tag core.ViewTag) interface{} { return tag.Tag }},
		{"label", monitorSchemaString(), func(tag core.ViewTag) interface{} { return tag.Label }},
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

func runSnapshotFields() []monitorField[obsmonitor.RunSnapshot] {
	return []monitorField[obsmonitor.RunSnapshot]{
		{"run_id", monitorSchemaString(), func(r obsmonitor.RunSnapshot) interface{} { return r.RunID }},
		{"status", monitorSchemaString(), func(r obsmonitor.RunSnapshot) interface{} { return r.Status }},
		{"state", monitorSchemaString(), func(r obsmonitor.RunSnapshot) interface{} { return r.State }},
		{"signal", monitorSchemaString(), func(r obsmonitor.RunSnapshot) interface{} { return r.Signal }},
		{"iteration", monitorSchemaInteger(), func(r obsmonitor.RunSnapshot) interface{} { return r.Iteration }},
		{"updated_at", monitorSchemaDateTime(), func(r obsmonitor.RunSnapshot) interface{} { return r.UpdatedAt }},
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

func sampleFields() []monitorField[obsmonitor.MetricSample] {
	return []monitorField[obsmonitor.MetricSample]{
		{"name", monitorSchemaString(), func(s obsmonitor.MetricSample) interface{} { return s.Name }},
		{"kind", monitorSchemaString(), func(s obsmonitor.MetricSample) interface{} { return s.Kind }},
		{"unit", monitorSchemaString(), func(s obsmonitor.MetricSample) interface{} { return s.Unit }},
		{"description", monitorSchemaString(), func(s obsmonitor.MetricSample) interface{} { return s.Description }},
		{"value", monitorSchemaNumber(), func(s obsmonitor.MetricSample) interface{} { return s.Value }},
		{"tool_name", monitorSchemaString(), func(s obsmonitor.MetricSample) interface{} { return s.ToolName }},
		{"run_id", monitorSchemaString(), func(s obsmonitor.MetricSample) interface{} { return s.RunID }},
		{"state", monitorSchemaString(), func(s obsmonitor.MetricSample) interface{} { return s.State }},
		{"signal", monitorSchemaString(), func(s obsmonitor.MetricSample) interface{} { return s.Signal }},
		{"status", monitorSchemaString(), func(s obsmonitor.MetricSample) interface{} { return s.Status }},
		{"attributes", monitorSchemaMap(monitorSchemaString()), func(s obsmonitor.MetricSample) interface{} { return safeLabels(s.Attributes) }},
		{"timestamp", monitorSchemaDateTime(), func(s obsmonitor.MetricSample) interface{} { return s.Timestamp }},
	}
}

func runEventFields() []monitorField[obsmonitor.RunEvent] {
	return []monitorField[obsmonitor.RunEvent]{
		{"iteration", monitorSchemaInteger(), func(e obsmonitor.RunEvent) interface{} { return e.Iteration }},
		{"timestamp", monitorSchemaDateTime(), func(e obsmonitor.RunEvent) interface{} { return e.Timestamp }},
		{"command_name", monitorSchemaString(), func(e obsmonitor.RunEvent) interface{} { return e.CommandName }},
		{"signal", monitorSchemaString(), func(e obsmonitor.RunEvent) interface{} { return e.Signal }},
		{"from_state", monitorSchemaString(), func(e obsmonitor.RunEvent) interface{} { return e.FromState }},
		{"to_state", monitorSchemaString(), func(e obsmonitor.RunEvent) interface{} { return e.ToState }},
		{"duration_ms", monitorSchemaNumber(), func(e obsmonitor.RunEvent) interface{} { return e.Duration.Milliseconds() }},
		{"tokens_in", monitorSchemaInteger(), func(e obsmonitor.RunEvent) interface{} { return e.TokensIn }},
		{"tokens_out", monitorSchemaInteger(), func(e obsmonitor.RunEvent) interface{} { return e.TokensOut }},
	}
}

func diagnosticFields() []monitorField[obsmonitor.Diagnostic] {
	return []monitorField[obsmonitor.Diagnostic]{
		{"stage", monitorSchemaString(), func(d obsmonitor.Diagnostic) interface{} { return d.Stage }},
		{"message", monitorSchemaString(), func(d obsmonitor.Diagnostic) interface{} { return d.Message }},
		{"metric", monitorSchemaString(), func(d obsmonitor.Diagnostic) interface{} { return d.Metric }},
		{"tool_name", monitorSchemaString(), func(d obsmonitor.Diagnostic) interface{} { return d.ToolName }},
		{"timestamp", monitorSchemaDateTime(), func(d obsmonitor.Diagnostic) interface{} { return d.Timestamp }},
	}
}

func recentErrorFields() []monitorField[obsmonitor.RecentError] {
	return []monitorField[obsmonitor.RecentError]{
		{"stage", monitorSchemaString(), func(e obsmonitor.RecentError) interface{} { return e.Stage }},
		{"message", monitorSchemaString(), func(e obsmonitor.RecentError) interface{} { return e.Message }},
		{"command_name", monitorSchemaString(), func(e obsmonitor.RecentError) interface{} { return e.CommandName }},
		{"timestamp", monitorSchemaDateTime(), func(e obsmonitor.RecentError) interface{} { return e.Timestamp }},
	}
}

func toolAggregateFields() []monitorField[obsmonitor.ToolAggregate] {
	return []monitorField[obsmonitor.ToolAggregate]{
		{"tool_name", monitorSchemaString(), func(a obsmonitor.ToolAggregate) interface{} { return a.ToolName }},
		{"dispatches", monitorSchemaInteger(), func(a obsmonitor.ToolAggregate) interface{} { return a.Dispatches }},
		{"successes", monitorSchemaInteger(), func(a obsmonitor.ToolAggregate) interface{} { return a.Successes }},
		{"failures", monitorSchemaInteger(), func(a obsmonitor.ToolAggregate) interface{} { return a.Failures }},
		{"samples", monitorSchemaInteger(), func(a obsmonitor.ToolAggregate) interface{} { return a.Samples }},
		{"total_duration_ms", monitorSchemaNumber(), func(a obsmonitor.ToolAggregate) interface{} { return a.TotalDuration.Milliseconds() }},
		{"last_signal", monitorSchemaString(), func(a obsmonitor.ToolAggregate) interface{} { return a.LastSignal }},
		{"last_status", monitorSchemaString(), func(a obsmonitor.ToolAggregate) interface{} { return a.LastStatus }},
		{"updated_at", monitorSchemaDateTime(), func(a obsmonitor.ToolAggregate) interface{} { return a.UpdatedAt }},
	}
}

func metricAggregateFields() []monitorField[obsmonitor.MetricAggregate] {
	return []monitorField[obsmonitor.MetricAggregate]{
		{"name", monitorSchemaString(), func(a obsmonitor.MetricAggregate) interface{} { return a.Name }},
		{"kind", monitorSchemaString(), func(a obsmonitor.MetricAggregate) interface{} { return a.Kind }},
		{"unit", monitorSchemaString(), func(a obsmonitor.MetricAggregate) interface{} { return a.Unit }},
		{"count", monitorSchemaInteger(), func(a obsmonitor.MetricAggregate) interface{} { return a.Count }},
		{"sum", monitorSchemaNumber(), func(a obsmonitor.MetricAggregate) interface{} { return a.Sum }},
		{"min", monitorSchemaNumber(), func(a obsmonitor.MetricAggregate) interface{} { return a.Min }},
		{"max", monitorSchemaNumber(), func(a obsmonitor.MetricAggregate) interface{} { return a.Max }},
		{"last_value", monitorSchemaNumber(), func(a obsmonitor.MetricAggregate) interface{} { return a.LastValue }},
		{"updated_at", monitorSchemaDateTime(), func(a obsmonitor.MetricAggregate) interface{} { return a.UpdatedAt }},
	}
}

func metricSchemaFields() []monitorField[obsmonitor.MetricSchema] {
	return []monitorField[obsmonitor.MetricSchema]{
		{"name", monitorSchemaString(), func(s obsmonitor.MetricSchema) interface{} { return s.Name }},
		{"kind", monitorSchemaString(), func(s obsmonitor.MetricSchema) interface{} { return s.Kind }},
		{"unit", monitorSchemaString(), func(s obsmonitor.MetricSchema) interface{} { return s.Unit }},
		{"description", monitorSchemaString(), func(s obsmonitor.MetricSchema) interface{} { return s.Description }},
		{"attributes", monitorSchemaArray(monitorSchemaString()), func(s obsmonitor.MetricSchema) interface{} { return s.Attributes }},
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

func writeMonitorError(s Surface, w http.ResponseWriter, route string, err error) {
	s.WriteJSON(w, http.StatusInternalServerError, map[string]interface{}{
		"endpoint": route, "failure_stage": "monitor_view", "message": err.Error(),
	})
}
