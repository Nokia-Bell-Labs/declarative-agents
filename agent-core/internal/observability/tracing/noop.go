// Copyright (c) 2026 Nokia. All rights reserved.

package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
)

// NoopTracer is a Tracer that discards all operations. Use it in tests
// and in code paths where tracing is optional.
type NoopTracer struct{}

func (NoopTracer) Push(_ string, _ ...attribute.KeyValue) (Tracer, func()) {
	return NoopTracer{}, func() {}
}

func (NoopTracer) Event(_ string, _ ...attribute.KeyValue) {}

func (NoopTracer) SetAttributes(_ ...attribute.KeyValue) {}

func (NoopTracer) RecordError(_ error) {}

func (NoopTracer) Context() context.Context { return context.Background() }

var _ Tracer = NoopTracer{}
