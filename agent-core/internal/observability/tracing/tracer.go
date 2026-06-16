// Copyright (c) 2026 Nokia. All rights reserved.

// Package tracing defines the Tracer port interface (ifc-tracer).
// Consumers import this package for the interface; they never import
// the concrete telemetry package directly.
package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
)

// SpanTracer creates child span scopes.
type SpanTracer interface {
	// Push creates a child span and returns a Tracer scoped to it
	// plus a done function that ends the span.
	Push(name string, attrs ...attribute.KeyValue) (Tracer, func())
}

// EventTracer records point-in-time events.
type EventTracer interface {
	Event(name string, attrs ...attribute.KeyValue)
}

// AttributeTracer records span attributes and errors.
type AttributeTracer interface {
	SetAttributes(attrs ...attribute.KeyValue)
	RecordError(err error)
}

// ContextTracer exposes the context carrying the current span.
type ContextTracer interface {
	Context() context.Context
}

// Tracer is the composed tracing port (ifc-tracer). Keep narrow sub-ports
// above for consumers that only need part of the tracing contract.
type Tracer interface {
	SpanTracer
	EventTracer
	AttributeTracer
	ContextTracer
}
