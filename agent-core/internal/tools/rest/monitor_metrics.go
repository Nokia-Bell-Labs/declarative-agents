// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"io"
	"net/http"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// SetMonitorRecorder connects REST client commands to the embedded monitor recorder.
func (c *clientCmd) SetMonitorRecorder(rec monitor.ToolMetricsRecorder) {
	c.recorder = rec
}

func (c *clientCmd) recordRESTMetrics(request *http.Request, result core.Result) {
	if c.recorder == nil || result.Metrics == nil {
		return
	}
	details := result.Metrics.Details
	attrs := map[string]string{"operation": c.operation.OperationName}
	c.recordRESTMetric(request, "rest.http_status_code", metricInt(details, "status"), "1", attrs)
	c.recordRESTMetric(request, "rest.retry_count", metricInt(details, "retry_count"), "{retry}", attrs)
	c.recordRESTMetric(request, "rest.request_bytes", float64(requestBodyBytes(request)), "By", attrs)
	c.recordRESTMetric(request, "rest.response_bytes", metricInt(details, "response_bytes"), "By", attrs)
}

func (c *clientCmd) recordRESTMetric(req *http.Request, name string, value float64, unit string, attrs map[string]string) {
	sample := monitor.MetricSample{
		Name: name, Kind: restInstrumentKind(name), Unit: unit,
		Description: "REST client metric from configured operation data.",
		Value:       value, ToolName: c.toolName, Attributes: attrs, Timestamp: time.Now(),
	}
	// Monitoring is observational; recorder failures must not change REST behavior.
	_ = c.recorder.RecordMetric(req.Context(), sample)
}

func metricInt(details map[string]any, key string) float64 {
	switch v := details[key].(type) {
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		return 0
	}
}

func requestBodyBytes(req *http.Request) int {
	if req.GetBody == nil {
		return 0
	}
	body, err := req.GetBody()
	if err != nil {
		return 0
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		return 0
	}
	return len(data)
}

func restInstrumentKind(name string) monitor.InstrumentKind {
	switch name {
	case "rest.http_status_code":
		return monitor.InstrumentGauge
	case "rest.retry_count":
		return monitor.InstrumentCounter
	default:
		return monitor.InstrumentHistogram
	}
}
