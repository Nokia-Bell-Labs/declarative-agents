// Copyright (c) 2026 Nokia. All rights reserved.

package telemetry

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// ParseTraceparent parses a W3C traceparent header (version 00) and
// returns a remote SpanContext. Format: "00-<traceID>-<spanID>-<flags>".
func ParseTraceparent(tp string) (trace.SpanContext, error) {
	parts := strings.Split(tp, "-")
	if len(parts) != 4 {
		return trace.SpanContext{}, fmt.Errorf("expected 4 dash-separated parts, got %d", len(parts))
	}
	if parts[0] != "00" {
		return trace.SpanContext{}, fmt.Errorf("unsupported version %q", parts[0])
	}
	traceID, err := trace.TraceIDFromHex(parts[1])
	if err != nil {
		return trace.SpanContext{}, fmt.Errorf("bad trace-id: %w", err)
	}
	spanID, err := trace.SpanIDFromHex(parts[2])
	if err != nil {
		return trace.SpanContext{}, fmt.Errorf("bad span-id: %w", err)
	}
	flagBytes, err := hex.DecodeString(parts[3])
	if err != nil || len(flagBytes) != 1 {
		return trace.SpanContext{}, fmt.Errorf("bad trace-flags %q", parts[3])
	}
	cfg := trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.TraceFlags(flagBytes[0]),
		Remote:     true,
	}
	return trace.NewSpanContext(cfg), nil
}

// FormatTraceparent builds a W3C traceparent string from a SpanContext.
// Returns "" if the SpanContext is invalid.
func FormatTraceparent(sc trace.SpanContext) string {
	if !sc.IsValid() {
		return ""
	}
	return fmt.Sprintf("00-%s-%s-%02x",
		sc.TraceID().String(),
		sc.SpanID().String(),
		byte(sc.TraceFlags()))
}

// ParseParentSpan is a convenience that parses a traceparent string and
// returns a context carrying the remote span. Returns context.Background()
// for an empty input.
func ParseParentSpan(raw string) (context.Context, error) {
	if raw == "" {
		return context.Background(), nil
	}
	sc, err := ParseTraceparent(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid traceparent %q: %w", raw, err)
	}
	return trace.ContextWithRemoteSpanContext(context.Background(), sc), nil
}
