// Copyright (c) 2026 Nokia. All rights reserved.

// Package tracing defines the Tracer port interface (ifc-tracer).
// Consumers import this package for the interface; they never import
// the concrete telemetry package directly.
package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
)

// Tracer is the port interface for tracing (ifc-tracer). It abstracts
// span creation, event recording, and attribute setting. The underlying
// context is accessible via Context() for propagation and cancellation.
type Tracer interface {
	// Push creates a child span and returns a Tracer scoped to it
	// plus a done function that ends the span.
	Push(name string, attrs ...attribute.KeyValue) (Tracer, func())

	// Event records a point-in-time span event.
	Event(name string, attrs ...attribute.KeyValue)

	// SetAttributes sets attributes on the current span.
	SetAttributes(attrs ...attribute.KeyValue)

	// RecordError records err on the current span and sets its status
	// to error with the given message.
	RecordError(err error)

	// Context returns the context carrying the current span.
	Context() context.Context
}
