// Copyright (c) 2026 Nokia. All rights reserved.

package monitor

import (
	"context"
	"fmt"
	"maps"
	"slices"
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

// DiagnosticRecorder accepts monitor diagnostics from declaration-driven emitters.
type DiagnosticRecorder interface {
	RecordDiagnostic(ctx context.Context, diagnostic Diagnostic) error
}

// NoopRecorder preserves disabled-mode behavior when monitoring is absent.
type NoopRecorder struct{}

// RecordMetric accepts a metric sample without recording it.
func (NoopRecorder) RecordMetric(context.Context, MetricSample) error { return nil }

// RecordEvent accepts a runtime event without recording it.
func (NoopRecorder) RecordEvent(context.Context, RunEvent) error { return nil }

// RecordRun accepts a run snapshot without recording it.
func (NoopRecorder) RecordRun(context.Context, RunSnapshot) error { return nil }

// RecordDiagnostic accepts a diagnostic without recording it.
func (NoopRecorder) RecordDiagnostic(context.Context, Diagnostic) error { return nil }

// Recorder records monitor samples in memory and optionally emits OTel metrics.
type Recorder struct {
	store      *Store
	meter      metric.Meter
	schemas    *schemaRegistry
	mu         sync.Mutex
	counters   map[string]metric.Float64Counter
	upDown     map[string]metric.Float64UpDownCounter
	histograms map[string]metric.Float64Histogram
	gauges     map[string]metric.Float64Gauge
	emit       func(context.Context, MetricSample) error
}

type schemaRegistry struct {
	metrics map[string]boundMetric
}

type boundMetric struct {
	schema  MetricSchema
	runtime bool
	tools   map[string]map[string]map[string]struct{}
}

// NewRecorder creates a recorder restricted to runtime-owned standard metrics.
// When the store was configured previously, additional recorders inherit that
// same trusted schema registry.
func NewRecorder(store *Store, meter metric.Meter) *Recorder {
	schemas := storeSchemaRegistry(store)
	if schemas == nil {
		schemas, _ = buildSchemaRegistry(RecorderConfig{})
	}
	return newRecorder(store, meter, schemas)
}

// NewRecorderWithConfig creates a recorder bound to setup-validated schemas.
func NewRecorderWithConfig(store *Store, meter metric.Meter, cfg RecorderConfig) (*Recorder, error) {
	schemas, err := buildSchemaRegistry(cfg)
	if err != nil {
		return nil, err
	}
	setStoreSchemaRegistry(store, schemas)
	if store != nil {
		for _, metric := range schemas.metrics {
			store.RegisterSchema(metric.schema)
		}
	}
	return newRecorder(store, meter, schemas), nil
}

func newRecorder(store *Store, meter metric.Meter, schemas *schemaRegistry) *Recorder {
	recorder := &Recorder{
		store:      store,
		meter:      meter,
		schemas:    schemas,
		counters:   make(map[string]metric.Float64Counter),
		upDown:     make(map[string]metric.Float64UpDownCounter),
		histograms: make(map[string]metric.Float64Histogram),
		gauges:     make(map[string]metric.Float64Gauge),
	}
	recorder.emit = recorder.emitMetric
	return recorder
}

// RecordMetric validates, stores, and exports one normalized metric sample.
func (r *Recorder) RecordMetric(ctx context.Context, sample MetricSample) error {
	if r == nil {
		return nil
	}
	normalized, diagnostics, err := r.validateSample(sample)
	for _, diagnostic := range diagnostics {
		r.recordDiagnostic(sample, diagnostic)
	}
	if err != nil {
		r.recordDiagnostic(sample, err)
		return err
	}
	if r.store != nil {
		r.store.RecordSample(normalized)
	}
	if r.emit != nil {
		if err := r.emit(ctx, normalized); err != nil {
			r.recordDiagnostic(normalized, err)
		}
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

// RecordDiagnostic records one monitor diagnostic in the store.
func (r *Recorder) RecordDiagnostic(_ context.Context, diagnostic Diagnostic) error {
	if r == nil || r.store == nil {
		return nil
	}
	r.store.RecordDiagnostic(diagnostic)
	return nil
}

func (r *Recorder) validateSample(sample MetricSample) (MetricSample, []error, error) {
	metric, policy, err := r.sampleSchema(sample)
	if err != nil {
		return MetricSample{}, nil, err
	}
	normalized := sample
	normalized.Description = metric.schema.Description
	attributes, diagnostics := filterMetricAttributes(sample, policy)
	normalized.Attributes = attributes
	return normalized, diagnostics, nil
}

func (r *Recorder) sampleSchema(
	sample MetricSample,
) (boundMetric, map[string]map[string]struct{}, error) {
	if sample.Name == "" {
		return boundMetric{}, nil, fmt.Errorf("monitor metric name required")
	}
	metric, ok := r.schemas.metrics[sample.Name]
	if !ok {
		return boundMetric{}, nil, fmt.Errorf("monitor metric %q is not declared", sample.Name)
	}
	if sample.Kind != metric.schema.Kind {
		return boundMetric{}, nil, fmt.Errorf(
			"monitor metric %q kind %q conflicts with declared kind %q",
			sample.Name, sample.Kind, metric.schema.Kind,
		)
	}
	if sample.Unit != metric.schema.Unit {
		return boundMetric{}, nil, fmt.Errorf(
			"monitor metric %q unit %q conflicts with declared unit %q",
			sample.Name, sample.Unit, metric.schema.Unit,
		)
	}
	policy, ok := metric.tools[sample.ToolName]
	if !ok && metric.runtime {
		policy = metric.tools[""]
		ok = true
	}
	if !ok {
		return boundMetric{}, nil, fmt.Errorf(
			"monitor metric %q is not declared for tool %q", sample.Name, sample.ToolName,
		)
	}
	return metric, policy, nil
}

func filterMetricAttributes(
	sample MetricSample,
	policy map[string]map[string]struct{},
) (map[string]string, []error) {
	normalized := make(map[string]string, len(sample.Attributes))
	var diagnostics []error
	for _, name := range attributeNames(sample.Attributes) {
		allowed, declared := policy[name]
		if !declared {
			diagnostics = append(diagnostics, fmt.Errorf(
				"monitor metric %q attribute %q is not declared and was omitted", sample.Name, name,
			))
			continue
		}
		if _, valid := allowed[sample.Attributes[name]]; !valid {
			diagnostics = append(diagnostics, fmt.Errorf(
				"monitor metric %q attribute %q value is outside its bounded declaration and was omitted",
				sample.Name, name,
			))
			continue
		}
		normalized[name] = sample.Attributes[name]
	}
	return normalized, diagnostics
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

func buildSchemaRegistry(cfg RecorderConfig) (*schemaRegistry, error) {
	registry := &schemaRegistry{metrics: make(map[string]boundMetric)}
	bindings := append(standardMetricBindings(), cfg.Bindings...)
	for _, binding := range bindings {
		policies, err := mergeAttributePolicies(binding.Attributes, cfg.GlobalAttributes)
		if err != nil {
			return nil, fmt.Errorf("metric %q: %w", binding.Schema.Name, err)
		}
		schema := binding.Schema
		schema.Attributes = sortedKeys(policies)
		existing, found := registry.metrics[schema.Name]
		if found {
			if existing.runtime && binding.ToolName != "" {
				return nil, fmt.Errorf("metric schema %q conflicts with runtime-owned metric", schema.Name)
			}
			if existing.schema.Kind != schema.Kind || existing.schema.Unit != schema.Unit ||
				!slices.Equal(existing.schema.Attributes, schema.Attributes) {
				return nil, fmt.Errorf("metric schema %q conflicts in kind, unit, or attribute set", schema.Name)
			}
		} else {
			existing = boundMetric{schema: schema, tools: make(map[string]map[string]map[string]struct{})}
		}
		if _, duplicate := existing.tools[binding.ToolName]; duplicate {
			return nil, fmt.Errorf("metric schema %q is declared more than once for tool %q", schema.Name, binding.ToolName)
		}
		existing.runtime = existing.runtime || binding.ToolName == ""
		existing.tools[binding.ToolName] = policies
		registry.metrics[schema.Name] = existing
	}
	return registry, nil
}

func standardMetricBindings() []MetricBinding {
	return []MetricBinding{
		{Schema: MetricSchema{Name: "dispatch_duration", Kind: InstrumentHistogram, Unit: "ms", Description: "Command dispatch duration."}},
		{Schema: MetricSchema{Name: "dispatch_count", Kind: InstrumentCounter, Unit: "{dispatch}", Description: "Command dispatch attempts."}},
		{Schema: MetricSchema{Name: "dispatch_success", Kind: InstrumentCounter, Unit: "{dispatch}", Description: "Successful command dispatches."}},
		{Schema: MetricSchema{Name: "dispatch_failure", Kind: InstrumentCounter, Unit: "{dispatch}", Description: "Failed command dispatches."}},
	}
}

func mergeAttributePolicies(groups ...[]AttributePolicy) (map[string]map[string]struct{}, error) {
	out := make(map[string]map[string]struct{})
	for _, group := range groups {
		for _, policy := range group {
			if policy.Name == "" || len(policy.AllowedValues) == 0 {
				return nil, fmt.Errorf("attribute %q requires bounded allowed values", policy.Name)
			}
			values := out[policy.Name]
			if values == nil {
				values = make(map[string]struct{}, len(policy.AllowedValues))
				out[policy.Name] = values
			}
			for _, value := range policy.AllowedValues {
				if value == "" {
					return nil, fmt.Errorf("attribute %q has an empty allowed value", policy.Name)
				}
				values[value] = struct{}{}
			}
		}
	}
	return out, nil
}

func sortedKeys[V any](items map[string]V) []string {
	keys := slices.Collect(maps.Keys(items))
	sort.Strings(keys)
	return keys
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
