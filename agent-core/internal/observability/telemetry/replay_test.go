// Copyright (c) 2026 Nokia. All rights reserved.

package telemetry

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stretchr/testify/require"
)

type recordingTraceService struct {
	coltracepb.UnimplementedTraceServiceServer
	requests chan *coltracepb.ExportTraceServiceRequest
	response *coltracepb.ExportTraceServiceResponse
}

func (s *recordingTraceService) Export(
	_ context.Context,
	req *coltracepb.ExportTraceServiceRequest,
) (*coltracepb.ExportTraceServiceResponse, error) {
	s.requests <- req
	if s.response != nil {
		return s.response, nil
	}
	return &coltracepb.ExportTraceServiceResponse{}, nil
}

func TestReplayFileExportsParsedSpanIdentity(t *testing.T) {
	service, endpoint := startRecordingTraceService(t)
	traceID := []byte("0123456789abcdef")
	spanID := []byte("span-id!")
	path := writeReplayRequest(t, &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{ScopeSpans: []*tracepb.ScopeSpans{{
			Spans: []*tracepb.Span{{TraceId: traceID, SpanId: spanID, Name: "replayed-span"}},
		}}}},
	})

	require.NoError(t, ReplayFile(path, endpoint))
	exported := <-service.requests
	spans := exported.GetResourceSpans()[0].GetScopeSpans()[0].GetSpans()
	require.Len(t, spans, 1)
	require.Equal(t, traceID, spans[0].GetTraceId())
	require.Equal(t, spanID, spans[0].GetSpanId())
	require.Equal(t, "replayed-span", spans[0].GetName())
}

func TestReplayFileBoundaries(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		err := ReplayFile(filepath.Join(t.TempDir(), "missing.json"), "127.0.0.1:1")
		require.ErrorContains(t, err, "replay: read")
	})
	t.Run("corrupt payload", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "trace.json")
		require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))
		err := ReplayFile(path, "127.0.0.1:1")
		require.ErrorContains(t, err, "replay: parse OTLP JSON")
	})
	t.Run("empty payload", func(t *testing.T) {
		service, endpoint := startRecordingTraceService(t)
		path := writeReplayRequest(t, &coltracepb.ExportTraceServiceRequest{})
		require.NoError(t, ReplayFile(path, endpoint))
		require.Empty(t, (<-service.requests).GetResourceSpans())
	})
	t.Run("partial rejection", func(t *testing.T) {
		service, endpoint := startRecordingTraceService(t)
		service.response = &coltracepb.ExportTraceServiceResponse{
			PartialSuccess: &coltracepb.ExportTracePartialSuccess{
				RejectedSpans: 1,
				ErrorMessage:  "invalid span",
			},
		}
		err := ReplayFile(writeReplayRequest(t, replayRequestWithOneSpan()), endpoint)
		require.ErrorContains(t, err, "1 spans rejected")
		require.ErrorContains(t, err, "invalid span")
	})
	t.Run("canceled export", func(t *testing.T) {
		_, endpoint := startRecordingTraceService(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := ReplayFileContext(ctx, writeReplayRequest(t, replayRequestWithOneSpan()), endpoint)
		require.ErrorIs(t, err, context.Canceled)
	})
}

func replayRequestWithOneSpan() *coltracepb.ExportTraceServiceRequest {
	return &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{ScopeSpans: []*tracepb.ScopeSpans{{
			Spans: []*tracepb.Span{{TraceId: []byte("0123456789abcdef"), SpanId: []byte("span-id!")}},
		}}}},
	}
}

func writeReplayRequest(t *testing.T, req *coltracepb.ExportTraceServiceRequest) string {
	t.Helper()
	data, err := protojson.Marshal(req)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "trace.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func startRecordingTraceService(t *testing.T) (*recordingTraceService, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	service := &recordingTraceService{requests: make(chan *coltracepb.ExportTraceServiceRequest, 1)}
	server := grpc.NewServer()
	coltracepb.RegisterTraceServiceServer(server, service)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})
	return service, listener.Addr().String()
}
