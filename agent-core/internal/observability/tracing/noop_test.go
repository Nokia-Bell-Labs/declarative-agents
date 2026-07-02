// Copyright (c) 2026 Nokia. All rights reserved.

package tracing

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

func TestNoopTracerSatisfiesInterfaceAndDiscardsOperations(t *testing.T) {
	t.Parallel()
	var tr Tracer = NoopTracer{}

	child, done := tr.Push("child", attribute.String("key", "value"))
	child.Event("event", attribute.Int("count", 1))
	child.SetAttributes(attribute.Bool("ok", true))
	child.RecordError(errors.New("ignored"))
	done()

	require.IsType(t, NoopTracer{}, child)
	require.Equal(t, context.Background(), tr.Context())
}
