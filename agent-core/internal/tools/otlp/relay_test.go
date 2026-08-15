// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type relayTraceService struct {
	coltracepb.UnimplementedTraceServiceServer
	requests chan *coltracepb.ExportTraceServiceRequest
	response *coltracepb.ExportTraceServiceResponse
}

func (s *relayTraceService) Export(
	_ context.Context,
	request *coltracepb.ExportTraceServiceRequest,
) (*coltracepb.ExportTraceServiceResponse, error) {
	s.requests <- request
	if s.response != nil {
		return s.response, nil
	}
	return &coltracepb.ExportTraceServiceResponse{}, nil
}

func TestRelayPreservesBatch(t *testing.T) {
	service, endpoint := startRelayService(t)
	request := spoolRequest()
	previous, err := awaitOutput(Batch{ID: "relay-1", Request: request, Received: time.Now().UTC()})
	require.NoError(t, err)
	command := RelayBuilder{
		ToolName: "relay_spans",
		Config: RelayConfig{
			Endpoint: endpoint, ReceiverAddress: "127.0.0.1:4317",
			BatchSource: "$.batch", Timeout: time.Second,
		},
	}.Build(core.Result{Output: previous})

	result := command.Execute()
	require.Equal(t, core.Signal("SpansRelayed"), result.Signal, result.Output)
	require.True(t, proto.Equal(request, <-service.requests))
}

func TestRelayReportsPartialRejection(t *testing.T) {
	service, endpoint := startRelayService(t)
	service.response = &coltracepb.ExportTraceServiceResponse{
		PartialSuccess: &coltracepb.ExportTracePartialSuccess{
			RejectedSpans: 1, ErrorMessage: "invalid span",
		},
	}
	previous, err := awaitOutput(Batch{
		ID: "relay-2", Request: spoolRequest(), Received: time.Now().UTC(),
	})
	require.NoError(t, err)
	result := RelayBuilder{
		ToolName: "relay_spans",
		Config: RelayConfig{
			Endpoint: endpoint, BatchSource: "$.batch", Timeout: time.Second,
		},
	}.Build(core.Result{Output: previous}).Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.ErrorContains(t, result.Err, "1 spans rejected")
	require.ErrorContains(t, result.Err, "invalid span")
}

func TestRelayHonorsCallerCancellation(t *testing.T) {
	t.Parallel()
	_, endpoint := startRelayService(t)
	previous, err := awaitOutput(Batch{
		ID: "relay-cancel", Request: spoolRequest(), Received: time.Now().UTC(),
	})
	require.NoError(t, err)
	command := RelayBuilder{
		ToolName: "relay_spans",
		Config: RelayConfig{
			Endpoint: endpoint, BatchSource: "$.batch", Timeout: time.Second,
		},
	}.Build(core.Result{Output: previous}).(*relayCommand)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := command.ExecuteContext(ctx)

	require.Equal(t, core.CommandError, result.Signal)
	require.ErrorIs(t, result.Err, context.Canceled)
}

func TestReceiverRejectsSelfLoopConfiguration(t *testing.T) {
	equivalent := [][2]string{
		{"0.0.0.0:4317", "127.0.0.1:4317"},
		{"[::]:4317", "[::1]:4317"},
		{"localhost:4317", "127.0.0.1:4317"},
	}
	for _, pair := range equivalent {
		err := validateRelayConfig("relay_spans", RelayConfig{
			Endpoint: pair[0], ReceiverAddress: pair[1],
			BatchSource: "$.batch", Timeout: time.Second,
		})
		require.ErrorContains(t, err, "own receiver")
	}
	require.NoError(t, validateRelayConfig("relay_spans", RelayConfig{
		Endpoint: "upstream:4317", ReceiverAddress: "0.0.0.0:4317",
		BatchSource: "$.batch", Timeout: time.Second,
	}))
}

func TestRelayAcceptsEmptyEndpointAtRegistrationOnly(t *testing.T) {
	config := RelayConfig{
		Endpoint: "", ReceiverAddress: "0.0.0.0:4317",
		BatchSource: "$.batch", Timeout: time.Second,
	}
	require.NoError(t, validateRelayConfig("relay_spans", config))

	previous, err := awaitOutput(Batch{
		ID: "relay-empty", Request: spoolRequest(), Received: time.Now().UTC(),
	})
	require.NoError(t, err)
	command := RelayBuilder{ToolName: "relay_spans", Config: config}.Build(
		core.Result{Output: previous})
	result := command.Execute()
	require.Equal(t, core.CommandError, result.Signal, result.Output)
	require.ErrorContains(t, result.Err, "relay endpoint is not configured")
}

func startRelayService(t *testing.T) (*relayTraceService, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	service := &relayTraceService{
		requests: make(chan *coltracepb.ExportTraceServiceRequest, 1),
	}
	server := grpc.NewServer()
	coltracepb.RegisterTraceServiceServer(server, service)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})
	return service, listener.Addr().String()
}
