// Copyright (c) 2026 Nokia. All rights reserved.

package filesystem

import (
	"context"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
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

func TestFilesystemMetricsCarryDispatchEnvelope(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cmd := (&WriteBuilder{Root: root, Metrics: filesystemMetrics("filesystem.bytes_written", "bytes_written")}).
		Build(toolReq(`{"path":"a.txt","content":"hello"}`))

	samples := runFilesystemMetricLoop(t, cmd, core.ToolDone)

	requireFilesystemMetric(t, samples, "filesystem.bytes_written", 5)
	requireFilesystemEnvelope(t, samples, "filesystem.bytes_written", cmd.Name())
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

func runFilesystemMetricLoop(t *testing.T, cmd core.Command, signal core.Signal) []monitor.MetricSample {
	t.Helper()
	// Keep this fixture package-local so filesystem assertions name filesystem commands and signals.
	store := monitor.NewStore(monitor.Limits{Samples: 10})
	params := filesystemMetricLoopParams(cmd, signal, monitor.NewRecorder(store, nil))
	_, err := core.Loop(params, context.Background())
	if err != nil {
		t.Fatalf("loop failed: %v", err)
	}
	return store.Snapshot().RecentSamples
}

func filesystemMetricLoopParams(cmd core.Command, signal core.Signal, rec monitor.RuntimeRecorder) core.LoopParams {
	spec := &core.MachineSpec{
		Name:           "filesystem-metrics",
		InitialState:   "Start",
		MetricLabels:   core.MetricLabels{"use_case": "rel04.0-monitor"},
		States:         core.StateSpecsFromNames("Start", "Working", "Done"),
		TerminalStates: []string{"Done"},
		Signals:        core.SignalSpecsFromNames(string(core.Seed), string(signal)),
		Transitions: []core.TransitionSpec{
			{State: "Start", Signal: string(core.Seed), Next: "Working", Action: cmd.Name(), MetricLabels: core.MetricLabels{"phase": "dispatch"}},
			{State: "Working", Signal: string(signal), Next: "Done"},
		},
	}
	return core.LoopParams{
		MachineSpec:     spec,
		AgentName:       "filesystem-run",
		Trace:           tracing.NoopTracer{},
		Budget:          core.Budget{MaxIterations: 3},
		MonitorRecorder: rec,
		InitFunc: func(reg *core.Registry) error {
			reg.Register(core.ToolSpec{Name: cmd.Name(), Visibility: core.Internal}, filesystemMetricBuilder{cmd: cmd})
			return nil
		},
		Hooks: core.LoopHooks{TerminalStatus: func(core.State) core.RunStatus { return core.StatusSucceeded }},
	}
}

type filesystemMetricBuilder struct {
	cmd core.Command
}

func (b filesystemMetricBuilder) Build(core.Result) core.Command {
	return b.cmd
}

func requireFilesystemEnvelope(t *testing.T, samples []monitor.MetricSample, name string, toolName string) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name != name {
			continue
		}
		if sample.ToolName != toolName || sample.RunID != "filesystem-run" ||
			sample.State != "Working" || sample.Signal != string(core.ToolDone) ||
			sample.Status != "success" || sample.Attributes["use_case"] != "rel04.0-monitor" ||
			sample.Attributes["phase"] != "dispatch" {
			t.Fatalf("bad metric envelope: %#v", sample)
		}
		return
	}
	t.Fatalf("missing metric %s in %#v", name, samples)
}
