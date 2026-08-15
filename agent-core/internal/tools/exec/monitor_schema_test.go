// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package exec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func runExecMetricLoop(t *testing.T, cmd core.Command, signal core.Signal) []monitor.MetricSample {
	t.Helper()
	store := monitor.NewStore(monitor.Limits{Samples: 10})
	rec, err := monitor.NewRecorderWithConfig(store, nil, monitor.RecorderConfig{
		GlobalAttributes: []monitor.AttributePolicy{
			{Name: "use_case", AllowedValues: []string{"rel04.0-monitor"}},
			{Name: "phase", AllowedValues: []string{"dispatch"}},
			{Name: "agent.name", AllowedValues: []string{"exec-agent"}},
		},
		Bindings: execMetricBindings(cmd.Name()),
	})
	require.NoError(t, err)
	params := execMetricLoopParams(cmd, signal, rec)
	_, err = core.Loop(params, context.Background())
	require.NoError(t, err)
	return store.Snapshot().RecentSamples
}

func execMetricBindings(toolName string) []monitor.MetricBinding {
	return []monitor.MetricBinding{
		{ToolName: toolName, Schema: monitor.MetricSchema{
			Name: "exec.process_duration", Kind: monitor.InstrumentHistogram, Unit: "ms",
		}},
		{ToolName: toolName, Schema: monitor.MetricSchema{
			Name: "exec.output_bytes", Kind: monitor.InstrumentHistogram, Unit: "By",
		}},
		{ToolName: toolName, Schema: monitor.MetricSchema{
			Name: "exec.exit_code", Kind: monitor.InstrumentGauge, Unit: "1",
		}},
	}
}

func execMetricLoopParams(cmd core.Command, signal core.Signal, rec monitor.RuntimeRecorder) core.LoopParams {
	spec := &core.MachineSpec{
		Name:           "exec-metrics",
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
		MachineSpec: spec, RunID: "exec-run", AgentName: "exec-agent",
		Trace: tracing.NoopTracer{}, Budget: core.Budget{MaxIterations: 3}, MonitorRecorder: rec,
		InitFunc: func(reg *core.Registry) error {
			reg.Register(core.ToolSpec{Name: cmd.Name(), Visibility: core.Internal}, execMetricBuilder{cmd: cmd})
			return nil
		},
		Hooks: core.LoopHooks{TerminalStatus: func(core.State) core.RunStatus { return core.StatusSucceeded }},
	}
}

type execMetricBuilder struct {
	cmd core.Command
}

func (b execMetricBuilder) Build(core.Result) core.Command { return b.cmd }

func requireExecEnvelope(t *testing.T, samples []monitor.MetricSample, name string, toolName string) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name != name {
			continue
		}
		require.Equal(t, toolName, sample.ToolName)
		require.Equal(t, "exec-run", sample.RunID)
		require.Equal(t, "Working", sample.State)
		require.Equal(t, string(core.ToolDone), sample.Signal)
		require.Equal(t, "success", sample.Status)
		require.Equal(t, "rel04.0-monitor", sample.Attributes["use_case"])
		require.Equal(t, "dispatch", sample.Attributes["phase"])
		return
	}
	t.Fatalf("missing metric %s in %#v", name, samples)
}
