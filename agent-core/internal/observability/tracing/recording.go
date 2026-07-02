// Copyright (c) 2026 Nokia. All rights reserved.

package tracing

import (
	"context"

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
type RecordingTracer struct {
	Events []RecordedEvent
	Spans  []RecordedSpan
	cur    int
}

// NewRecordingTracer creates a RecordingTracer with no active span.
func NewRecordingTracer() *RecordingTracer { return &RecordingTracer{cur: -1} }

func (r *RecordingTracer) Push(name string, attrs ...attribute.KeyValue) (Tracer, func()) {
	m := make(map[string]interface{})
	for _, a := range attrs {
		m[string(a.Key)] = AttrValue(a.Value)
	}
	idx := len(r.Spans)
	r.Spans = append(r.Spans, RecordedSpan{Name: name, Attrs: m, SetAttrs: make(map[string]interface{})})
	child := &RecordingTracer{Events: r.Events, Spans: r.Spans, cur: idx}
	return child, func() { r.Spans[idx].Completed = true }
}

func (r *RecordingTracer) Event(name string, attrs ...attribute.KeyValue) {
	m := make(map[string]interface{})
	for _, a := range attrs {
		m[string(a.Key)] = AttrValue(a.Value)
	}
	r.Events = append(r.Events, RecordedEvent{Name: name, Attrs: m})
}

func (r *RecordingTracer) SetAttributes(attrs ...attribute.KeyValue) {
	if r.cur >= 0 && r.cur < len(r.Spans) {
		for _, a := range attrs {
			r.Spans[r.cur].SetAttrs[string(a.Key)] = AttrValue(a.Value)
		}
	}
}

func (r *RecordingTracer) RecordError(_ error) {
	if r.cur >= 0 && r.cur < len(r.Spans) {
		r.Spans[r.cur].HasError = true
	}
}

func (r *RecordingTracer) Context() context.Context { return context.Background() }

// FindEvent returns the first event with the given name, or nil.
func (r *RecordingTracer) FindEvent(name string) *RecordedEvent {
	for i := range r.Events {
		if r.Events[i].Name == name {
			return &r.Events[i]
		}
	}
	return nil
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
