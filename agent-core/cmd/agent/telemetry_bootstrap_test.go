// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry"
)

const testParentTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func TestInitRunTelemetryRejectsMalformedExplicitParent(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  runtimeConfig
	}{
		{
			name: "without exporters",
			cfg:  runtimeConfig{Telemetry: telemetry.Config{ParentSpan: "malformed"}},
		},
		{
			name: "before provider setup",
			cfg: runtimeConfig{
				Telemetry: telemetry.Config{
					ParentSpan: "malformed",
					LogFile:    filepath.Join(t.TempDir(), "missing", "trace.json"),
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracer, meter, shutdown, err := initRunTelemetry(tc.cfg)

			require.Error(t, err)
			require.Contains(t, err.Error(), "--otel-parent-span")
			require.Contains(t, err.Error(), "invalid traceparent")
			require.Nil(t, tracer)
			require.Nil(t, meter)
			require.Nil(t, shutdown)
		})
	}
}

func TestInitRunTelemetryParenting(t *testing.T) {
	for _, tc := range []struct {
		name        string
		parent      string
		wantTraceID string
		wantSpanID  string
		wantRemote  bool
	}{
		{
			name:        "valid input parents agent root",
			parent:      testParentTraceparent,
			wantTraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
			wantSpanID:  "00f067aa0ba902b7",
			wantRemote:  true,
		},
		{
			name:        "empty input leaves agent root unparented",
			wantTraceID: "00000000000000000000000000000000",
			wantSpanID:  "0000000000000000",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracePath := filepath.Join(t.TempDir(), "trace.json")
			_, _, shutdown, err := initRunTelemetry(runtimeConfig{
				Telemetry: telemetry.Config{
					LogFile:    tracePath,
					ParentSpan: tc.parent,
				},
			})
			require.NoError(t, err)
			shutdown()

			root := readExportedRootSpan(t, tracePath)
			require.Equal(t, "agent.run", root.Name)
			require.Equal(t, tc.wantTraceID, root.Parent.TraceID)
			require.Equal(t, tc.wantSpanID, root.Parent.SpanID)
			require.Equal(t, tc.wantRemote, root.Parent.Remote)
		})
	}
}

func readExportedRootSpan(t *testing.T, path string) exportedRootSpan {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var root exportedRootSpan
	require.NoError(t, json.NewDecoder(bytes.NewReader(data)).Decode(&root))
	return root
}

type exportedRootSpan struct {
	Name   string
	Parent struct {
		TraceID string
		SpanID  string
		Remote  bool
	}
}
