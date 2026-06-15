// Copyright (c) 2026 Nokia. All rights reserved.

package filesystem

import (
	"context"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
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

func (r *readCmd) recordFilesystemMetric(name string, value float64, unit string, description string) {
	recordFilesystemMetric(context.Background(), r.recorder, r.Name(), name, value, unit, description)
}

func (w *writeCmd) recordFilesystemMetric(name string, value float64, unit string, description string) {
	recordFilesystemMetric(context.Background(), w.recorder, w.Name(), name, value, unit, description)
}

func (e *editCmd) recordFilesystemMetric(name string, value float64, unit string, description string) {
	recordFilesystemMetric(context.Background(), e.recorder, e.Name(), name, value, unit, description)
}

func recordFilesystemMetric(ctx context.Context, rec monitor.ToolMetricsRecorder, toolName, name string, value float64, unit string, description string) {
	if rec == nil {
		return
	}
	sample := monitor.MetricSample{
		Name: name, Kind: monitor.InstrumentHistogram, Unit: unit,
		Description: description, Value: value, ToolName: toolName,
		Attributes: map[string]string{"operation": toolName}, Timestamp: time.Now(),
	}
	// Monitoring is observational; recorder failures must not change file behavior.
	_ = rec.RecordMetric(ctx, sample)
}

func bytesChanged(oldString, newString string) int {
	delta := len(newString) - len(oldString)
	if delta < 0 {
		return -delta
	}
	return delta
}
