// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
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

func pingDefinition(t *testing.T, baseURL string) Definition {
	t.Helper()
	def := Definition{
		Version: "v1",
		Auth:    map[string]AuthProfile{"none": {Type: authNone}},
		Limits:  map[string]LimitProfile{"test": {}},
		Clients: map[string]Client{
			"svc": {
				BaseURL: baseURL, AuthRef: "none", LimitsRef: "test",
				Operations: map[string]Operation{
					"ping": {
						Method:        http.MethodGet,
						Path:          "/ping",
						Params:        RequestBinding{BodySource: bodySourceNone},
						Success:       StatusMapping{Status: []int{200}, Signal: "Pinged"},
						Response:      ResponseMapping{Output: map[string]string{"ok": "$.ok"}},
						SideEffects:   []SideEffect{{Kind: "external_api", State: "read_only"}},
						Reversibility: Reversibility{Classification: "reversible", Undo: "noop"},
					},
				},
			},
		},
	}
	require.NoError(t, ValidateDefinition(def))
	return def
}

func TestRESTClient_InjectsTraceparentFromActiveSpan(t *testing.T) {
	t.Parallel()
	var gotTraceparent, gotTracestate string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotTraceparent = req.Header.Get("traceparent")
		gotTracestate = req.Header.Get("tracestate")
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	}))
	defer srv.Close()

	def := pingDefinition(t, srv.URL)
	op := resolveThreadingOp(t, def, "svc", "ping")
	sc := traceContextFixture(t, "vendor=abc123")

	cmd := threadingCommand(op, core.Result{})
	cmd.(core.TraceContextAware).SetTraceContext(sc)
	result := cmd.Execute()

	require.Equal(t, core.Signal("Pinged"), result.Signal, result.Output)
	require.Equal(t, telemetry.FormatTraceparent(sc), gotTraceparent)
	require.Equal(t, "vendor=abc123", gotTracestate)
}

func TestRESTClient_TraceparentUniformAndOmittedWithoutSpan(t *testing.T) {
	t.Parallel()

	var withSpan string
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		withSpan = req.Header.Get("traceparent")
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	}))
	defer srv1.Close()
	op1 := resolveThreadingOp(t, pingDefinition(t, srv1.URL), "svc", "ping")
	cmd1 := threadingCommand(op1, core.Result{})
	cmd1.(core.TraceContextAware).SetTraceContext(traceContextFixture(t, ""))
	require.Equal(t, core.Signal("Pinged"), cmd1.Execute().Signal)
	require.NotEmpty(t, withSpan, "traceparent injected when a span is active")

	var withoutSpan = "sentinel"
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		withoutSpan = req.Header.Get("traceparent")
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	}))
	defer srv2.Close()
	op2 := resolveThreadingOp(t, pingDefinition(t, srv2.URL), "svc", "ping")
	cmd2 := threadingCommand(op2, core.Result{})
	cmd2.(core.TraceContextAware).SetTraceContext(oteltrace.SpanContext{})
	require.Equal(t, core.Signal("Pinged"), cmd2.Execute().Signal)
	require.Empty(t, withoutSpan, "no traceparent emitted without a recording span")
}
