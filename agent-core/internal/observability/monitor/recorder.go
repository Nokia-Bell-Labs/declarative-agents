// Copyright (c) 2026 Nokia. All rights reserved.

package monitor

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// ToolMetricsRecorder is the tool-facing monitor recorder port.
type ToolMetricsRecorder interface {
	RecordMetric(ctx context.Context, sample MetricSample) error
}

// RuntimeRecorder is the runtime-facing monitor recorder port.
type RuntimeRecorder interface {
	ToolMetricsRecorder
	RecordEvent(ctx context.Context, event RunEvent) error
	RecordRun(ctx context.Context, run RunSnapshot) error
}

// NoopRecorder preserves disabled-mode behavior when monitoring is absent.
type NoopRecorder struct{}

// RecordMetric accepts a metric sample without recording it.
func (NoopRecorder) RecordMetric(context.Context, MetricSample) error { return nil }

// RecordEvent accepts a runtime event without recording it.
func (NoopRecorder) RecordEvent(context.Context, RunEvent) error { return nil }

// RecordRun accepts a run snapshot without recording it.
func (NoopRecorder) RecordRun(context.Context, RunSnapshot) error { return nil }

// Recorder records monitor samples in memory and optionally emits OTel metrics.
type Recorder struct {
	store      *Store
	meter      metric.Meter
	mu         sync.Mutex
	counters   map[string]metric.Float64Counter
	upDown     map[string]metric.Float64UpDownCounter
	histograms map[string]metric.Float64Histogram
	gauges     map[string]metric.Float64Gauge
}

// NewRecorder creates a recorder backed by a store and optional OTel meter.
func NewRecorder(store *Store, meter metric.Meter) *Recorder {
	return &Recorder{
		store:      store,
		meter:      meter,
		counters:   make(map[string]metric.Float64Counter),
		upDown:     make(map[string]metric.Float64UpDownCounter),
		histograms: make(map[string]metric.Float64Histogram),
		gauges:     make(map[string]metric.Float64Gauge),
	}
}

// RecordMetric validates, stores, and exports one normalized metric sample.
func (r *Recorder) RecordMetric(ctx context.Context, sample MetricSample) error {
	if r == nil {
		return nil
	}
	if err := validateSample(sample); err != nil {
		r.recordDiagnostic(sample, err)
		return err
	}
	r.registerSchema(sample)
	if r.store != nil {
		r.store.RecordSample(sample)
	}
	if err := r.emitMetric(ctx, sample); err != nil {
		r.recordDiagnostic(sample, err)
	}
	return nil
}

// RecordEvent records one runtime event in the store.
func (r *Recorder) RecordEvent(_ context.Context, event RunEvent) error {
	if r == nil || r.store == nil {
		return nil
	}
	r.store.RecordEvent(event)
	return nil
}

// RecordRun records the current run state in the store.
func (r *Recorder) RecordRun(_ context.Context, run RunSnapshot) error {
	if r == nil || r.store == nil {
		return nil
	}
	r.store.UpdateRun(run)
	return nil
}

func validateSample(sample MetricSample) error {
	if sample.Name == "" {
		return fmt.Errorf("monitor metric name required")
	}
	switch sample.Kind {
	case InstrumentCounter, InstrumentUpDownCounter, InstrumentHistogram, InstrumentGauge:
		return nil
	default:
		return fmt.Errorf("unsupported monitor instrument kind %q", sample.Kind)
	}
}

func (r *Recorder) registerSchema(sample MetricSample) {
	if r.store == nil {
		return
	}
	r.store.RegisterSchema(MetricSchema{
		Name:        sample.Name,
		Kind:        sample.Kind,
		Unit:        sample.Unit,
		Description: sample.Description,
		Attributes:  attributeNames(sample.Attributes),
	})
}

func (r *Recorder) recordDiagnostic(sample MetricSample, err error) {
	if r.store == nil {
		return
	}
	r.store.RecordDiagnostic(Diagnostic{
		Stage:    "record_metric",
		Message:  err.Error(),
		Metric:   sample.Name,
		ToolName: sample.ToolName,
	})
}

func attributeNames(attrs map[string]string) []string {
	names := make([]string, 0, len(attrs))
	for name := range attrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func metricAttrs(sample MetricSample) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, len(sample.Attributes)+5)
	attrs = append(attrs, attribute.String("tool.name", sample.ToolName))
	attrs = append(attrs, attribute.String("run.id", sample.RunID))
	attrs = append(attrs, attribute.String("state", sample.State))
	attrs = append(attrs, attribute.String("signal", sample.Signal))
	attrs = append(attrs, attribute.String("status", sample.Status))
	for _, name := range attributeNames(sample.Attributes) {
		attrs = append(attrs, attribute.String(name, sample.Attributes[name]))
	}
	return attrs
}
