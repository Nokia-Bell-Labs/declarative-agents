// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// InitSpoolMetrics identifies the deterministic metric spool factory.
const InitSpoolMetrics = "spool_metrics"

// SpoolMetricsBuilder constructs metric spool commands. It reuses SpoolConfig
// (path, batch_source, rotation) so the metric spool shares the trace spool's
// bounded-rotation discipline (srd042 R9.3).
type SpoolMetricsBuilder struct {
	ToolName string
	Config   SpoolConfig
}

// Build captures the previous result for current-value selectors.
func (b SpoolMetricsBuilder) Build(previous core.Result) core.Command {
	return &spoolMetricsCommand{toolName: b.ToolName, config: b.Config, previous: previous}
}

type spoolMetricsCommand struct {
	toolName string
	config   SpoolConfig
	previous core.Result
	view     core.CommandStateView
}

func (c *spoolMetricsCommand) Name() string { return c.toolName }

func (c *spoolMetricsCommand) SetCommandState(view core.CommandStateView) { c.view = view }

func (c *spoolMetricsCommand) Execute() core.Result {
	request, err := resolveMetricBatch(c.config.BatchSource, c.previous.Output, c.view)
	if err != nil {
		return receiverError(c.Name(), fmt.Errorf("%s: %w", c.Name(), err))
	}
	lines, err := encodeMetricRecords(request)
	if err != nil {
		return receiverError(c.Name(), fmt.Errorf("%s: %w", c.Name(), err))
	}
	written, err := appendSpool(c.config, lines)
	if err != nil {
		return receiverError(c.Name(), fmt.Errorf("%s: %w", c.Name(), err))
	}
	payload, err := protojson.Marshal(request)
	if err != nil {
		return receiverError(c.Name(), err)
	}
	output := struct {
		Path           string          `json:"path"`
		MetricCount    int             `json:"metric_count"`
		DataPointCount int             `json:"data_point_count"`
		Bytes          int             `json:"bytes_written"`
		Batch          json.RawMessage `json:"batch"`
	}{
		Path: c.config.Path, MetricCount: requestMetricCount(request),
		DataPointCount: requestDataPointCount(request), Bytes: written, Batch: payload,
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return receiverError(c.Name(), err)
	}
	return core.Result{Signal: core.Signal("MetricsSpooled"), CommandName: c.Name(), Output: string(encoded)}
}

func (c *spoolMetricsCommand) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.Name())
}

var _ core.CommandStateAware = (*spoolMetricsCommand)(nil)

func resolveMetricBatch(
	selector string,
	previous string,
	view core.CommandStateView,
) (*colmetricpb.ExportMetricsServiceRequest, error) {
	payload, err := resolveSelectorPayload(selector, previous, view)
	if err != nil {
		return nil, err
	}
	var request colmetricpb.ExportMetricsServiceRequest
	if err := protojson.Unmarshal(payload, &request); err != nil {
		return nil, fmt.Errorf("decode selected OTLP metric batch: %w", err)
	}
	return &request, nil
}

// metricRecord is one spooled NDJSON metric line. It flattens the identity a
// query reads (name, type, resource attributes) and embeds the full metric as
// protojson so no fidelity is lost.
type metricRecord struct {
	Name           string           `json:"Name"`
	Description    string           `json:"Description"`
	Unit           string           `json:"Unit"`
	Type           string           `json:"Type"`
	DataPointCount int              `json:"DataPointCount"`
	Resource       []map[string]any `json:"Resource"`
	Scope          map[string]any   `json:"Scope"`
	Metric         json.RawMessage  `json:"Metric"`
}

func encodeMetricRecords(request *colmetricpb.ExportMetricsServiceRequest) ([]byte, error) {
	var output strings.Builder
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	for _, resourceMetrics := range request.GetResourceMetrics() {
		resource := stdoutAttributes(resourceMetrics.GetResource().GetAttributes())
		for _, scopeMetrics := range resourceMetrics.GetScopeMetrics() {
			scope := map[string]any{
				"Name":       scopeMetrics.GetScope().GetName(),
				"Version":    scopeMetrics.GetScope().GetVersion(),
				"SchemaURL":  scopeMetrics.GetSchemaUrl(),
				"Attributes": stdoutAttributes(scopeMetrics.GetScope().GetAttributes()),
			}
			for _, metric := range scopeMetrics.GetMetrics() {
				encodedMetric, err := protojson.Marshal(metric)
				if err != nil {
					return nil, fmt.Errorf("encode metric %q: %w", metric.GetName(), err)
				}
				record := metricRecord{
					Name: metric.GetName(), Description: metric.GetDescription(),
					Unit: metric.GetUnit(), Type: metricType(metric),
					DataPointCount: metricDataPointCount(metric),
					Resource:       resource, Scope: scope, Metric: encodedMetric,
				}
				if err := encoder.Encode(record); err != nil {
					return nil, fmt.Errorf("encode metric record %q: %w", metric.GetName(), err)
				}
			}
		}
	}
	return []byte(output.String()), nil
}

func requestMetricCount(request *colmetricpb.ExportMetricsServiceRequest) int {
	count := 0
	for _, resourceMetrics := range request.GetResourceMetrics() {
		for _, scopeMetrics := range resourceMetrics.GetScopeMetrics() {
			count += len(scopeMetrics.GetMetrics())
		}
	}
	return count
}

func metricType(metric *metricpb.Metric) string {
	switch {
	case metric.GetGauge() != nil:
		return "Gauge"
	case metric.GetSum() != nil:
		return "Sum"
	case metric.GetHistogram() != nil:
		return "Histogram"
	case metric.GetExponentialHistogram() != nil:
		return "ExponentialHistogram"
	case metric.GetSummary() != nil:
		return "Summary"
	default:
		return "Unknown"
	}
}
