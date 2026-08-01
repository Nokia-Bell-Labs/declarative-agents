// Copyright (c) 2026 Nokia. All rights reserved.

package otlp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

func TestReceiverExportAwaitMetrics(t *testing.T) {
	state := NewState()
	output, err := state.Launch(testReceiverConfig("metrics"))
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = state.Stop("metrics") })

	// Trace and metric services share one gRPC listener; a metric export must
	// not disturb the trace queue.
	traceConn, traceCli := traceClient(t, output["address"].(string))
	defer func() { _ = traceConn.Close() }()
	_, err = traceCli.Export(context.Background(), traceRequest("chatbot", 2))
	require.NoError(t, err)

	conn, client := metricClient(t, output["address"].(string))
	defer func() { _ = conn.Close() }()
	request := metricRequest("chatbot", "dispatch_count", 3)
	response, err := client.Export(context.Background(), request)
	require.NoError(t, err)
	require.Zero(t, response.GetPartialSuccess().GetRejectedDataPoints())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	batch, err := state.NextMetric(ctx, "metrics")
	require.NoError(t, err)
	require.Equal(t, 3, batch.DataPointCount())
	require.True(t, proto.Equal(request, batch.Request))
	require.Contains(t, batch.ID, "metric")

	// The trace queue still holds its own batch, untouched.
	traceBatch, err := state.Next(ctx, "metrics")
	require.NoError(t, err)
	require.Equal(t, 2, traceBatch.SpanCount())
}

func TestReceiverMetricOverflowPolicies(t *testing.T) {
	tests := []struct {
		name       string
		policy     OverflowPolicy
		rejected   int64
		expectedDP int
	}{
		{name: "reject", policy: OverflowReject, rejected: 2, expectedDP: 1},
		{name: "drop oldest", policy: OverflowDropOldest, expectedDP: 2},
		{name: "drop newest", policy: OverflowDropNewest, expectedDP: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := NewState()
			cfg := testReceiverConfig("moverflow")
			cfg.QueueCapacity = 1
			cfg.OverflowPolicy = test.policy
			cfg.DrainPolicy = DrainDrop
			output, err := state.Launch(cfg)
			require.NoError(t, err)
			t.Cleanup(func() { _, _ = state.Stop("moverflow") })
			conn, client := metricClient(t, output["address"].(string))
			defer func() { _ = conn.Close() }()

			_, err = client.Export(context.Background(), metricRequest("first", "m", 1))
			require.NoError(t, err)
			response, err := client.Export(context.Background(), metricRequest("second", "m", 2))
			require.NoError(t, err)
			require.Equal(t, test.rejected, response.GetPartialSuccess().GetRejectedDataPoints())

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			batch, err := state.NextMetric(ctx, "moverflow")
			require.NoError(t, err)
			require.Equal(t, test.expectedDP, batch.DataPointCount())

			stop, err := state.Stop("moverflow")
			require.NoError(t, err)
			require.Contains(t, stop, "dropped_metric_batches")
			require.Contains(t, stop, "dropped_data_points")
		})
	}
}

func TestAwaitMetricsSignals(t *testing.T) {
	t.Run("received", func(t *testing.T) {
		state := NewState()
		_, err := state.Launch(testReceiverConfig("mawait"))
		require.NoError(t, err)
		t.Cleanup(func() { _, _ = state.Stop("mawait") })
		runtime, err := state.runtime("mawait")
		require.NoError(t, err)
		runtime.metricQueue <- MetricBatch{
			ID: "metric-1", Request: metricRequest("chatbot", "dispatch_count", 2), Received: time.Now().UTC(),
		}

		result := MetricAwaitBuilder{
			ToolName: "await_metrics",
			Config:   AwaitConfig{Receiver: "mawait", Timeout: time.Second},
			State:    state,
		}.Build(core.Result{}).Execute()
		require.Equal(t, core.Signal("MetricsReceived"), result.Signal, result.Output)
		var out map[string]any
		require.NoError(t, json.Unmarshal([]byte(result.Output), &out))
		require.Equal(t, float64(2), out["data_point_count"])
		require.Equal(t, []any{"chatbot"}, out["service_names"])
		require.Equal(t, []any{"dispatch_count"}, out["metric_names"])
	})

	t.Run("timeout and stop", func(t *testing.T) {
		state := NewState()
		_, err := state.Launch(testReceiverConfig("msignals"))
		require.NoError(t, err)
		timeout := MetricAwaitBuilder{
			ToolName: "await_metrics",
			Config:   AwaitConfig{Receiver: "msignals", Timeout: time.Millisecond},
			State:    state,
		}.Build(core.Result{}).Execute()
		require.Equal(t, core.Signal("AwaitTimedOut"), timeout.Signal)
		_, err = state.Stop("msignals")
		require.NoError(t, err)
		stopped := MetricAwaitBuilder{
			ToolName: "await_metrics",
			Config:   AwaitConfig{Receiver: "msignals", Timeout: time.Second},
			State:    state,
		}.Build(core.Result{}).Execute()
		require.Equal(t, core.Signal("ReceiverStopped"), stopped.Signal)
	})
}

func TestSpoolMetricsRoundTripAndRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.ndjson")
	awaitJSON, err := awaitMetricOutput(MetricBatch{
		ID: "metric-1", Request: metricRequest("chatbot", "dispatch_count", 2), Received: time.Now().UTC(),
	})
	require.NoError(t, err)
	builder := SpoolMetricsBuilder{
		ToolName: "spool_metrics",
		Config:   SpoolConfig{Path: path, BatchSource: "$.batch", MaxBytes: 1, MaxFiles: 2},
	}
	for range 3 {
		result := builder.Build(core.Result{Output: awaitJSON}).Execute()
		require.Equal(t, core.Signal("MetricsSpooled"), result.Signal, result.Output)
	}
	_, err = os.Stat(path)
	require.NoError(t, err)
	_, err = os.Stat(path + ".1")
	require.NoError(t, err)
	_, err = os.Stat(path + ".2")
	require.ErrorIs(t, err, os.ErrNotExist)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var record metricRecord
	require.NoError(t, json.Unmarshal(data[:len(data)-1], &record))
	require.Equal(t, "dispatch_count", record.Name)
	require.Equal(t, "Sum", record.Type)
	require.Equal(t, 2, record.DataPointCount)
	require.NotEmpty(t, record.Resource)
	// stdoutAttributes sorts keys, so integration.run_id precedes service.name.
	require.Equal(t, "integration.run_id", record.Resource[0]["Key"])
	require.Equal(t, "service.name", record.Resource[1]["Key"])
	require.NotNil(t, record.Metric)
}

func TestSpoolMetricsResolvesCommandStateBatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selected-metrics.ndjson")
	awaitJSON, err := awaitMetricOutput(MetricBatch{
		ID: "metric-2", Request: metricRequest("rag0", "hits", 1), Received: time.Now().UTC(),
	})
	require.NoError(t, err)
	command := SpoolMetricsBuilder{
		ToolName: "spool_metrics",
		Config:   SpoolConfig{Path: path, BatchSource: "$from(await_batch).batch"},
	}.Build(core.Result{})
	command.(*spoolMetricsCommand).SetCommandState(mapCommandState{"await_batch": awaitJSON})
	result := command.Execute()
	require.Equal(t, core.Signal("MetricsSpooled"), result.Signal, result.Output)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var record metricRecord
	require.NoError(t, json.Unmarshal(data[:len(data)-1], &record))
	require.Equal(t, "hits", record.Name)
}

func metricClient(
	t *testing.T,
	address string,
) (*grpc.ClientConn, colmetricpb.MetricsServiceClient) {
	t.Helper()
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	return conn, colmetricpb.NewMetricsServiceClient(conn)
}

func metricRequest(service, name string, points int) *colmetricpb.ExportMetricsServiceRequest {
	items := make([]*metricpb.NumberDataPoint, 0, points)
	for index := 0; index < points; index++ {
		items = append(items, &metricpb.NumberDataPoint{
			TimeUnixNano: uint64(time.Unix(1_700_000_000, int64(index)).UnixNano()),
			Value:        &metricpb.NumberDataPoint_AsInt{AsInt: int64(index + 1)},
		})
	}
	return &colmetricpb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricpb.ResourceMetrics{{
			Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
				stringAttribute("service.name", service),
				stringAttribute("integration.run_id", "run-42"),
			}},
			ScopeMetrics: []*metricpb.ScopeMetrics{{
				Scope: &commonpb.InstrumentationScope{Name: "agent-core", Version: "test"},
				Metrics: []*metricpb.Metric{{
					Name: name, Unit: "1", Description: "test counter",
					Data: &metricpb.Metric_Sum{Sum: &metricpb.Sum{DataPoints: items}},
				}},
			}},
		}},
	}
}
