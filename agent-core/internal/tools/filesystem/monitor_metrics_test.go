// Copyright (c) 2026 Nokia. All rights reserved.

package filesystem

import (
	"context"
	"testing"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

type filesystemMetricRecorder struct {
	samples []monitor.MetricSample
}

func (r *filesystemMetricRecorder) RecordMetric(_ context.Context, sample monitor.MetricSample) error {
	r.samples = append(r.samples, sample)
	return nil
}

func TestFilesystemCommandsRecordMonitorMetrics(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	rec := &filesystemMetricRecorder{}

	write := (&WriteBuilder{Root: root, Metrics: filesystemMetrics("filesystem.bytes_written", "bytes_written")}).
		Build(toolReq(`{"path":"a.txt","content":"hello"}`))
	write.(core.MonitorRecorderAware).SetMonitorRecorder(rec)
	if res := write.Execute(); res.Signal != core.ToolDone {
		t.Fatalf("write signal = %s", res.Signal)
	}

	read := (&ReadBuilder{Root: root, Metrics: filesystemMetrics("filesystem.bytes_read", "bytes_read")}).
		Build(toolReq(`{"path":"a.txt"}`))
	read.(core.MonitorRecorderAware).SetMonitorRecorder(rec)
	if res := read.Execute(); res.Signal != core.ToolDone {
		t.Fatalf("read signal = %s", res.Signal)
	}

	edit := (&EditBuilder{Root: root, Metrics: filesystemMetrics("filesystem.bytes_changed", "bytes_changed")}).
		Build(toolReq(`{"path":"a.txt","old_string":"hello","new_string":"hello!"}`))
	edit.(core.MonitorRecorderAware).SetMonitorRecorder(rec)
	if res := edit.Execute(); res.Signal != core.EditDone {
		t.Fatalf("edit signal = %s", res.Signal)
	}

	requireFilesystemMetric(t, rec.samples, "filesystem.bytes_written", 5)
	requireFilesystemMetric(t, rec.samples, "filesystem.bytes_read", 5)
	requireFilesystemMetric(t, rec.samples, "filesystem.bytes_changed", 1)
	for _, sample := range rec.samples {
		if _, ok := sample.Attributes["path"]; ok {
			t.Fatalf("path leaked in metric attrs: %#v", sample.Attributes)
		}
	}
}

func TestFilesystemMetricsRespectDisabledConfig(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	rec := &filesystemMetricRecorder{}
	cmd := (&WriteBuilder{Root: root, Metrics: core.MetricConfig{Disabled: true}}).
		Build(toolReq(`{"path":"a.txt","content":"hello"}`))
	cmd.(core.MonitorRecorderAware).SetMonitorRecorder(rec)

	res := cmd.Execute()

	if res.Signal != core.ToolDone {
		t.Fatalf("write signal = %s", res.Signal)
	}
	if len(rec.samples) != 0 {
		t.Fatalf("disabled metrics recorded samples: %#v", rec.samples)
	}
}

func filesystemMetrics(name, source string) core.MetricConfig {
	return core.MetricConfig{
		Instruments: []core.MetricInstrument{{
			Name: name, Kind: "histogram", Unit: "By",
			Description: "Filesystem metric from declared source.", ValueSource: source,
			Attributes: []string{"operation", "path"},
		}},
		Attributes: []core.MetricAttribute{
			{Name: "operation", Source: "tool_name", Cardinality: "low", Redaction: "none"},
			{Name: "path", Source: "user_free_text", Cardinality: "low", Redaction: "none"},
		},
	}
}

func requireFilesystemMetric(t *testing.T, samples []monitor.MetricSample, name string, value float64) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name == name && sample.Value == value {
			return
		}
	}
	t.Fatalf("missing metric %s=%v in %#v", name, value, samples)
}
