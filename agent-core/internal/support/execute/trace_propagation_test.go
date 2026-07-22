// Copyright (c) 2026 Nokia. All rights reserved.

package execute

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/subprocess"
)

const traceChildModeEnv = "AGENT_TRACE_PROPAGATION_CHILD"

func TestRunAgentPropagatesOTelParentThroughChildProcess(t *testing.T) {
	if os.Getenv(traceChildModeEnv) == "1" {
		runTraceChild(t)
		return
	}

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	require.NoError(t, err)
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), parent)
	artifact := filepath.Join(t.TempDir(), "child-trace.json")
	script := writeTraceChildLauncher(t)

	result := RunAgent(ctx, Config{
		Binary: script, OTelLogFile: artifact, Timeout: 5 * time.Second,
	})

	require.True(t, result.Success(), "stdout=%s stderr=%s err=%v", result.Stdout, result.Stderr, result.Err)
	span := readExportedChildSpan(t, artifact)
	require.Equal(t, parent.TraceID().String(), span.SpanContext.TraceID)
	require.Equal(t, parent.TraceID().String(), span.Parent.TraceID)
	require.Equal(t, parent.SpanID().String(), span.Parent.SpanID)
}

func TestRunAgentOTelParentBoundaries(t *testing.T) {
	if os.Getenv(traceChildModeEnv) == "1" {
		runTraceChild(t)
		return
	}
	script := writeTraceChildLauncher(t)

	t.Run("missing parent emits root", func(t *testing.T) {
		artifact := filepath.Join(t.TempDir(), "root-trace.json")
		result := RunAgent(context.Background(), Config{
			Binary: script, OTelLogFile: artifact, Timeout: 5 * time.Second,
		})
		require.True(t, result.Success(), result.Stderr)
		span := readExportedChildSpan(t, artifact)
		require.Equal(t, "00000000000000000000000000000000", span.Parent.TraceID)
		require.Equal(t, "0000000000000000", span.Parent.SpanID)
	})

	t.Run("malformed parent reports parser diagnostic", func(t *testing.T) {
		result := subprocess.Run(context.Background(), subprocess.Spec{
			Binary: script,
			Args: []string{
				"--otel-log-file", filepath.Join(t.TempDir(), "invalid.json"),
				"--otel-parent-span", "malformed",
			},
			Timeout: 5 * time.Second,
		})
		require.False(t, result.Success())
		require.Contains(t, result.Stderr, "invalid traceparent")
		require.Contains(t, result.Stderr, "malformed")
	})

	t.Run("child failure preserves diagnostics", func(t *testing.T) {
		result := subprocess.Run(context.Background(), subprocess.Spec{
			Binary: script, Args: []string{"--fail-child"}, Timeout: 5 * time.Second,
		})
		require.False(t, result.Success())
		require.Equal(t, 42, result.ExitCode)
		require.Contains(t, result.Stderr, "controlled child failure")
	})
}

func runTraceChild(t *testing.T) {
	t.Helper()
	rawParent := os.Getenv("AGENT_TRACE_PARENT")
	artifact := os.Getenv("AGENT_TRACE_ARTIFACT")
	parentCtx, err := telemetry.ParseParentSpan(rawParent)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "parse --otel-parent-span: %v\n", err)
		t.FailNow()
	}
	tracer, shutdown, err := telemetry.NewRoot(
		"trace-child",
		"child.process",
		telemetry.ExporterConfig{FilePath: artifact},
		parentCtx,
	)
	require.NoError(t, err)
	tracer.Event("child emitted span")
	shutdown()
}

func writeTraceChildLauncher(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trace-child")
	script := fmt.Sprintf(`#!/bin/sh
set -eu
parent=
artifact=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --otel-parent-span) parent="$2"; shift 2 ;;
    --otel-log-file) artifact="$2"; shift 2 ;;
    --fail-child) echo "controlled child failure" >&2; exit 42 ;;
    *) shift ;;
  esac
done
AGENT_TRACE_PROPAGATION_CHILD=1 AGENT_TRACE_PARENT="$parent" AGENT_TRACE_ARTIFACT="$artifact" \
  exec %q -test.run='^TestRunAgent(PropagatesOTelParentThroughChildProcess|OTelParentBoundaries)$'
`, os.Args[0])
	require.NoError(t, os.WriteFile(path, []byte(script), 0o700))
	return path
}

type exportedChildSpan struct {
	Name        string `json:"Name"`
	SpanContext struct {
		TraceID string `json:"TraceID"`
		SpanID  string `json:"SpanID"`
	} `json:"SpanContext"`
	Parent struct {
		TraceID string `json:"TraceID"`
		SpanID  string `json:"SpanID"`
	} `json:"Parent"`
}

func readExportedChildSpan(t *testing.T, path string) exportedChildSpan {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var span exportedChildSpan
		if json.Unmarshal(scanner.Bytes(), &span) == nil && span.Name == "child.process" {
			return span
		}
	}
	require.NoError(t, scanner.Err())
	t.Fatalf("child.process span not found in %s", path)
	return exportedChildSpan{}
}
