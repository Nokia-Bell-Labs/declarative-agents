// Copyright (c) 2026 Nokia. All rights reserved.

package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMonitorStore_BoundedSnapshot(t *testing.T) {
	t.Parallel()
	store := NewStore(Limits{Events: 3, Samples: 4, Errors: 2, Diagnostics: 2})

	for i := 1; i <= 8; i++ {
		store.RecordEvent(RunEvent{Iteration: i, CommandName: "tool"})
		store.RecordSample(MetricSample{
			Name:      "dispatch_count",
			Kind:      InstrumentCounter,
			Unit:      "{dispatch}",
			Value:     1,
			ToolName:  "tool",
			Status:    "success",
			Timestamp: time.Unix(int64(i), 0),
		})
		store.RecordError(RecentError{Stage: "test", Message: "err"})
	}

	snapshot := store.Snapshot()
	require.Len(t, snapshot.RecentEvents, 3)
	require.Equal(t, 6, snapshot.RecentEvents[0].Iteration)
	require.Len(t, snapshot.RecentSamples, 4)
	require.Len(t, snapshot.RecentErrors, 2)
	require.False(t, snapshot.ReadsExecuteTools)
	require.False(t, snapshot.ReadsWorkspace)
}

func TestToolMetricsRecorder_OwnershipBoundaries(t *testing.T) {
	t.Parallel()
	store := NewStore(Limits{})
	rec := NewRecorder(store, nil)

	err := rec.RecordMetric(context.Background(), MetricSample{
		Name:       "dispatch_duration",
		Kind:       InstrumentHistogram,
		Unit:       "ms",
		Value:      25,
		ToolName:   "edit",
		State:      "Working",
		Signal:     "ToolDone",
		Status:     "success",
		Attributes: map[string]string{"workflow": "edit"},
	})
	require.NoError(t, err)
	require.NoError(t, rec.RecordMetric(context.Background(), MetricSample{
		Name:     "files_written",
		Kind:     InstrumentCounter,
		Unit:     "{file}",
		Value:    2,
		ToolName: "edit",
		Status:   "success",
	}))

	snapshot := store.Snapshot()
	require.Equal(t, 2, snapshot.Tools["edit"].Samples)
	require.Equal(t, 25*time.Millisecond, snapshot.Tools["edit"].TotalDuration)
	require.Equal(t, 2.0, snapshot.Metrics["files_written"].LastValue)
}

func TestMonitorMetricConfig_RejectsUnsafeLabels(t *testing.T) {
	t.Parallel()
	store := NewStore(Limits{})
	rec := NewRecorder(store, nil)

	err := rec.RecordMetric(context.Background(), MetricSample{
		Kind:     InstrumentCounter,
		ToolName: "bad",
		Value:    1,
	})

	require.Error(t, err)
	snapshot := store.Snapshot()
	require.Empty(t, snapshot.RecentSamples)
	require.Len(t, snapshot.Diagnostics, 1)
	require.Contains(t, snapshot.Diagnostics[0].Message, "name required")
}

func TestMonitorDisabledMode_NoStoreAllocation(t *testing.T) {
	t.Parallel()
	rec := NoopRecorder{}

	require.NoError(t, rec.RecordRun(context.Background(), RunSnapshot{Status: "running"}))
	require.NoError(t, rec.RecordEvent(context.Background(), RunEvent{CommandName: "noop"}))
	require.NoError(t, rec.RecordMetric(context.Background(), MetricSample{Kind: InstrumentCounter}))
}
