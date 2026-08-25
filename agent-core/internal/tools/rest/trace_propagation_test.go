// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry/genai"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func traceContextFixture(t *testing.T, tracestate string) oteltrace.SpanContext {
	t.Helper()
	traceID, err := oteltrace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	require.NoError(t, err)
	spanID, err := oteltrace.SpanIDFromHex("b7ad6b7169203331")
	require.NoError(t, err)
	cfg := oteltrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: oteltrace.FlagsSampled,
	}
	if tracestate != "" {
		ts, err := oteltrace.ParseTraceState(tracestate)
		require.NoError(t, err)
		cfg.TraceState = ts
	}
	return oteltrace.NewSpanContext(cfg)
}

// TestRESTServer_ExtractsTraceparentParentsMachineRequestSpan proves an inbound
// REST server endpoint parents the machine_request span on the extracted remote
// span, so a caller's client span and the callee's server span share one trace
// (srd016 R5; rel08.0-uc001 S2).
func TestRESTServer_ExtractsTraceparentParentsMachineRequestSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(prev)

	cfg := machineRequestConfig("DocumentationReady", 0, false)
	cfg.Response.TerminalSignals["DocumentationReady"] = restdef.MachineResponseMapping{Status: 200, Body: map[string]string{"path": "$.path"}}
	cfg.InitFunc = func(reg *core.Registry) error {
		reg.Register(core.ToolSpec{Name: "respond"}, pathEchoBuilder{})
		return nil
	}
	state, baseURL := launchMachineRequestServerWithConfig(t, cfg, catchAllDocsEndpoint(cfg))
	defer stopRESTServer(t, state, "machine")

	sc := traceContextFixture(t, "")
	req, err := http.NewRequest(http.MethodGet, baseURL+"/docs/VISION.yaml", nil)
	require.NoError(t, err)
	req.Header.Set("traceparent", telemetry.FormatTraceparent(sc))
	req.Header.Set("X-Request-ID", "request-traceparent")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	span := findMachineRequestSpan(t, recorder)
	require.Equal(t, sc.TraceID(), span.SpanContext().TraceID(), "server span joins the caller's trace")
	require.Equal(t, sc.SpanID(), span.Parent().SpanID(), "server span is a child of the caller's span")
	require.True(t, span.Parent().IsRemote(), "the parent is the remote client span")
	require.Equal(t, "request-traceparent", stringSpanAttribute(t, span, string(core.AttrRequestID)))
	require.Equal(t, "request-traceparent", stringSpanAttribute(t, span, string(genai.AttrConversationID)))
}

func TestRESTServer_ConcurrentMachineRequestsHaveDistinctConversationIdentity(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(prev)

	cfg := machineRequestConfig("DocumentationReady", 0, false)
	cfg.Timeout = "1s"
	cfg.Response.TerminalSignals["DocumentationReady"] = restdef.MachineResponseMapping{
		Status: 200,
		Body:   map[string]string{"path": "$.path"},
	}
	cfg.InitFunc = func(reg *core.Registry) error {
		reg.Register(core.ToolSpec{Name: "respond"}, pathEchoBuilder{})
		return nil
	}
	state, baseURL := launchMachineRequestServerWithConfig(t, cfg, catchAllDocsEndpoint(cfg))
	defer stopRESTServer(t, state, "machine")

	type requestOutcome struct {
		status int
		err    error
	}
	outcomes := make(chan requestOutcome, 2)
	for _, path := range []string{"FIRST.yaml", "SECOND.yaml"} {
		go func() {
			resp, err := http.Get(baseURL + "/docs/" + path)
			if err != nil {
				outcomes <- requestOutcome{err: err}
				return
			}
			_ = resp.Body.Close()
			outcomes <- requestOutcome{status: resp.StatusCode}
		}()
	}
	for range 2 {
		outcome := <-outcomes
		require.NoError(t, outcome.err)
		require.Equal(t, http.StatusOK, outcome.status)
	}

	machineSpans := spansNamedPrefix(recorder.Ended(), "machine_request ")
	groupingSpans := spansNamedPrefix(recorder.Ended(), "invoke_agent ")
	dispatchSpans := spansNamedPrefix(recorder.Ended(), "execute_tool ")
	require.Len(t, machineSpans, 2)
	require.Len(t, groupingSpans, 2)
	require.Len(t, dispatchSpans, 2)

	machineConversations := map[oteltrace.SpanID]string{}
	conversationIDs := map[string]struct{}{}
	for _, span := range machineSpans {
		requestID := stringSpanAttribute(t, span, string(core.AttrRequestID))
		conversationID := stringSpanAttribute(t, span, string(genai.AttrConversationID))
		require.Equal(t, requestID, conversationID)
		require.NotEmpty(t, requestID)
		machineConversations[span.SpanContext().SpanID()] = conversationID
		conversationIDs[conversationID] = struct{}{}
	}
	require.Len(t, conversationIDs, 2, "concurrent requests must not share conversation identity")

	groupingConversations := map[oteltrace.SpanID]string{}
	for _, span := range groupingSpans {
		parentConversation, ok := machineConversations[span.Parent().SpanID()]
		require.True(t, ok, "invoke_agent remains a child of machine_request")
		require.Equal(t, parentConversation, stringSpanAttribute(t, span, string(core.AttrRequestID)))
		require.Equal(t, parentConversation, stringSpanAttribute(t, span, string(genai.AttrConversationID)))
		groupingConversations[span.SpanContext().SpanID()] = parentConversation
	}
	for _, span := range dispatchSpans {
		parentConversation, ok := groupingConversations[span.Parent().SpanID()]
		require.True(t, ok, "dispatch remains a child of invoke_agent")
		require.Equal(t, parentConversation, stringSpanAttribute(t, span, string(genai.AttrConversationID)))
	}
}

