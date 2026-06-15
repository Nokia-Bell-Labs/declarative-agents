// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"fmt"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
)

// RecordDeclaredToolMetrics emits specialized tool metrics allowed by cfg.
func RecordDeclaredToolMetrics(
	ctx context.Context,
	rec monitor.ToolMetricsRecorder,
	toolName string,
	cfg MetricConfig,
	values map[string]float64,
	attrs map[string]string,
) {
	if rec == nil || cfg.Disabled {
		return
	}
	declaredAttrs := metricAttrsByName(cfg.Attributes)
	for _, inst := range cfg.Instruments {
		value, ok := values[inst.ValueSource]
		if !ok {
			recordMetricDiagnostic(ctx, rec, toolName, inst.Name, "value source unavailable: "+inst.ValueSource)
			continue
		}
		_ = rec.RecordMetric(ctx, monitor.MetricSample{
			Name:        inst.Name,
			Kind:        monitor.InstrumentKind(inst.Kind),
			Unit:        inst.Unit,
			Description: inst.Description,
			Value:       value,
			ToolName:    toolName,
			Attributes:  metricAttributes(toolName, inst, declaredAttrs, attrs),
			Timestamp:   time.Now(),
		})
	}
}

func metricAttrsByName(attrs []MetricAttribute) map[string]MetricAttribute {
	out := make(map[string]MetricAttribute, len(attrs))
	for _, attr := range attrs {
		out[attr.Name] = attr
	}
	return out
}

func metricAttributes(
	toolName string,
	inst MetricInstrument,
	declared map[string]MetricAttribute,
	sources map[string]string,
) map[string]string {
	out := map[string]string{}
	for _, name := range inst.Attributes {
		attr, ok := declared[name]
		if !ok {
			continue
		}
		value := metricAttributeValue(toolName, attr, sources)
		if metricAttributeAllowed(attr, value) {
			out[attr.Name] = value
		}
	}
	return out
}

func metricAttributeValue(toolName string, attr MetricAttribute, sources map[string]string) string {
	switch attr.Source {
	case "tool_name":
		return toolName
	default:
		if value := sources[attr.Source]; value != "" {
			return value
		}
		return sources[attr.Name]
	}
}

func metricAttributeAllowed(attr MetricAttribute, value string) bool {
	if value == "" || attr.Redaction == "omit" {
		return false
	}
	if len(attr.AllowedValues) == 0 {
		return true
	}
	for _, allowed := range attr.AllowedValues {
		if value == allowed {
			return true
		}
	}
	return false
}

func recordMetricDiagnostic(
	ctx context.Context,
	rec monitor.ToolMetricsRecorder,
	toolName string,
	metric string,
	message string,
) {
	if diag, ok := rec.(monitor.DiagnosticRecorder); ok {
		_ = diag.RecordDiagnostic(ctx, monitor.Diagnostic{
			Stage: "record_metric", Message: message, Metric: metric, ToolName: toolName,
		})
		return
	}
	_ = rec.RecordMetric(ctx, monitor.MetricSample{
		Name: metric, ToolName: toolName, Description: fmt.Sprintf("diagnostic: %s", message),
	})
}
