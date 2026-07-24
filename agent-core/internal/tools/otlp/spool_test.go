// Copyright (c) 2026 Nokia. All rights reserved.

package otlp

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/evaluation"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestAwaitSpansSignals(t *testing.T) {
	t.Run("received", func(t *testing.T) {
		state := NewState()
		_, err := state.Launch(testReceiverConfig("await"))
		require.NoError(t, err)
		t.Cleanup(func() { _, _ = state.Stop("await") })
		runtime, err := state.runtime("await")
		require.NoError(t, err)
		runtime.queue <- Batch{ID: "batch-1", Request: spoolRequest(), Received: time.Now().UTC()}

		command := AwaitBuilder{
			ToolName: "await_spans",
			Config:   AwaitConfig{Receiver: "await", Timeout: time.Second},
			State:    state,
		}.Build(core.Result{})
		result := command.Execute()
		require.Equal(t, core.Signal("SpansReceived"), result.Signal)
		var output map[string]any
		require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
		require.Equal(t, float64(1), output["span_count"])
		require.Equal(t, []any{"chatbot"}, output["service_names"])
		require.NotNil(t, output["batch"])
	})

	t.Run("timeout and stop", func(t *testing.T) {
		state := NewState()
		_, err := state.Launch(testReceiverConfig("signals"))
		require.NoError(t, err)
		timeout := AwaitBuilder{
			ToolName: "await_spans",
			Config:   AwaitConfig{Receiver: "signals", Timeout: time.Millisecond},
			State:    state,
		}.Build(core.Result{}).Execute()
		require.Equal(t, core.Signal("AwaitTimedOut"), timeout.Signal)
		_, err = state.Stop("signals")
		require.NoError(t, err)
		stopped := AwaitBuilder{
			ToolName: "await_spans",
			Config:   AwaitConfig{Receiver: "signals", Timeout: time.Second},
			State:    state,
		}.Build(core.Result{}).Execute()
		require.Equal(t, core.Signal("ReceiverStopped"), stopped.Signal)
	})
}

func TestSpoolStdoutTraceAndRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collector.ndjson")
	awaitJSON, err := awaitOutput(Batch{
		ID: "batch-1", Request: spoolRequest(), Received: time.Now().UTC(),
	})
	require.NoError(t, err)
	builder := SpoolBuilder{
		ToolName: "spool_spans",
		Config: SpoolConfig{
			Path: path, BatchSource: "$.batch", MaxBytes: 1, MaxFiles: 2,
		},
	}

	for range 3 {
		result := builder.Build(core.Result{Output: awaitJSON}).Execute()
		require.Equal(t, core.Signal("SpansSpooled"), result.Signal, result.Output)
	}
	_, err = os.Stat(path)
	require.NoError(t, err)
	_, err = os.Stat(path + ".1")
	require.NoError(t, err)
	_, err = os.Stat(path + ".2")
	require.ErrorIs(t, err, os.ErrNotExist)

	for _, current := range []string{path, path + ".1"} {
		spans, readErr := evaluation.ReadTraceFile(current)
		require.NoError(t, readErr)
		require.Len(t, spans, 1)
		require.Equal(t, "client request", spans[0].Name)
		require.Equal(t, "0807060504030201", spans[0].SpanContext.SpanID)
		require.Equal(t, "0102030405060708", spans[0].Parent.SpanID)
		require.Equal(t, "GET /chat", evaluation.StrAttr(spans[0], "http.route"))
		requireCompleteJSONLines(t, current)
	}

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data[:len(data)-1], &raw))
	resource := raw["Resource"].([]any)
	require.NotEmpty(t, resource)
	require.Equal(t, "integration.run_id", resource[0].(map[string]any)["Key"])
	require.Equal(t, "service.name", resource[1].(map[string]any)["Key"])
}

func TestSpoolResolvesCommandStateBatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selected.ndjson")
	awaitJSON, err := awaitOutput(Batch{
		ID: "batch-2", Request: spoolRequest(), Received: time.Now().UTC(),
	})
	require.NoError(t, err)
	command := SpoolBuilder{
		ToolName: "spool_spans",
		Config: SpoolConfig{
			Path: path, BatchSource: "$from(await_batch).batch",
		},
	}.Build(core.Result{})
	command.(*spoolCommand).SetCommandState(mapCommandState{"await_batch": awaitJSON})
	result := command.Execute()
	require.Equal(t, core.Signal("SpansSpooled"), result.Signal, result.Output)
}

func requireCompleteJSONLines(t *testing.T, path string) {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		var value map[string]any
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &value))
		count++
	}
	require.NoError(t, scanner.Err())
	require.Equal(t, 1, count)
}

type mapCommandState map[string]string

func (m mapCommandState) Lookup(label string) (string, bool) {
	value, ok := m[label]
	return value, ok
}

func spoolRequest() *coltracepb.ExportTraceServiceRequest {
	traceID := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	return &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
				stringAttribute("service.name", "chatbot"),
				stringAttribute("integration.run_id", "run-42"),
			}},
			ScopeSpans: []*tracepb.ScopeSpans{{
				Scope: &commonpb.InstrumentationScope{Name: "agent-core", Version: "test"},
				Spans: []*tracepb.Span{{
					TraceId:           traceID,
					SpanId:            []byte{8, 7, 6, 5, 4, 3, 2, 1},
					ParentSpanId:      []byte{1, 2, 3, 4, 5, 6, 7, 8},
					Name:              "client request",
					Kind:              tracepb.Span_SPAN_KIND_CLIENT,
					StartTimeUnixNano: uint64(time.Unix(1_700_000_000, 0).UnixNano()),
					EndTimeUnixNano:   uint64(time.Unix(1_700_000_001, 0).UnixNano()),
					Attributes:        []*commonpb.KeyValue{stringAttribute("http.route", "GET /chat")},
					Events: []*tracepb.Span_Event{{
						Name: "sent", TimeUnixNano: uint64(time.Unix(1_700_000_000, 1).UnixNano()),
					}},
					Status: &tracepb.Status{Code: tracepb.Status_STATUS_CODE_OK},
				}},
			}},
		}},
	}
}

func stringAttribute(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key:   key,
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}},
	}
}
