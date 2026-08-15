// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package tracing

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/attribute"
)

// RecordedEvent is a point-in-time span event captured by RecordingTracer.
type RecordedEvent struct {
	Name  string
	Attrs map[string]interface{}
}

// RecordedSpan is a span captured by RecordingTracer.
type RecordedSpan struct {
	Name      string
	Attrs     map[string]interface{}
	SetAttrs  map[string]interface{}
	HasError  bool
	Completed bool
}

// RecordingTracer captures all tracing calls in memory for test assertions.
// It implements Tracer by recording spans, events, and attributes.
//
// A tracer and every child scope Push returns share one recorder through the
// root pointer, so events and spans emitted through a child are visible on the
// root the test holds. A mutex on the root guards recording, so concurrent
// spans and events are race-safe; read the recorded slices after the recording
// goroutines have joined.
type RecordingTracer struct {
	Events []RecordedEvent
	Spans  []RecordedSpan

	root *RecordingTracer
	mu   sync.Mutex
	cur  int
}

// NewRecordingTracer creates a RecordingTracer with no active span.
func NewRecordingTracer() *RecordingTracer {
	r := &RecordingTracer{cur: -1}
	r.root = r
	return r
}

// base returns the shared root recorder. A tracer built by NewRecordingTracer
// is its own root; every child Push creates points back to that same root.
func (r *RecordingTracer) base() *RecordingTracer {
	if r.root != nil {
		return r.root
	}
	return r
}

func (r *RecordingTracer) Push(name string, attrs ...attribute.KeyValue) (Tracer, func()) {
	base := r.base()
	span := RecordedSpan{Name: name, Attrs: attrsToMap(attrs), SetAttrs: make(map[string]interface{})}

	base.mu.Lock()
	idx := len(base.Spans)
	base.Spans = append(base.Spans, span)
	base.mu.Unlock()

	child := &RecordingTracer{root: base, cur: idx}
	return child, func() {
		base.mu.Lock()
		base.Spans[idx].Completed = true
		base.mu.Unlock()
	}
}

func (r *RecordingTracer) Event(name string, attrs ...attribute.KeyValue) {
	base := r.base()
	event := RecordedEvent{Name: name, Attrs: attrsToMap(attrs)}
	base.mu.Lock()
	base.Events = append(base.Events, event)
	base.mu.Unlock()
}

func (r *RecordingTracer) SetAttributes(attrs ...attribute.KeyValue) {
	base := r.base()
	base.mu.Lock()
	defer base.mu.Unlock()
	if r.cur >= 0 && r.cur < len(base.Spans) {
		for _, a := range attrs {
			base.Spans[r.cur].SetAttrs[string(a.Key)] = AttrValue(a.Value)
		}
	}
}

func (r *RecordingTracer) RecordError(_ error) {
	base := r.base()
	base.mu.Lock()
	defer base.mu.Unlock()
	if r.cur >= 0 && r.cur < len(base.Spans) {
		base.Spans[r.cur].HasError = true
	}
}

func (r *RecordingTracer) Context() context.Context { return context.Background() }

// FindEvent returns the first event with the given name, or nil. Call it after
// recording completes; it reads the shared recorder without locking.
func (r *RecordingTracer) FindEvent(name string) *RecordedEvent {
	base := r.base()
	for i := range base.Events {
		if base.Events[i].Name == name {
			return &base.Events[i]
		}
	}
	return nil
}

func attrsToMap(attrs []attribute.KeyValue) map[string]interface{} {
	m := make(map[string]interface{}, len(attrs))
	for _, a := range attrs {
		m[string(a.Key)] = AttrValue(a.Value)
	}
	return m
}

// AttrValue extracts a Go value from an OTel attribute.Value.
func AttrValue(v attribute.Value) interface{} {
	switch v.Type() {
	case attribute.STRING:
		return v.AsString()
	case attribute.INT64:
		return v.AsInt64()
	case attribute.BOOL:
		return v.AsBool()
	default:
		return v.String()
	}
}

var _ Tracer = (*RecordingTracer)(nil)
