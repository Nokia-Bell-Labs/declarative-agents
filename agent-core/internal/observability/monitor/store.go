// Copyright (c) 2026 Nokia. All rights reserved.

package monitor

import (
	"sync"
	"time"
)

const defaultLimit = 100

// Store caches current and recent monitor state for one agent process.
type Store struct {
	mu          sync.RWMutex
	limits      Limits
	run         RunSnapshot
	events      []RunEvent
	samples     []MetricSample
	diagnostics []Diagnostic
	errors      []RecentError
	tools       map[string]ToolAggregate
	metrics     map[string]MetricAggregate
	schemas     map[string]MetricSchema
}

// NewStore creates an in-memory monitor store with bounded retention.
func NewStore(limits Limits) *Store {
	return &Store{
		limits:  normalizeLimits(limits),
		tools:   make(map[string]ToolAggregate),
		metrics: make(map[string]MetricAggregate),
		schemas: make(map[string]MetricSchema),
	}
}

// UpdateRun updates the cached current run state.
func (s *Store) UpdateRun(run RunSnapshot) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if run.UpdatedAt.IsZero() {
		run.UpdatedAt = time.Now().UTC()
	}
	s.run = run
}

// RecordEvent adds one bounded runtime event.
func (s *Store) RecordEvent(event RunEvent) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	s.events = appendBoundedEvents(s.events, event, s.limits.Events)
}

// RecordSample adds one accepted metric sample and updates aggregates.
func (s *Store) RecordSample(sample MetricSample) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if sample.Timestamp.IsZero() {
		sample.Timestamp = time.Now().UTC()
	}
	s.samples = appendBoundedSamples(s.samples, cloneSample(sample), s.limits.Samples)
	s.updateToolAggregate(sample)
	s.updateMetricAggregate(sample)
}

// RegisterSchema records a known metric schema for snapshots.
func (s *Store) RegisterSchema(schema MetricSchema) {
	if s == nil || schema.Name == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.schemas[schema.Name] = cloneSchema(schema)
}

// RecordDiagnostic adds a bounded monitor diagnostic.
func (s *Store) RecordDiagnostic(d Diagnostic) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.Timestamp.IsZero() {
		d.Timestamp = time.Now().UTC()
	}
	s.diagnostics = appendBoundedDiagnostics(s.diagnostics, d, s.limits.Diagnostics)
}

// RecordError adds a bounded monitor-visible error summary.
func (s *Store) RecordError(err RecentError) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err.Timestamp.IsZero() {
		err.Timestamp = time.Now().UTC()
	}
	s.errors = appendBoundedErrors(s.errors, err, s.limits.Errors)
}

// Snapshot returns a copy of cached monitor state.
func (s *Store) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{
		Run:           s.run,
		Tools:         cloneToolAggregates(s.tools),
		Metrics:       cloneMetricAggregates(s.metrics),
		Schemas:       cloneSchemas(s.schemas),
		RecentEvents:  append([]RunEvent(nil), s.events...),
		RecentSamples: cloneSamples(s.samples),
		Diagnostics:   append([]Diagnostic(nil), s.diagnostics...),
		RecentErrors:  append([]RecentError(nil), s.errors...),
	}
}

func (s *Store) updateToolAggregate(sample MetricSample) {
	tool := sample.ToolName
	if tool == "" {
		tool = "unknown"
	}
	agg := s.tools[tool]
	agg.ToolName = tool
	agg.Samples++
	agg.LastSignal = sample.Signal
	agg.LastStatus = sample.Status
	agg.UpdatedAt = sample.Timestamp
	updateDispatchCounts(&agg, sample)
	s.tools[tool] = agg
}

func (s *Store) updateMetricAggregate(sample MetricSample) {
	agg := s.metrics[sample.Name]
	agg.Name = sample.Name
	agg.Kind = sample.Kind
	agg.Unit = sample.Unit
	agg.Count++
	agg.Sum += sample.Value
	agg.LastValue = sample.Value
	agg.UpdatedAt = sample.Timestamp
	if agg.Count == 1 || sample.Value < agg.Min {
		agg.Min = sample.Value
	}
	if agg.Count == 1 || sample.Value > agg.Max {
		agg.Max = sample.Value
	}
	s.metrics[sample.Name] = agg
}

func updateDispatchCounts(agg *ToolAggregate, sample MetricSample) {
	switch sample.Name {
	case "dispatch_count":
		agg.Dispatches++
	case "dispatch_success":
		agg.Successes++
	case "dispatch_failure":
		agg.Failures++
	case "dispatch_duration":
		agg.TotalDuration += time.Duration(sample.Value) * time.Millisecond
	}
}

func normalizeLimits(l Limits) Limits {
	if l.Events <= 0 {
		l.Events = defaultLimit
	}
	if l.Samples <= 0 {
		l.Samples = defaultLimit
	}
	if l.Diagnostics <= 0 {
		l.Diagnostics = defaultLimit
	}
	if l.Errors <= 0 {
		l.Errors = defaultLimit
	}
	return l
}

func appendBoundedEvents(items []RunEvent, item RunEvent, limit int) []RunEvent {
	items = append(items, item)
	if len(items) <= limit {
		return items
	}
	return append([]RunEvent(nil), items[len(items)-limit:]...)
}

func appendBoundedSamples(items []MetricSample, item MetricSample, limit int) []MetricSample {
	items = append(items, item)
	if len(items) <= limit {
		return items
	}
	return append([]MetricSample(nil), items[len(items)-limit:]...)
}

func appendBoundedDiagnostics(items []Diagnostic, item Diagnostic, limit int) []Diagnostic {
	items = append(items, item)
	if len(items) <= limit {
		return items
	}
	return append([]Diagnostic(nil), items[len(items)-limit:]...)
}

func appendBoundedErrors(items []RecentError, item RecentError, limit int) []RecentError {
	items = append(items, item)
	if len(items) <= limit {
		return items
	}
	return append([]RecentError(nil), items[len(items)-limit:]...)
}
