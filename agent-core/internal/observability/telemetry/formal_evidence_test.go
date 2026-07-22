// Copyright (c) 2026 Nokia. All rights reserved.

package telemetry

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

// These tests back the srd008-telemetry formal evidence in
// docs/specs/test-suites/test-rel00.0.yaml for three behaviors that previously
// cited tests with no implementation: OTLP exporter configuration, the
// TraceAdapter interface wrapper, and ReplayFile's read path.

// TestNewRoot_OTLPExporter asserts an OTLP endpoint is accepted: NewRoot's OTLP
// exporter construction succeeds and the exporters shut down cleanly. It
// exercises otlpExporters directly rather than the full NewRoot so the test
// stays offline — the lazily-connected exporters never dial an endpoint.
func TestNewRoot_OTLPExporter(t *testing.T) {
	t.Parallel()
	traceExp, metricExp, err := otlpExporters("localhost:4317")
	require.NoError(t, err)
	require.NotNil(t, traceExp)
	require.NotNil(t, metricExp)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, traceExp.Shutdown(ctx))
	require.NoError(t, metricExp.Shutdown(ctx))
}

// TestTraceAdapter_SatisfiesInterface asserts TraceAdapter satisfies
// tracing.Tracer and that Push wraps the child span back into a TraceAdapter so
// the interface return type is met.
func TestTraceAdapter_SatisfiesInterface(t *testing.T) {
	t.Parallel()
	tr, shutdown, err := NewRoot("test", "root",
		ExporterConfig{FilePath: filepath.Join(t.TempDir(), "trace.json")}, context.Background())
	require.NoError(t, err)
	defer shutdown()

	var adapter tracing.Tracer = TraceAdapter{T: tr}
	child, done := adapter.Push("child", attribute.String("k", "v"))
	require.NotNil(t, child)
	require.IsType(t, TraceAdapter{}, child)
	defer done()

	// The wrapped operations forward without panicking.
	child.Event("event", attribute.String("e", "1"))
	child.SetAttributes(attribute.String("a", "b"))
	require.NotNil(t, child.Context())
}

// TestReplayFile_InvalidPath asserts ReplayFile surfaces a read error for a
// missing file before any network connection is attempted.
func TestReplayFile_InvalidPath(t *testing.T) {
	t.Parallel()
	err := ReplayFile(filepath.Join(t.TempDir(), "does-not-exist.json"), "localhost:4317")
	require.Error(t, err)
	require.Contains(t, err.Error(), "read")
}
