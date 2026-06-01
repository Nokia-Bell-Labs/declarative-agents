// Copyright (c) 2026 Nokia. All rights reserved.

package telemetry

import (
	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
)

// TraceAdapter wraps a concrete Trace to satisfy tracing.Tracer.
// Push returns a TraceAdapter so the interface return type is met.
type TraceAdapter struct {
	T Trace
}

// Push creates a child span, wrapping the result in a TraceAdapter.
func (a TraceAdapter) Push(name string, attrs ...attribute.KeyValue) (tracing.Tracer, func()) {
	child, done := a.T.Push(name, attrs...)
	return TraceAdapter{T: child}, done
}

// Event records a span event on the current span.
func (a TraceAdapter) Event(name string, attrs ...attribute.KeyValue) {
	a.T.Event(name, attrs...)
}

// SetAttributes sets attributes on the current span.
func (a TraceAdapter) SetAttributes(attrs ...attribute.KeyValue) {
	a.T.SetAttributes(attrs...)
}

var _ tracing.Tracer = TraceAdapter{}
