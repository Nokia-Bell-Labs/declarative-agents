// Copyright (c) 2026 Nokia. All rights reserved.

// Implements srd008-telemetry R7 (offline replay).
package telemetry

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
)

// ReplayFile reads an OTLP JSON trace file and sends its spans to a remote
// OTLP endpoint. It is idempotent: replaying the same file twice produces
// duplicate spans on the collector, which is acceptable (srd007 R7.4).
//
// An optional tracer records the replay lifecycle. If no tracer is provided,
// the function operates silently (errors are still returned).
func ReplayFile(path string, endpoint string, tr ...tracing.Tracer) error {
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

	ctx := context.Background()
	conn, err := grpc.NewClient(endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		replayEvent(t, "replay.connect_error", attribute.String("error", err.Error()))
		return fmt.Errorf("replay: connect to %s: %w", endpoint, err)
	}
	defer conn.Close()

	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		replayEvent(t, "replay.exporter_error", attribute.String("error", err.Error()))
		return fmt.Errorf("replay: create exporter: %w", err)
	}
	defer func() { _ = exp.Shutdown(ctx) }()

	if err := exp.ExportSpans(ctx, nil); err != nil {
		replayEvent(t, "replay.export_error", attribute.String("error", err.Error()))
		return fmt.Errorf("replay: export to %s: %w", endpoint, err)
	}

	replayEvent(t, "replay.done", attribute.Int("span_count", spanCount))
	return nil
}

func replayEvent(t tracing.Tracer, name string, attrs ...attribute.KeyValue) {
	if t != nil {
		t.Event(name, attrs...)
	}
}
