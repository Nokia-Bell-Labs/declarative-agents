// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

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
	rec, err := NewRecorderWithConfig(store, meter, RecorderConfig{
		GlobalAttributes: []AttributePolicy{
			{Name: "workflow", AllowedValues: []string{"build"}},
			{Name: "profile", AllowedValues: []string{"monitor"}},
		},
		Envelope: EnvelopePolicy{
			RunID: "run-1", ToolNames: []string{"build"},
			States: []string{"Working"}, Signals: []string{"ToolDone"},
		},
	})
	require.NoError(t, err)

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
	err = rec.RecordMetric(context.Background(), sample)

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
	rec, err := NewRecorderWithConfig(store, nil, RecorderConfig{
		GlobalAttributes: []AttributePolicy{{Name: "workflow", AllowedValues: []string{"build"}}},
		Envelope: EnvelopePolicy{
			RunID: "run-1", ToolNames: []string{"build"},
			States: []string{"Working"}, Signals: []string{"ToolDone"},
		},
	})
	require.NoError(t, err)
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
	require.Len(t, snapshot.RecentSamples, 1)
	require.Equal(t, sample.Name, snapshot.RecentSamples[0].Name)
	require.Equal(t, sample.Signal, snapshot.RecentSamples[0].Signal)
	require.Equal(t, sample.Attributes, snapshot.RecentSamples[0].Attributes)
	require.Len(t, snapshot.Diagnostics, 1)
	require.Equal(t, "record_metric", snapshot.Diagnostics[0].Stage)
	require.Equal(t, sample.Name, snapshot.Diagnostics[0].Metric)
	require.Equal(t, sample.ToolName, snapshot.Diagnostics[0].ToolName)
	require.ErrorContains(t, errors.New(snapshot.Diagnostics[0].Message), exportErr.Error())
}

func TestMonitorRecorderOmitsUndeclaredAndUnboundedAttributesBeforeStoreAndOTel(t *testing.T) {
	t.Parallel()
	store := NewStore(Limits{})
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	rec, err := NewRecorderWithConfig(store, provider.Meter("monitor-test"), RecorderConfig{
		GlobalAttributes: []AttributePolicy{{Name: "workflow", AllowedValues: []string{"build"}}},
		Envelope: EnvelopePolicy{
			RunID: "run-1", ToolNames: []string{"build"},
			States: []string{"Working"}, Signals: []string{"ToolDone"},
		},
	})
	require.NoError(t, err)

	require.NoError(t, rec.RecordMetric(context.Background(), MetricSample{
		Name: "dispatch_count", Kind: InstrumentCounter, Unit: "{dispatch}", Value: 1,
		ToolName: "build", RunID: "run-1", State: "Working",
		Signal: "ToolDone", Status: "success",
		Attributes: map[string]string{
			"workflow": "build", "secret": "token-value", "request_id": "request-123",
		},
	}))

	snapshot := store.Snapshot()
	require.Equal(t, map[string]string{"workflow": "build"}, snapshot.RecentSamples[0].Attributes)
	require.Len(t, snapshot.Diagnostics, 2)
	for _, diagnostic := range snapshot.Diagnostics {
		require.Contains(t, diagnostic.Message, "was omitted")
	}

	var exported metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &exported))
	point := requireExportedMetric(t, exported, "dispatch_count").Data.(metricdata.Sum[float64]).DataPoints[0]
	requireMetricAttribute(t, point.Attributes, "workflow", "build")
	_, hasSecret := point.Attributes.Value(attribute.Key("secret"))
	_, hasRequestID := point.Attributes.Value(attribute.Key("request_id"))
	require.False(t, hasSecret)
	require.False(t, hasRequestID)
}

