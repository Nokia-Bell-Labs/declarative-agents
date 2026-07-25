// Copyright (c) 2026 Nokia. All rights reserved.

package otlp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	"github.com/stretchr/testify/require"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

func TestReceiverExportAwait(t *testing.T) {
	state := NewState()
	output, err := state.Launch(testReceiverConfig("receive"))
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = state.Stop("receive") })

	conn, client := traceClient(t, output["address"].(string))
	defer conn.Close()
	request := traceRequest("chatbot", 3)
	response, err := client.Export(context.Background(), request)
	require.NoError(t, err)
	require.Zero(t, response.GetPartialSuccess().GetRejectedSpans())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	batch, err := state.Next(ctx, "receive")
	require.NoError(t, err)
	require.Equal(t, 3, batch.SpanCount())
	require.True(t, proto.Equal(request, batch.Request))
	require.NotEmpty(t, batch.ID)
}

func TestReceiverOverflowPolicies(t *testing.T) {
	tests := []struct {
		name          string
		policy        OverflowPolicy
		rejected      int64
		expectedSpans int
	}{
		{name: "reject", policy: OverflowReject, rejected: 2, expectedSpans: 1},
		{name: "drop oldest", policy: OverflowDropOldest, expectedSpans: 2},
		{name: "drop newest", policy: OverflowDropNewest, expectedSpans: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := NewState()
			cfg := testReceiverConfig("overflow")
			cfg.QueueCapacity = 1
			cfg.OverflowPolicy = test.policy
			output, err := state.Launch(cfg)
			require.NoError(t, err)
			t.Cleanup(func() { _, _ = state.Stop("overflow") })
			conn, client := traceClient(t, output["address"].(string))
			defer conn.Close()

			_, err = client.Export(context.Background(), traceRequest("first", 1))
			require.NoError(t, err)
			response, err := client.Export(context.Background(), traceRequest("second", 2))
			require.NoError(t, err)
			require.Equal(t, test.rejected, response.GetPartialSuccess().GetRejectedSpans())

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			batch, err := state.Next(ctx, "overflow")
			require.NoError(t, err)
			require.Equal(t, test.expectedSpans, batch.SpanCount())
		})
	}
}

func TestReceiverStopUnblocksAndReleases(t *testing.T) {
	state := NewState()
	output, err := state.Launch(testReceiverConfig("lifecycle"))
	require.NoError(t, err)
	address := output["address"].(string)

	waited := make(chan error, 1)
	go func() {
		_, nextErr := state.Next(context.Background(), "lifecycle")
		waited <- nextErr
	}()
	stopOutput, err := state.Stop("lifecycle")
	require.NoError(t, err)
	require.Equal(t, "stopped", stopOutput["status"])
	require.ErrorIs(t, <-waited, ErrReceiverStopped)

	repeated, err := state.Stop("lifecycle")
	require.NoError(t, err)
	require.Equal(t, stopOutput, repeated)

	cfg := testReceiverConfig("replacement")
	cfg.Address = address
	_, err = state.Launch(cfg)
	require.NoError(t, err)
	_, err = state.Stop("replacement")
	require.NoError(t, err)
}

func TestReceiverStopAppliesDrainPolicy(t *testing.T) {
	tests := []struct {
		name            string
		policy          DrainPolicy
		expectedDropped int
		expectedQueued  int
	}{
		{name: "preserve", policy: DrainPreserve, expectedQueued: 2},
		{name: "drop", policy: DrainDrop, expectedDropped: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := NewState()
			cfg := testReceiverConfig("drain-" + test.name)
			cfg.DrainPolicy = test.policy
			_, err := state.Launch(cfg)
			require.NoError(t, err)
			runtime, err := state.runtime(cfg.Name)
			require.NoError(t, err)
			runtime.queue <- Batch{ID: "first", Request: traceRequest("first", 1)}
			runtime.queue <- Batch{ID: "second", Request: traceRequest("second", 2)}

			output, err := state.Stop(cfg.Name)
			require.NoError(t, err)
			require.Equal(t, 2, output["queued_batches"])
			require.Equal(t, test.expectedDropped, output["dropped_on_stop"])
			require.Equal(t, test.expectedDropped, output["dropped_batches"])
			require.Equal(t, test.expectedQueued, len(runtime.queue))
		})
	}
}

func TestReceiverRegistrationRejectsMalformedConfig(t *testing.T) {
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, NewState())
	factory, ok := br.Resolve(InitReceiverLaunch)
	require.True(t, ok)

	_, err := factory(catalog.ToolDef{
		Name: "launch_receiver",
		Config: map[string]any{
			"receiver": "ingress", "queue_capacity": -1,
			"overflow_policy": "invented", "shutdown_timeout": "bad",
		},
	}, nil)
	require.ErrorContains(t, err, "launch_receiver")

	_, err = factory(catalog.ToolDef{Name: "launch_receiver"}, nil)
	require.ErrorContains(t, err, "requires receiver")
}

func TestReceiverBuildersShareLifecycleState(t *testing.T) {
	state := NewState()
	launch := ReceiverBuilder{
		ToolName: "launch_receiver", Init: InitReceiverLaunch,
		Config: testReceiverConfig("shared"), State: state,
	}.Build(core.Result{})
	result := launch.Execute()
	require.Equal(t, core.Signal("ReceiverLaunched"), result.Signal)
	var launched map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Output), &launched))

	stop := ReceiverBuilder{
		ToolName: "stop_receiver", Init: InitReceiverStop,
		Config: ReceiverConfig{Name: "shared"}, State: state,
	}.Build(core.Result{})
	result = stop.Execute()
	require.Equal(t, core.Signal("ReceiverStopped"), result.Signal)
}

func testReceiverConfig(name string) ReceiverConfig {
	return ReceiverConfig{
		Name: name, Address: "127.0.0.1:0", QueueCapacity: 2,
		OverflowPolicy: OverflowReject, ShutdownTimeout: time.Second,
		DrainPolicy: DrainPreserve,
	}
}

func traceClient(
	t *testing.T,
	address string,
) (*grpc.ClientConn, coltracepb.TraceServiceClient) {
	t.Helper()
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	return conn, coltracepb.NewTraceServiceClient(conn)
}

func traceRequest(service string, spans int) *coltracepb.ExportTraceServiceRequest {
	items := make([]*tracepb.Span, 0, spans)
	for index := 0; index < spans; index++ {
		items = append(items, &tracepb.Span{
			TraceId: []byte("0123456789abcdef"),
			SpanId:  []byte{0, 0, 0, 0, 0, 0, 0, byte(index + 1)},
			Name:    service,
		})
	}
	return &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			ScopeSpans: []*tracepb.ScopeSpans{{Spans: items}},
		}},
	}
}
