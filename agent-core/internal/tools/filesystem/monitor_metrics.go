// Copyright (c) 2026 Nokia. All rights reserved.

package filesystem

import (
	"context"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// SetMonitorRecorder connects read to the embedded monitor recorder.
func (r *readCmd) SetMonitorRecorder(rec monitor.ToolMetricsRecorder) {
	r.recorder = rec
}

// SetMonitorRecorder connects write to the embedded monitor recorder.
func (w *writeCmd) SetMonitorRecorder(rec monitor.ToolMetricsRecorder) {
	w.recorder = rec
}

// SetMonitorRecorder connects edit to the embedded monitor recorder.
func (e *editCmd) SetMonitorRecorder(rec monitor.ToolMetricsRecorder) {
	e.recorder = rec
}

func (r *readCmd) recordFilesystemMetric(source string, value float64) {
	recordFilesystemMetric(context.Background(), r.recorder, r.Name(), r.metrics, source, value)
}

func (w *writeCmd) recordFilesystemMetric(source string, value float64) {
	recordFilesystemMetric(context.Background(), w.recorder, w.Name(), w.metrics, source, value)
}

func (e *editCmd) recordFilesystemMetric(source string, value float64) {
	recordFilesystemMetric(context.Background(), e.recorder, e.Name(), e.metrics, source, value)
}

func recordFilesystemMetric(
	ctx context.Context,
	rec monitor.ToolMetricsRecorder,
	toolName string,
	cfg core.MetricConfig,
	source string,
	value float64,
) {
	core.RecordDeclaredToolMetrics(ctx, rec, toolName, cfg, map[string]float64{source: value}, nil)
}

func bytesChanged(oldString, newString string) int {
	delta := len(newString) - len(oldString)
	if delta < 0 {
		return -delta
	}
	return delta
}
