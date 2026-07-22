// Copyright (c) 2026 Nokia. All rights reserved.

// Implements srd008-telemetry R7 (offline replay).
package telemetry

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel/attribute"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

// ReplayFile reads an OTLP JSON trace file and sends its spans to a remote
// OTLP endpoint. It is idempotent: replaying the same file twice produces
// duplicate spans on the collector, which is acceptable (srd007 R7.4).
//
// An optional tracer records the replay lifecycle. If no tracer is provided,
// the function operates silently (errors are still returned).
func ReplayFile(path string, endpoint string, tr ...tracing.Tracer) error {
	return ReplayFileContext(context.Background(), path, endpoint, tr...)
}

// ReplayFileContext is ReplayFile with caller-controlled cancellation.
func ReplayFileContext(ctx context.Context, path string, endpoint string, tr ...tracing.Tracer) error {
	var t tracing.Tracer
	if len(tr) > 0 && tr[0] != nil {
		t = tr[0]
	}

	replayEvent(t, "replay.file_read_start",
		attribute.String("replay.path", path),
		attribute.String("replay.endpoint", endpoint),
	)

	data, err := os.ReadFile(path)
	if err != nil {
		replayEvent(t, "replay.file_read_error", attribute.String("error", err.Error()))
		return fmt.Errorf("replay: read %s: %w", path, err)
	}
	replayEvent(t, "replay.file_read_done", attribute.Int("file_bytes", len(data)))

	var req coltracepb.ExportTraceServiceRequest
	if err := protojson.Unmarshal(data, &req); err != nil {
		replayEvent(t, "replay.parse_error", attribute.String("error", err.Error()))
		return fmt.Errorf("replay: parse OTLP JSON from %s: %w", path, err)
	}

	spanCount := 0
	for _, rs := range req.GetResourceSpans() {
		for _, ss := range rs.GetScopeSpans() {
			spanCount += len(ss.GetSpans())
		}
	}
	replayEvent(t, "replay.parsed", attribute.Int("span_count", spanCount))

	conn, err := grpc.NewClient(endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		replayEvent(t, "replay.connect_error", attribute.String("error", err.Error()))
		return fmt.Errorf("replay: connect to %s: %w", endpoint, err)
	}
	defer func() { _ = conn.Close() }()

	resp, err := coltracepb.NewTraceServiceClient(conn).Export(ctx, &req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		replayEvent(t, "replay.export_error", attribute.String("error", err.Error()))
		return fmt.Errorf("replay: export to %s: %w", endpoint, err)
	}
	if partial := resp.GetPartialSuccess(); partial != nil && partial.GetRejectedSpans() > 0 {
		err := fmt.Errorf("%d spans rejected: %s", partial.GetRejectedSpans(), partial.GetErrorMessage())
		replayEvent(t, "replay.export_error", attribute.String("error", err.Error()))
		return fmt.Errorf("replay: partial export to %s: %w", endpoint, err)
	}

	replayEvent(t, "replay.done", attribute.Int("span_count", spanCount))
	return nil
}

func replayEvent(t tracing.Tracer, name string, attrs ...attribute.KeyValue) {
	if t != nil {
		t.Event(name, attrs...)
	}
}
