// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"os"
	"testing"
)

func TestMonitorRuntimeOptInProfileSetsLoopRecorder(t *testing.T) {
	t.Parallel()
	machine := monitorRuntimeMachine()

	optIn, err := newMonitorRuntime(machine, nil, toolrest.Collection{}, nil, "monitor-runtime-run")
	require.NoError(t, err)
	require.NotNil(t, optIn.Store)
	require.NotNil(t, optIn.Recorder)

	params := core.LoopParams{MonitorRecorder: optIn.Recorder}
	require.NotNil(t, params.MonitorRecorder)

	disabled, err := newMonitorRuntime(core.MachineSpec{}, nil, toolrest.Collection{}, nil, "")
	require.NoError(t, err)
	require.Nil(t, disabled.Store)
	require.Nil(t, disabled.Recorder)
}

func TestMonitorRuntimeRecordsDispatchMetricsInStore(t *testing.T) {
	t.Parallel()
	runtime, err := newMonitorRuntime(monitorRuntimeMachine(), nil, toolrest.Collection{}, nil, "monitor-runtime-run")
	require.NoError(t, err)

	result := runMonitorRuntimeLoop(t, runtime)

	require.Equal(t, core.StatusSucceeded, result.Status)
	snapshot := runtime.Store.Snapshot()
	requireMonitorSample(t, snapshot.RecentSamples, "dispatch_count")
	requireMonitorSample(t, snapshot.RecentSamples, "dispatch_success")
}

func TestMonitorRuntimeUsesTelemetryMeter(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	runtime, err := newMonitorRuntime(monitorRuntimeMachine(), nil, toolrest.Collection{}, provider.Meter("agent"), "monitor-runtime-run")
	require.NoError(t, err)

	_ = runMonitorRuntimeLoop(t, runtime)

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))
	requireMetricData(t, data, "dispatch_count")
}

func TestMonitorRuntimeRejectsUnboundedToolAttributeSetup(t *testing.T) {
	t.Parallel()
	defs := []catalog.ToolDef{{
		Name: "invoke", Metrics: core.MetricConfig{
			Instruments: []core.MetricInstrument{{
				Name: "llm.tokens", Kind: "histogram", Unit: "1",
				Description: "Token count.", ValueSource: "tokens", Attributes: []string{"model"},
			}},
			Attributes: []core.MetricAttribute{{
				Name: "model", Source: "config_literal", Cardinality: "low", Redaction: "none",
			}},
		},
	}}

	_, err := newMonitorRuntime(monitorRuntimeMachine(), defs, toolrest.Collection{}, nil, "monitor-runtime-run")

	require.ErrorContains(t, err, `attribute "model" has no bounded allowed values`)
}

func TestMonitorRuntimeBindsDeclaredToolAndWorkflowAttributes(t *testing.T) {
	t.Parallel()
	machine := monitorRuntimeMachine()
	defs := []catalog.ToolDef{{
		Name: "read", Metrics: core.MetricConfig{
			Instruments: []core.MetricInstrument{{
				Name: "filesystem.bytes_read", Kind: "histogram", Unit: "By",
				Description: "Bytes read.", ValueSource: "bytes_read", Attributes: []string{"operation"},
			}},
			Attributes: []core.MetricAttribute{{
				Name: "operation", Source: "tool_name", Cardinality: "low", Redaction: "none",
			}},
		},
	}}
	runtime, err := newMonitorRuntime(machine, defs, toolrest.Collection{}, nil, "monitor-runtime-run")
	require.NoError(t, err)

	require.NoError(t, runtime.Recorder.RecordMetric(context.Background(), monitor.MetricSample{
		Name: "filesystem.bytes_read", Kind: monitor.InstrumentHistogram, Unit: "By",
		ToolName: "read", RunID: "monitor-runtime-run", State: "Working",
		Signal: "ToolDone", Status: "success", Value: 4,
		Attributes: map[string]string{
			"operation": "read", "profile": "monitor", "secret": "do-not-store",
		},
	}))

	snapshot := runtime.Store.Snapshot()
	require.Equal(t, map[string]string{"operation": "read", "profile": "monitor"},
		snapshot.RecentSamples[0].Attributes)
	require.Len(t, snapshot.Diagnostics, 1)
	require.NotContains(t, snapshot.Diagnostics[0].Message, "do-not-store")
}

func TestMonitorReleaseProfileProof(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })
	requireMainWiresMonitorRecorder(t)

	proof := monitorReleaseProof(t)
	resultCh := make(chan loopResult, 1)
	go func() {
		result, err := core.Loop(proof.params, context.Background())
		resultCh <- loopResult{result: result, err: err}
	}()
	waitForProofMonitorRoute(t, proof.monitorBaseURL+"/monitor/state")
	postProofMonitorExit(t, proof.monitorBaseURL+"/monitor/control/exit")
	outcome := receiveLoopResult(t, resultCh)
	require.NoError(t, outcome.err)
	require.Equal(t, core.State("Done"), outcome.result.FinalState)
	require.Equal(t, core.StatusSucceeded, outcome.result.Status)

	snapshot := proof.monitor.Store.Snapshot()
	requireMonitorSample(t, snapshot.RecentSamples, "dispatch_count")
	requireMonitorSampleAttribute(t, snapshot.RecentSamples, "dispatch_duration", "profile", "monitor")
	requireMonitorSampleAttribute(t, snapshot.RecentSamples, "dispatch_duration", "route_group", "monitor")

	var data metricdata.ResourceMetrics
	require.NoError(t, proof.metricReader.Collect(context.Background(), &data))
	requireMetricData(t, data, "dispatch_count")

	state, baseURL := launchProofMonitorREST(t, proof)
	defer func() { _, _ = state.Stop("monitor") }()
	metrics := proofRequestBody(t, baseURL+"/monitor/metrics")
	require.Contains(t, metrics, "dispatch_count")
	require.Contains(t, metrics, "route_group")
	require.Contains(t, proofRequestBody(t, baseURL+"/monitor/openapi"), "/monitor/metrics")
	require.Contains(t, proofRequestBody(t, baseURL+"/monitor/events/stream"), "event: metric_sample")
}

func TestMonitorCLIProfileServesUntilControlExit(t *testing.T) {
	root := repoRootFromTest(t)
	profilePath := profilePathFromTest(t, "monitor/profile.yaml")
	cmd, stdout, stderr := startMonitorAgentProcess(t, root, profilePath)
	resultCh := waitForProcess(t, cmd)

	baseURL := waitForMonitorBaseURL(t, stderr)
	waitForProofMonitorRoute(t, baseURL+"/monitor/state")
	stateBody := proofRequestBody(t, baseURL+"/monitor/state")
	require.Contains(t, stateBody, `"state"`)
	require.Contains(t, stateBody, `"run_id"`)
	require.NotContains(t, stateBody, `"State"`)
	require.NotContains(t, stateBody, `"RunID"`)
	require.Contains(t, proofRequestBody(t, baseURL+"/monitor/metrics"), "dispatch_count")
	requireProcessStillRunning(t, resultCh)
	postProofMonitorExit(t, baseURL+"/monitor/control/exit")
	requireProcessSucceeded(t, resultCh, stdout, stderr)
}

func TestMonitorProfileUsesEphemeralLoopbackDefault(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(profilePathFromTest(t, "monitor/rest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(data), "address: 127.0.0.1:0")
	require.NotContains(t, string(data), "address: 127.0.0.1:18083")
}
