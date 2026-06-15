// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"io"
	"net/http"

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
	values := map[string]float64{
		"http_status_code": metricInt(details, "status"),
		"retry_count":      metricInt(details, "retry_count"),
		"request_bytes":    float64(requestBodyBytes(request)),
		"response_bytes":   metricInt(details, "response_bytes"),
	}
	core.RecordDeclaredToolMetrics(request.Context(), c.recorder, c.toolName, c.metrics, values, map[string]string{
		"configured_operation": c.operation.OperationName,
	})
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
