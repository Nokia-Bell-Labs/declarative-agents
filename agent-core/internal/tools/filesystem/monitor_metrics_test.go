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

	write := (&WriteBuilder{Root: root}).Build(toolReq(`{"path":"a.txt","content":"hello"}`))
	write.(core.MonitorRecorderAware).SetMonitorRecorder(rec)
	if res := write.Execute(); res.Signal != core.ToolDone {
		t.Fatalf("write signal = %s", res.Signal)
	}

	read := (&ReadBuilder{Root: root}).Build(toolReq(`{"path":"a.txt"}`))
	read.(core.MonitorRecorderAware).SetMonitorRecorder(rec)
	if res := read.Execute(); res.Signal != core.ToolDone {
		t.Fatalf("read signal = %s", res.Signal)
	}

	edit := (&EditBuilder{Root: root}).Build(toolReq(`{"path":"a.txt","old_string":"hello","new_string":"hello!"}`))
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

func requireFilesystemMetric(t *testing.T, samples []monitor.MetricSample, name string, value float64) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name == name && sample.Value == value {
			return
		}
	}
	t.Fatalf("missing metric %s=%v in %#v", name, value, samples)
}
