// Copyright (c) 2026 Nokia. All rights reserved.

package monitor

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

func (r *Recorder) emitMetric(ctx context.Context, sample MetricSample) error {
	if r.meter == nil {
		return nil
	}
	switch sample.Kind {
	case InstrumentCounter:
		return r.emitCounter(ctx, sample)
	case InstrumentUpDownCounter:
		return r.emitUpDownCounter(ctx, sample)
	case InstrumentHistogram:
		return r.emitHistogram(ctx, sample)
	case InstrumentGauge:
		return r.emitGauge(ctx, sample)
	default:
		return fmt.Errorf("unsupported monitor instrument kind %q", sample.Kind)
	}
}

func (r *Recorder) emitCounter(ctx context.Context, sample MetricSample) error {
	inst, err := r.counter(sample)
	if err != nil {
		return err
	}
	inst.Add(ctx, sample.Value, metric.WithAttributes(metricAttrs(sample)...))
	return nil
}

func (r *Recorder) emitUpDownCounter(ctx context.Context, sample MetricSample) error {
	inst, err := r.upDownCounter(sample)
	if err != nil {
		return err
	}
	inst.Add(ctx, sample.Value, metric.WithAttributes(metricAttrs(sample)...))
	return nil
}

func (r *Recorder) emitHistogram(ctx context.Context, sample MetricSample) error {
	inst, err := r.histogram(sample)
	if err != nil {
		return err
	}
	inst.Record(ctx, sample.Value, metric.WithAttributes(metricAttrs(sample)...))
	return nil
}

func (r *Recorder) emitGauge(ctx context.Context, sample MetricSample) error {
	inst, err := r.gauge(sample)
	if err != nil {
		return err
	}
	inst.Record(ctx, sample.Value, metric.WithAttributes(metricAttrs(sample)...))
	return nil
}

func (r *Recorder) counter(sample MetricSample) (metric.Float64Counter, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if inst, ok := r.counters[sample.Name]; ok {
		return inst, nil
	}
	inst, err := r.meter.Float64Counter(sample.Name)
	if err != nil {
		return nil, err
	}
	r.counters[sample.Name] = inst
	return inst, nil
}

func (r *Recorder) upDownCounter(sample MetricSample) (metric.Float64UpDownCounter, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if inst, ok := r.upDown[sample.Name]; ok {
		return inst, nil
	}
	inst, err := r.meter.Float64UpDownCounter(sample.Name)
	if err != nil {
		return nil, err
	}
	r.upDown[sample.Name] = inst
	return inst, nil
}

func (r *Recorder) histogram(sample MetricSample) (metric.Float64Histogram, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if inst, ok := r.histograms[sample.Name]; ok {
		return inst, nil
	}
	inst, err := r.meter.Float64Histogram(sample.Name)
	if err != nil {
		return nil, err
	}
	r.histograms[sample.Name] = inst
	return inst, nil
}

func (r *Recorder) gauge(sample MetricSample) (metric.Float64Gauge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if inst, ok := r.gauges[sample.Name]; ok {
		return inst, nil
	}
	inst, err := r.meter.Float64Gauge(sample.Name)
	if err != nil {
		return nil, err
	}
	r.gauges[sample.Name] = inst
	return inst, nil
}
