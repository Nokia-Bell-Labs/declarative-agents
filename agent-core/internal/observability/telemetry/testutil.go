// Copyright (c) 2026 Nokia. All rights reserved.

package telemetry

import (
	"context"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// NewTestTrace creates a Trace backed by an in-memory exporter suitable
// for unit tests. The TracerProvider is shut down via t.Cleanup.
// Returns the Trace and the exporter for span inspection.
func NewTestTrace(t testing.TB, service string) (Trace, *tracetest.InMemoryExporter) {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	tr := NewTraceFromProvider(tp, service, context.Background())
	return tr, exp
}

// NewTestTracer is a convenience that returns a TraceAdapter wrapping
// NewTestTrace, ready to pass to components expecting a tracing.Tracer.
func NewTestTracer(t testing.TB, service string) (TraceAdapter, *tracetest.InMemoryExporter) {
	t.Helper()
	tr, exp := NewTestTrace(t, service)
	return TraceAdapter{T: tr}, exp
}
