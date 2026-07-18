// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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

// TestRESTClient_InjectsTraceparentFromActiveSpan proves an outbound REST client
// request carries a traceparent formatted from the active dispatch span, and
// tracestate when the span carries it (srd016 R4; rel08.0-uc001 S1).
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

// TestRESTClient_TraceparentUniformAndOmittedWithoutSpan proves injection applies
// with no per-operation configuration, and that no traceparent is emitted when no
// recording span is active (srd016 R4; rel08.0-uc001 S4).
func TestRESTClient_TraceparentUniformAndOmittedWithoutSpan(t *testing.T) {
	t.Parallel()

	// With a span: header present.
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

	// Without a span (zero SpanContext, as the engine injects when no recording
	// span is active): no header.
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
	cfg.Response.TerminalSignals["DocumentationReady"] = MachineResponseMapping{Status: 200, Body: map[string]string{"path": "$.path"}}
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
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	span := findMachineRequestSpan(t, recorder)
	require.Equal(t, sc.TraceID(), span.SpanContext().TraceID(), "server span joins the caller's trace")
	require.Equal(t, sc.SpanID(), span.Parent().SpanID(), "server span is a child of the caller's span")
	require.True(t, span.Parent().IsRemote(), "the parent is the remote client span")
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
			cfg.Response.TerminalSignals["DocumentationReady"] = MachineResponseMapping{Status: 200, Body: map[string]string{"path": "$.path"}}
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

// TestOtelParentSpanPropagationUnchanged proves the subprocess trace-context path
// (ParseParentSpan) still parents a child on the caller's remote span,
// independent of and complementary to the HTTP path (srd016 R5.4; rel08.0-uc001
// S1).
func TestOtelParentSpanPropagationUnchanged(t *testing.T) {
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
	for _, span := range recorder.Ended() {
		if strings.HasPrefix(span.Name(), "machine_request") {
			return span
		}
	}
	t.Fatalf("no machine_request span was recorded")
	return nil
}
