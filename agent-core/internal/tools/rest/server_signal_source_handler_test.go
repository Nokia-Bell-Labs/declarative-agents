// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestSignalSourceHandler_DefaultConflictAndConfiguredStatuses(t *testing.T) {
	t.Run("default conflict is 409", func(t *testing.T) {
		cfg := testSignalSourceBinding()
		runner := &recordingSignalSourceRunner{run: func(
			_ context.Context, envelope core.SignalEnvelope,
		) core.SignalAdmission {
			return signalAdmission(envelope, core.AdmissionRefusedConflict, "", "concurrent_conflict")
		}}
		state, baseURL := launchSignalSourceTestServer(t, cfg, runner)
		defer stopRESTServer(t, state, "signals")

		body := postJSON(t, baseURL+"/events", validSignalSourceJSON("secret"), http.StatusConflict)

		require.Equal(t, "refused_conflict", body["outcome"])
		require.Equal(t, "run-1", body["run_id"])
		require.Equal(t, "OrderCreated", body["signal"])
	})

	t.Run("configured accepted and conflict statuses", func(t *testing.T) {
		tests := []struct {
			name      string
			outcome   core.AdmissionOutcome
			runStatus core.RunStatus
			stage     string
			configure func(*SignalSourceBinding)
			want      int
		}{
			{
				name: "accepted", outcome: core.AdmissionAccepted,
				runStatus: core.StatusSucceeded, stage: "succeeded",
				configure: func(cfg *SignalSourceBinding) {
					cfg.Responses.Accepted.Status = http.StatusCreated
				},
				want: http.StatusCreated,
			},
			{
				name: "conflict", outcome: core.AdmissionRefusedConflict,
				runStatus: "", stage: "stale_expected_state",
				configure: func(cfg *SignalSourceBinding) {
					cfg.Responses.RefusedConflict.Status = http.StatusTooManyRequests
				},
				want: http.StatusTooManyRequests,
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				cfg := testSignalSourceBinding()
				test.configure(&cfg)
				runner := &recordingSignalSourceRunner{run: func(
					_ context.Context, envelope core.SignalEnvelope,
				) core.SignalAdmission {
					return signalAdmission(envelope, test.outcome, test.runStatus, test.stage)
				}}
				state, baseURL := launchSignalSourceTestServer(t, cfg, runner)
				defer stopRESTServer(t, state, "signals")

				body := postJSON(t, baseURL+"/events", validSignalSourceJSON("secret"), test.want)

				require.Equal(t, string(test.outcome), body["outcome"])
			})
		}
	})
}

func TestSignalSourceHandler_AcceptedRunFailureStaysAccepted(t *testing.T) {
	cfg := testSignalSourceBinding()
	cfg.Responses.MachineRunFailed.Status = http.StatusBadGateway
	cfg.Responses.MachineRunFailed.IncludeDiagnostic = true
	cfg.Responses.MachineRunFailed.IncludeOutput = true
	runner := &recordingSignalSourceRunner{run: func(
		_ context.Context, envelope core.SignalEnvelope,
	) core.SignalAdmission {
		admission := signalAdmission(envelope, core.AdmissionAccepted, core.StatusFailed, "/private/credentials/admin-token")
		admission.Err = errors.New(`open /private/credentials/admin-token: permission denied for profile "/srv/profiles/root.yaml" with secret`)
		admission.Run = core.RunResult{Status: core.StatusFailed, Summary: "failed secret", FinalState: "Failed"}
		return admission
	}}
	state, baseURL := launchSignalSourceTestServer(t, cfg, runner)
	defer stopRESTServer(t, state, "signals")

	body := postJSON(t, baseURL+"/events", validSignalSourceJSON("secret"), http.StatusBadGateway)

	require.Equal(t, "accepted", body["outcome"])
	require.Equal(t, string(core.StatusFailed), body["run_status"])
	require.Equal(t, "machine_run_failed", body["diagnostic"])
	require.Equal(t, "failed [REDACTED]", body["output"])
	require.NotContains(t, body, "secret")
	require.NotContains(t, body, "/private/")
	require.NotContains(t, body, "admin-token")
	require.NotContains(t, body, "root.yaml")
}