// TestRESTServer_TraceparentFallbackToNewRoot proves an absent or malformed
// traceparent starts a new root span and the request still succeeds (srd016 R5.3;
// rel08.0-uc001 S3).
func TestRESTServer_TraceparentFallbackToNewRoot(t *testing.T) {
	for _, tc := range []struct {
		name   string
		header string
		set    bool
	}{
		{"absent", "", false},
		{"malformed", "not-a-traceparent", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			prev := otel.GetTracerProvider()
			otel.SetTracerProvider(provider)
			defer otel.SetTracerProvider(prev)

			cfg := machineRequestConfig("DocumentationReady", 0, false)
			cfg.Response.TerminalSignals["DocumentationReady"] = restdef.MachineResponseMapping{Status: 200, Body: map[string]string{"path": "$.path"}}
			cfg.InitFunc = func(reg *core.Registry) error {
				reg.Register(core.ToolSpec{Name: "respond"}, pathEchoBuilder{})
				return nil
			}
			state, baseURL := launchMachineRequestServerWithConfig(t, cfg, catchAllDocsEndpoint(cfg))
			defer stopRESTServer(t, state, "machine")

			req, err := http.NewRequest(http.MethodGet, baseURL+"/docs/VISION.yaml", nil)
			require.NoError(t, err)
			if tc.set {
				req.Header.Set("traceparent", tc.header)
			}
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode, "request succeeds despite %s traceparent", tc.name)

			span := findMachineRequestSpan(t, recorder)
			require.False(t, span.Parent().IsValid(), "a new root span has no parent")
		})
	}
}

// TestParseParentSpanRoundTripAndEmptyInput proves the in-process parser restores
// the formatted remote context and treats empty input as no parent.
func TestParseParentSpanRoundTripAndEmptyInput(t *testing.T) {
	t.Parallel()
	sc := traceContextFixture(t, "")
	ctx, err := telemetry.ParseParentSpan(telemetry.FormatTraceparent(sc))
	require.NoError(t, err)
	parent := oteltrace.SpanContextFromContext(ctx)
	require.Equal(t, sc.TraceID(), parent.TraceID())
	require.Equal(t, sc.SpanID(), parent.SpanID())
	require.True(t, parent.IsRemote())

	// Empty input remains a no-op that yields a background context.
	empty, err := telemetry.ParseParentSpan("")
	require.NoError(t, err)
	require.False(t, oteltrace.SpanContextFromContext(empty).IsValid())
}

func findMachineRequestSpan(t *testing.T, recorder *tracetest.SpanRecorder) sdktrace.ReadOnlySpan {
	t.Helper()
	spans := spansNamedPrefix(recorder.Ended(), "machine_request")
	if len(spans) > 0 {
		return spans[0]
	}
	t.Fatalf("no machine_request span was recorded")
	return nil
}

func spansNamedPrefix(spans []sdktrace.ReadOnlySpan, prefix string) []sdktrace.ReadOnlySpan {
	var matched []sdktrace.ReadOnlySpan
	for _, span := range spans {
		if strings.HasPrefix(span.Name(), prefix) {
			matched = append(matched, span)
		}
	}
	return matched
}

func stringSpanAttribute(t *testing.T, span sdktrace.ReadOnlySpan, key string) string {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			return attr.Value.AsString()
		}
	}
	t.Fatalf("span %q has no %q attribute", span.Name(), key)
	return ""
}