func TestMonitorRecorderRejectsUntrustedEnvelopeBeforeStoreAndOTel(t *testing.T) {
	t.Parallel()
	store := NewStore(Limits{})
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	rec, err := NewRecorderWithConfig(store, provider.Meter("monitor-test"), RecorderConfig{
		Envelope: EnvelopePolicy{
			RunID: "run-trusted", ToolNames: []string{"build"},
			States: []string{"Working"}, Signals: []string{"ToolDone"},
		},
	})
	require.NoError(t, err)
	valid := MetricSample{
		Name: "dispatch_count", Kind: InstrumentCounter, Unit: "{dispatch}", Value: 1,
		ToolName: "build", RunID: "run-trusted", State: "Working",
		Signal: "ToolDone", Status: "success",
	}
	tests := map[string]func(*MetricSample){
		"tool":      func(sample *MetricSample) { sample.ToolName = "spoofed-tool" },
		"run":       func(sample *MetricSample) { sample.RunID = "spoofed-run" },
		"empty run": func(sample *MetricSample) { sample.RunID = "" },
		"state":     func(sample *MetricSample) { sample.State = "SpoofedState" },
		"signal":    func(sample *MetricSample) { sample.Signal = "SpoofedSignal" },
		"status":    func(sample *MetricSample) { sample.Status = "spoofed-status" },
	}
	for name, spoof := range tests {
		sample := valid
		spoof(&sample)
		require.Error(t, rec.RecordMetric(context.Background(), sample), name)
	}

	snapshot := store.Snapshot()
	require.Empty(t, snapshot.RecentSamples)
	for _, diagnostic := range snapshot.Diagnostics {
		require.NotEqual(t, "spoofed-tool", diagnostic.ToolName)
		for _, spoofed := range []string{
			"spoofed-tool", "spoofed-run", "SpoofedState", "SpoofedSignal", "spoofed-status",
		} {
			require.NotContains(t, diagnostic.Message, spoofed)
		}
	}
	var exported metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &exported))
	require.False(t, hasExportedMetric(exported, valid.Name))
}

func TestMonitorRecorderTrustedEnvelopeScopeIsBoundedAndDoesNotMutateParent(t *testing.T) {
	t.Parallel()
	store := NewStore(Limits{})
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	parent, err := NewRecorderWithConfig(store, provider.Meter("monitor-test"), RecorderConfig{
		Envelope: EnvelopePolicy{
			RunID: "parent-run", ToolNames: []string{"parent-tool"},
			States: []string{"ParentState"}, Signals: []string{"ParentSignal"},
		},
	})
	require.NoError(t, err)
	scoped := parent.WithTrustedEnvelope(EnvelopePolicy{
		RunID: "different-child-run", ToolNames: []string{"request-tool"},
		States: []string{"RequestState"}, Signals: []string{"RequestSignal"},
	})
	requestSample := MetricSample{
		Name: "dispatch_count", Kind: InstrumentCounter, Unit: "{dispatch}", Value: 1,
		ToolName: "request-tool", RunID: "parent-run", State: "RequestState",
		Signal: "RequestSignal", Status: "success",
	}

	require.NoError(t, scoped.RecordMetric(context.Background(), requestSample))
	for name, spoof := range map[string]func(*MetricSample){
		"run":    func(sample *MetricSample) { sample.RunID = "different-child-run" },
		"tool":   func(sample *MetricSample) { sample.ToolName = "spoofed-tool" },
		"state":  func(sample *MetricSample) { sample.State = "SpoofedState" },
		"signal": func(sample *MetricSample) { sample.Signal = "SpoofedSignal" },
	} {
		sample := requestSample
		spoof(&sample)
		require.Error(t, scoped.RecordMetric(context.Background(), sample), name)
	}
	require.Error(t, parent.RecordMetric(context.Background(), requestSample),
		"deriving a scope must not extend the parent allowlists")
	parentSample := requestSample
	parentSample.ToolName = "parent-tool"
	parentSample.State = "ParentState"
	parentSample.Signal = "ParentSignal"
	require.NoError(t, parent.RecordMetric(context.Background(), parentSample))

	snapshot := store.Snapshot()
	require.Len(t, snapshot.RecentSamples, 2)
	require.Equal(t, "request-tool", snapshot.RecentSamples[0].ToolName)
	require.Equal(t, "parent-tool", snapshot.RecentSamples[1].ToolName)
	var exported metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &exported))
	sum := requireExportedMetric(t, exported, requestSample.Name).Data.(metricdata.Sum[float64])
	require.Len(t, sum.DataPoints, 2)
}

func TestMonitorRecorderRejectsConflictingSchemasAtSetup(t *testing.T) {
	t.Parallel()
	_, err := NewRecorderWithConfig(NewStore(Limits{}), nil, RecorderConfig{
		Bindings: []MetricBinding{
			{ToolName: "read", Schema: MetricSchema{Name: "tool.bytes", Kind: InstrumentHistogram, Unit: "By"}},
			{ToolName: "write", Schema: MetricSchema{Name: "tool.bytes", Kind: InstrumentCounter, Unit: "By"}},
		},
	})
	require.ErrorContains(t, err, `metric schema "tool.bytes" conflicts`)
}

func hasExportedMetric(data metricdata.ResourceMetrics, name string) bool {
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				return true
			}
		}
	}
	return false
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
