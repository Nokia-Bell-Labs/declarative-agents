// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
)

func TestLoopMonitorSamplesIncludeWorkflowMetricLabels(t *testing.T) {
	t.Parallel()
	store := monitor.NewStore(monitor.Limits{Samples: 10})
	rec, err := monitor.NewRecorderWithConfig(store, nil, workflowRecorderConfig(true))
	require.NoError(t, err)
	params := workflowMetricLoopParams(rec)

	rr, err := Loop(params, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, rr.Status)
	snapshot := store.Snapshot()
	require.Equal(t, "workflow-run", snapshot.Run.RunID)
	requireSampleLabels(t, snapshot.RecentSamples, "dispatch_duration", map[string]string{
		"use_case": "rel04.0-monitor", "phase": "dispatch", "agent.name": "workflow-agent",
	})
	requireSampleEnvelope(t, snapshot.RecentSamples, "dispatch_duration", monitor.MetricSample{
		ToolName: "emit_metric", RunID: "workflow-run", State: "Working",
		Signal: string(ToolDone), Status: "success",
	})
	requireSampleLabels(t, snapshot.RecentSamples, "tool.bytes", map[string]string{
		"use_case": "rel04.0-monitor", "phase": "dispatch",
	})
	requireSampleEnvelope(t, snapshot.RecentSamples, "tool.bytes", monitor.MetricSample{
		ToolName: "emit_metric", RunID: "workflow-run", State: "Working",
		Signal: string(ToolDone), Status: "success",
	})

	spanIdentity := map[string]string{}
	for _, attr := range runSpanAttrs(params) {
		switch string(attr.Key) {
		case "run.id", "gen_ai.agent.name":
			spanIdentity[string(attr.Key)] = attr.Value.AsString()
		}
	}
	require.Equal(t, "workflow-run", spanIdentity["run.id"])
	require.Equal(t, "workflow-agent", spanIdentity["gen_ai.agent.name"])
}

func TestLoopMetricRejectionDoesNotChangeCommandSignal(t *testing.T) {
	t.Parallel()
	store := monitor.NewStore(monitor.Limits{Samples: 10, Diagnostics: 10})
	rec, err := monitor.NewRecorderWithConfig(store, nil, workflowRecorderConfig(false))
	require.NoError(t, err)

	result, err := Loop(workflowMetricLoopParams(rec), context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, result.Status)
	require.Equal(t, State("Finished"), result.FinalState)
	snapshot := store.Snapshot()
	require.NotContains(t, snapshot.Metrics, "tool.bytes")
	require.Contains(t, snapshot.Diagnostics[0].Message, `"tool.bytes" is not declared`)
}

func workflowRecorderConfig(includeToolMetric bool) monitor.RecorderConfig {
	cfg := monitor.RecorderConfig{GlobalAttributes: []monitor.AttributePolicy{
		{Name: "use_case", AllowedValues: []string{"rel04.0-monitor"}},
		{Name: "phase", AllowedValues: []string{"setup", "dispatch"}},
		{Name: "agent.name", AllowedValues: []string{"workflow-agent"}},
	}}
	if includeToolMetric {
		cfg.Bindings = []monitor.MetricBinding{{
			ToolName: "emit_metric",
			Schema:   monitor.MetricSchema{Name: "tool.bytes", Kind: monitor.InstrumentHistogram, Unit: "By"},
		}}
	}
	return cfg
}
