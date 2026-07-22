// Copyright (c) 2026 Nokia. All rights reserved.

package telemetry

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestParseTraceparentProtocolBoundaries(t *testing.T) {
	t.Parallel()
	const (
		traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
		spanID  = "00f067aa0ba902b7"
	)
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "sampled", value: "00-" + traceID + "-" + spanID + "-01", valid: true},
		{name: "unsampled", value: "00-" + traceID + "-" + spanID + "-00", valid: true},
		{name: "all flag bits", value: "00-" + traceID + "-" + spanID + "-ff", valid: true},
		{name: "zero trace id", value: "00-" + strings.Repeat("0", 32) + "-" + spanID + "-01"},
		{name: "zero span id", value: "00-" + traceID + "-" + strings.Repeat("0", 16) + "-01"},
		{name: "short trace id", value: "00-" + traceID[:31] + "-" + spanID + "-01"},
		{name: "long trace id", value: "00-" + traceID + "0-" + spanID + "-01"},
		{name: "short span id", value: "00-" + traceID + "-" + spanID[:15] + "-01"},
		{name: "long span id", value: "00-" + traceID + "-" + spanID + "0-01"},
		{name: "uppercase", value: "00-4BF92F3577B34DA6A3CE929D0E0E4736-" + spanID + "-01"},
		{name: "nonhex", value: "00-" + traceID[:31] + "g-" + spanID + "-01"},
		{name: "extra component", value: "00-" + traceID + "-" + spanID + "-01-extra"},
		{name: "version 01", value: "01-" + traceID + "-" + spanID + "-01"},
		{name: "version ff", value: "ff-" + traceID + "-" + spanID + "-01"},
		{name: "leading whitespace", value: " 00-" + traceID + "-" + spanID + "-01"},
		{name: "embedded nul", value: "00-" + traceID + "-" + spanID + "-0\x00"},
		{name: "short flags", value: "00-" + traceID + "-" + spanID + "-0"},
		{name: "long flags", value: "00-" + traceID + "-" + spanID + "-001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sc, err := ParseTraceparent(tt.value)
			if !tt.valid {
				require.Error(t, err)
				assert.False(t, sc.IsValid())
				return
			}
			require.NoError(t, err)
			assert.True(t, sc.IsValid())
			assert.True(t, sc.IsRemote())
			assert.Equal(t, tt.value, FormatTraceparent(sc))
		})
	}
}

func TestFormatTraceparent_Invalid(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", FormatTraceparent(trace.SpanContext{}))
}

func TestParseTraceparentPreservesEveryFlagByte(t *testing.T) {
	t.Parallel()
	for flags := 0; flags <= 0xff; flags++ {
		value := fmt.Sprintf(
			"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-%02x",
			flags,
		)
		sc, err := ParseTraceparent(value)
		require.NoError(t, err, "flags %02x", flags)
		assert.Equal(t, trace.TraceFlags(flags), sc.TraceFlags(), "flags %02x", flags)
		assert.Equal(t, value, FormatTraceparent(sc), "flags %02x", flags)
	}
}

func FuzzTraceparentRoundTrip(f *testing.F) {
	for _, seed := range []string{
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-ff",
		"00-00000000000000000000000000000000-0000000000000000-00",
		"garbage",
		"",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		sc, err := ParseTraceparent(raw)
		if err != nil {
			if sc.IsValid() {
				t.Fatalf("failed parse returned valid context for %q", raw)
			}
			return
		}
		if !sc.IsValid() || !sc.IsRemote() {
			t.Fatalf("successful parse returned invalid or local context for %q", raw)
		}
		formatted := FormatTraceparent(sc)
		if len(formatted) != 55 || formatted != strings.ToLower(formatted) {
			t.Fatalf("noncanonical format %q from %q", formatted, raw)
		}
		roundTrip, roundTripErr := ParseTraceparent(formatted)
		if roundTripErr != nil {
			t.Fatalf("parse canonical format %q: %v", formatted, roundTripErr)
		}
		if roundTrip.TraceID() != sc.TraceID() ||
			roundTrip.SpanID() != sc.SpanID() ||
			roundTrip.TraceFlags() != sc.TraceFlags() {
			t.Fatalf("format round trip changed context: before=%v after=%v", sc, roundTrip)
		}
	})
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
