// Copyright (c) 2026 Nokia. All rights reserved.

package monitor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric/noop"
)

func TestMonitorOTelExport_NormalizedSamples(t *testing.T) {
	t.Parallel()
	store := NewStore(Limits{})
	meter := noop.NewMeterProvider().Meter("monitor-test")
	rec := NewRecorder(store, meter)

	err := rec.RecordMetric(context.Background(), MetricSample{
		Name:       "dispatch_count",
		Kind:       InstrumentCounter,
		Unit:       "{dispatch}",
		Value:      1,
		ToolName:   "build",
		RunID:      "run-1",
		State:      "Working",
		Signal:     "ToolDone",
		Status:     "success",
		Attributes: map[string]string{"workflow": "build"},
	})

	require.NoError(t, err)
	snapshot := store.Snapshot()
	require.Equal(t, 1, snapshot.Metrics["dispatch_count"].Count)
	require.Equal(t, "success", snapshot.Tools["build"].LastStatus)
	require.Empty(t, snapshot.Diagnostics)
}
