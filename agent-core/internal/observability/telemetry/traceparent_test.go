// Copyright (c) 2026 Nokia. All rights reserved.

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestParseTraceparent_Valid(t *testing.T) {
	t.Parallel()
	sc, err := ParseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	require.NoError(t, err)
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", sc.TraceID().String())
	assert.Equal(t, "00f067aa0ba902b7", sc.SpanID().String())
	assert.Equal(t, trace.TraceFlags(1), sc.TraceFlags())
	assert.True(t, sc.IsRemote())
}

func TestParseTraceparent_BadParts(t *testing.T) {
	t.Parallel()
	_, err := ParseTraceparent("00-abc-def")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "4 dash-separated parts")
}

func TestParseTraceparent_BadVersion(t *testing.T) {
	t.Parallel()
	_, err := ParseTraceparent("01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported version")
}

func TestFormatTraceparent_Valid(t *testing.T) {
	t.Parallel()
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	assert.Equal(t, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", FormatTraceparent(sc))
}

func TestFormatTraceparent_Invalid(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", FormatTraceparent(trace.SpanContext{}))
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	original := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	sc, err := ParseTraceparent(original)
	require.NoError(t, err)
	assert.Equal(t, original, FormatTraceparent(sc))
}

func TestParseParentSpan_Empty(t *testing.T) {
	t.Parallel()
	ctx, err := ParseParentSpan("")
	require.NoError(t, err)
	assert.NotNil(t, ctx)
	sc := trace.SpanContextFromContext(ctx)
	assert.False(t, sc.IsValid())
}

func TestParseParentSpan_Valid(t *testing.T) {
	t.Parallel()
	ctx, err := ParseParentSpan("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	require.NoError(t, err)
	sc := trace.SpanContextFromContext(ctx)
	assert.True(t, sc.IsValid())
	assert.True(t, sc.IsRemote())
}

func TestParseParentSpan_Invalid(t *testing.T) {
	t.Parallel()
	_, err := ParseParentSpan("garbage")
	assert.Error(t, err)
}
