// Copyright (c) 2026 Nokia. All rights reserved.

package monitor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func TestMonitorOTelExport_NormalizedSamples(t *testing.T) {
	t.Parallel()
	store := NewStore(Limits{})
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("monitor-agent"),
		)),
	)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	meter := provider.Meter("monitor-test")
	rec := NewRecorder(store, meter)

	sample := MetricSample{
		Name:       "dispatch_count",
		Kind:       InstrumentCounter,
		Unit:       "{dispatch}",
		Value:      1,
		ToolName:   "build",
		RunID:      "run-1",
		State:      "Working",
		Signal:     "ToolDone",
		Status:     "success",
		Attributes: map[string]string{"workflow": "build", "profile": "monitor"},
		Timestamp:  time.Unix(10, 0),
	}
	err := rec.RecordMetric(context.Background(), sample)

	require.NoError(t, err)
	snapshot := store.Snapshot()
	require.Equal(t, 1, snapshot.Metrics["dispatch_count"].Count)
	require.Equal(t, "success", snapshot.Tools["build"].LastStatus)
	require.Empty(t, snapshot.Diagnostics)

	var exported metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &exported))
	service, ok := exported.Resource.Set().Value(semconv.ServiceNameKey)
	require.True(t, ok)
	require.Equal(t, "monitor-agent", service.AsString())
	metric := requireExportedMetric(t, exported, sample.Name)
	require.Equal(t, sample.Unit, metric.Unit)
	sum, ok := metric.Data.(metricdata.Sum[float64])
	require.True(t, ok)
	require.Len(t, sum.DataPoints, 1)
	point := sum.DataPoints[0]
	require.Equal(t, sample.Value, point.Value)
	requireMetricAttribute(t, point.Attributes, "tool.name", "build")
	requireMetricAttribute(t, point.Attributes, "run.id", "run-1")
	requireMetricAttribute(t, point.Attributes, "state", "Working")
	requireMetricAttribute(t, point.Attributes, "signal", "ToolDone")
	requireMetricAttribute(t, point.Attributes, "status", "success")
	requireMetricAttribute(t, point.Attributes, "workflow", "build")
	requireMetricAttribute(t, point.Attributes, "profile", "monitor")
}

func TestMonitorOTelExport_FailureRecordsDiagnosticAndPreservesSample(t *testing.T) {
	t.Parallel()
	store := NewStore(Limits{})
	rec := NewRecorder(store, nil)
	exportErr := errors.New("collector unavailable")
	rec.emit = func(context.Context, MetricSample) error { return exportErr }
	sample := MetricSample{
		Name: "dispatch_duration", Kind: InstrumentHistogram, Unit: "ms", Value: 17,
		ToolName: "build", RunID: "run-1", State: "Working", Signal: "ToolDone",
		Status: "success", Attributes: map[string]string{"workflow": "build"},
		Timestamp: time.Unix(20, 0),
	}

	require.NoError(t, rec.RecordMetric(context.Background(), sample),
		"export failure must not alter the originating command path")
	snapshot := store.Snapshot()
	require.Equal(t, []MetricSample{sample}, snapshot.RecentSamples)
	require.Len(t, snapshot.Diagnostics, 1)
	require.Equal(t, "record_metric", snapshot.Diagnostics[0].Stage)
	require.Equal(t, sample.Name, snapshot.Diagnostics[0].Metric)
	require.Equal(t, sample.ToolName, snapshot.Diagnostics[0].ToolName)
	require.ErrorContains(t, errors.New(snapshot.Diagnostics[0].Message), exportErr.Error())
}

func requireExportedMetric(t *testing.T, data metricdata.ResourceMetrics, name string) metricdata.Metrics {
	t.Helper()
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				return metric
			}
		}
	}
	t.Fatalf("metric %q not exported", name)
	return metricdata.Metrics{}
}

func requireMetricAttribute(t *testing.T, attrs attribute.Set, key, value string) {
	t.Helper()
	got, ok := attrs.Value(attribute.Key(key))
	require.Truef(t, ok, "missing metric attribute %q", key)
	require.Equal(t, value, got.AsString())
}