func TestSignalSourceHandler_TimeoutAndCancellationAreDistinct(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		cfg := testSignalSourceBinding()
		cfg.Timeout = "10ms"
		cfg.Responses.MachineRunFailed.Status = http.StatusGatewayTimeout
		cfg.Responses.MachineRunFailed.IncludeDiagnostic = true
		runner := &recordingSignalSourceRunner{run: successfulAfterSignalContext}
		state, baseURL := launchSignalSourceTestServer(t, cfg, runner)
		defer stopRESTServer(t, state, "signals")

		body := postJSON(t, baseURL+"/events", validSignalSourceJSON("secret"), http.StatusGatewayTimeout)

		require.Equal(t, "accepted", body["outcome"])
		require.Equal(t, string(core.StatusSucceeded), body["run_status"])
		require.Equal(t, "timeout", body["diagnostic"])
	})

	t.Run("caller cancellation", func(t *testing.T) {
		cfg := testSignalSourceBinding()
		cfg.Responses.MachineRunFailed.IncludeDiagnostic = true
		runner := &recordingSignalSourceRunner{run: successfulAfterSignalContext}
		endpoint := signalSourceTestEndpoint(cfg)
		runtime := &serverRuntime{
			name: "signals",
			def:  ServerDefinition{Name: "signals", SignalSourceRunner: runner},
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(validSignalSourceJSON("secret"))).WithContext(ctx)
		req.Header.Set("X-Request-ID", "cancelled-request")
		recorder := httptest.NewRecorder()

		runtime.handleSignalSource(recorder, req, "webhook", endpoint,
			signalSourceRequestPayload("created", "run-1", "public", "secret"))

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		body := decodeSignalSourceBody(t, recorder.Body.String())
		require.Equal(t, "accepted", body["outcome"])
		require.Equal(t, string(core.StatusSucceeded), body["run_status"])
		require.Equal(t, "cancelled", body["diagnostic"])
	})
}

func TestSignalSourceHandler_TraceIsBoundedAndSecretFree(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() {
		otel.SetTracerProvider(previous)
		require.NoError(t, provider.Shutdown(context.Background()))
	}()

	cfg := testSignalSourceBinding()
	cfg.Responses.MachineRunFailed.IncludeDiagnostic = true
	runner := &recordingSignalSourceRunner{run: func(
		_ context.Context, envelope core.SignalEnvelope,
	) core.SignalAdmission {
		admission := signalAdmission(envelope, core.AdmissionAccepted, core.StatusFailed, "command_error")
		admission.Err = errors.New("failed with secret")
		return admission
	}}
	state, baseURL := launchSignalSourceTestServer(t, cfg, runner)
	defer stopRESTServer(t, state, "signals")

	body := postJSON(t, baseURL+"/events", validSignalSourceJSON("secret"), http.StatusInternalServerError)

	require.NotContains(t, body, "secret")
	spans := spansNamedPrefix(recorder.Ended(), "signal_source webhook")
	require.Len(t, spans, 1)
	span := spans[0]
	require.Equal(t, "orders", stringSpanAttribute(t, span, "signal.source"))
	require.Equal(t, "webhook", stringSpanAttribute(t, span, "signal.route"))
	require.Equal(t, "run-1", stringSpanAttribute(t, span, "run.id"))
	require.Equal(t, "OrderCreated", stringSpanAttribute(t, span, "signal.name"))
	require.Equal(t, "Waiting", stringSpanAttribute(t, span, "signal.state_before"))
	require.Equal(t, "Failed", stringSpanAttribute(t, span, "signal.state_after"))
	require.Equal(t, "accepted", stringSpanAttribute(t, span, "signal.admission.outcome"))
	require.Equal(t, string(core.StatusFailed), stringSpanAttribute(t, span, "run.status"))
	for _, attr := range span.Attributes() {
		require.NotContains(t, attr.Value.String(), "secret")
		require.LessOrEqual(t, len(attr.Value.String()), signalSourceTraceLimit)
	}
}

func successfulAfterSignalContext(
	ctx context.Context,
	envelope core.SignalEnvelope,
) core.SignalAdmission {
	<-ctx.Done()
	return signalAdmission(envelope, core.AdmissionAccepted, core.StatusSucceeded, "succeeded")
}

func signalAdmission(
	envelope core.SignalEnvelope,
	outcome core.AdmissionOutcome,
	runStatus core.RunStatus,
	stage string,
) core.SignalAdmission {
	return core.SignalAdmission{
		Outcome: outcome, Source: envelope.Source, RequestID: envelope.RequestID,
		RunID: envelope.RunID, Signal: envelope.Signal, StateBefore: "Waiting",
		StateAfter: "Failed", RunStatus: runStatus, Stage: stage,
	}
}

func validSignalSourceJSON(secret string) string {
	return `{"event":"created","run_id":"run-1","expected_state":"Waiting","data":"public","token":"` + secret + `"}`
}

func signalSourceTestEndpoint(cfg SignalSourceBinding) Endpoint {
	return Endpoint{
		Method: http.MethodPost, Path: "/events", Binding: bindingSignalSource,
		SignalSource: cfg,
	}
}
