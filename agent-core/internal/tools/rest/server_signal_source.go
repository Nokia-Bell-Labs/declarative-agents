// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const signalSourceTraceLimit = 128

func (r *serverRuntime) handleSignalSource(
	w http.ResponseWriter,
	req *http.Request,
	route string,
	endpoint Endpoint,
	requestPayload map[string]interface{},
) {
	requestID := signalSourceRequestID(req.Header.Get("X-Request-ID"))
	ctx, finish := startSignalSourceSpan(req.Context(), req, endpoint.SignalSource.Source, route, requestID)
	start := time.Now()
	envelope, mapped, err := mapSignalSourceRequest(endpoint.SignalSource, route, requestID, requestPayload)
	if err != nil {
		r.writeMappedSignalValidationError(w, endpoint.SignalSource, envelope, err, start, finish)
		return
	}
	if !mapped {
		r.writeUndeclaredSignalSource(w, endpoint.SignalSource, envelope, start, finish)
		return
	}
	if r.def.SignalSourceRunner == nil {
		r.writeUnavailableSignalSource(w, endpoint.SignalSource, envelope, start, finish)
		return
	}
	r.runMappedSignalSource(w, req, ctx, endpoint.SignalSource, envelope, start, finish)
}

func (r *serverRuntime) runMappedSignalSource(
	w http.ResponseWriter,
	req *http.Request,
	ctx context.Context,
	cfg SignalSourceBinding,
	envelope core.SignalEnvelope,
	start time.Time,
	finish func(signalSourceTrace),
) {
	ctx, cancel := context.WithTimeout(ctx, mustSignalSourceTimeout(cfg.Timeout))
	defer cancel()
	admission := r.def.SignalSourceRunner.RequestSignal(ctx, envelope)
	kind := signalSourceResponseKind(admission)
	if req.Context().Err() != nil {
		admission.Stage = "cancelled"
		kind = "machine_run_failed"
	} else if ctx.Err() == context.DeadlineExceeded {
		admission.Stage = "timeout"
		kind = "machine_run_failed"
	}
	status := signalSourceResponse(cfg.Responses, kind).Status
	finish(signalSourceAdmissionTrace(envelope, admission, status, time.Since(start)))
	r.writeSignalSourceResponse(w, cfg, envelope, signalSourceResult{
		kind: kind, outcome: string(admission.Outcome), admission: admission,
	})
}

func (r *serverRuntime) writeMappedSignalValidationError(
	w http.ResponseWriter,
	cfg SignalSourceBinding,
	envelope core.SignalEnvelope,
	err error,
	start time.Time,
	finish func(signalSourceTrace),
) {
	finish(signalSourceTrace{
		envelope: envelope, outcome: "source_validation_error",
		status: cfg.Responses.SourceValidation.Status,
		stage:  "source_validation", elapsed: time.Since(start), err: err,
	})
	r.writeSignalSourceResponse(w, cfg, envelope,
		signalSourceResult{kind: "source_validation", outcome: "source_validation_error"})
}

func (r *serverRuntime) writeUndeclaredSignalSource(
	w http.ResponseWriter,
	cfg SignalSourceBinding,
	envelope core.SignalEnvelope,
	start time.Time,
	finish func(signalSourceTrace),
) {
	admission := core.SignalAdmission{
		Outcome: core.AdmissionRefusedUndeclared, Source: envelope.Source,
		RequestID: envelope.RequestID, RunID: envelope.RunID, Signal: envelope.Signal,
		Stage: "undeclared",
	}
	finish(signalSourceAdmissionTrace(envelope, admission,
		cfg.Responses.RefusedUndeclared.Status, time.Since(start)))
	r.writeSignalSourceResponse(w, cfg, envelope,
		signalSourceResult{kind: "refused_undeclared", outcome: string(admission.Outcome), admission: admission})
}

func (r *serverRuntime) writeUnavailableSignalSource(
	w http.ResponseWriter,
	cfg SignalSourceBinding,
	envelope core.SignalEnvelope,
	start time.Time,
	finish func(signalSourceTrace),
) {
	err := fmt.Errorf("signal source runner is not configured")
	finish(signalSourceTrace{
		envelope: envelope, outcome: "machine_run_failed",
		status: cfg.Responses.MachineRunFailed.Status,
		stage:  "runner_unavailable", elapsed: time.Since(start), err: err,
	})
	r.writeSignalSourceResponse(w, cfg, envelope,
		signalSourceResult{kind: "machine_run_failed", outcome: "machine_run_failed"})
}

func (r *serverRuntime) handleSignalSourceValidationError(
	w http.ResponseWriter,
	req *http.Request,
	route string,
	endpoint Endpoint,
	err error,
) {
	requestID := signalSourceRequestID(req.Header.Get("X-Request-ID"))
	_, finish := startSignalSourceSpan(req.Context(), req, endpoint.SignalSource.Source, route, requestID)
	envelope := core.SignalEnvelope{
		Source: endpoint.SignalSource.Source, Route: route, RequestID: requestID,
	}
	status := endpoint.SignalSource.Responses.SourceValidation.Status
	finish(signalSourceTrace{
		envelope: envelope, outcome: "source_validation_error", status: status,
		stage: "source_validation", err: err,
	})
	r.writeSignalSourceResponse(w, endpoint.SignalSource, envelope,
		signalSourceResult{kind: "source_validation", outcome: "source_validation_error"})
}

func mapSignalSourceRequest(
	cfg SignalSourceBinding,
	route string,
	requestID string,
	request map[string]interface{},
) (core.SignalEnvelope, bool, error) {
	envelope := core.SignalEnvelope{Source: cfg.Source, Route: route, RequestID: requestID}
	if authority := callerSignalAuthority(request); authority != "" {
		return envelope, false, fmt.Errorf("caller field %q cannot provide signal or program authority", authority)
	}
	discriminator, err := requiredMappedString(request, cfg.DiscriminatorField, "discriminator")
	if err != nil {
		return envelope, false, err
	}
	envelope.RunID, err = requiredMappedString(request, cfg.RunIDField, "run_id")
	if err != nil {
		return envelope, false, err
	}
	if err := mapSignalExpectedState(request, cfg, &envelope); err != nil {
		return envelope, false, err
	}
	if err := mapSignalPayload(request, cfg, &envelope); err != nil {
		return envelope, false, err
	}
	signal, mapped := cfg.SignalMapping[discriminator]
	if mapped {
		envelope.Signal = core.Signal(signal)
	}
	return envelope, mapped, nil
}

func mapSignalExpectedState(
	request map[string]interface{},
	cfg SignalSourceBinding,
	envelope *core.SignalEnvelope,
) error {
	if cfg.ExpectedStateField == "" {
		return nil
	}
	expected, err := requiredMappedString(request, cfg.ExpectedStateField, "expected_state")
	if err != nil {
		return err
	}
	envelope.ExpectedState = core.State(expected)
	return nil
}

func mapSignalPayload(
	request map[string]interface{},
	cfg SignalSourceBinding,
	envelope *core.SignalEnvelope,
) error {
	payload := make(map[string]interface{}, len(cfg.Payload))
	for name, selector := range cfg.Payload {
		value, ok := resolveResultSelector(selector, request)
		if !ok {
			return fmt.Errorf("mapped payload field %q is missing", name)
		}
		payload[name] = value
	}
	var err error
	envelope.Payload, err = json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("map signal payload: %w", err)
	}
	for _, field := range cfg.Sensitive {
		envelope.SensitivePaths = append(envelope.SensitivePaths, core.OutputRedactionPath{field})
	}
	return nil
}

func requiredMappedString(
	request map[string]interface{},
	selector string,
	label string,
) (string, error) {
	value, ok := resolveResultSelector(selector, request)
	if !ok {
		return "", fmt.Errorf("mapped %s field is missing", label)
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("mapped %s must be a non-empty string", label)
	}
	return text, nil
}

func callerSignalAuthority(value interface{}) string {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, child := range typed {
			if signalSourceAuthorityFields[strings.ToLower(key)] {
				return key
			}
			if found := callerSignalAuthority(child); found != "" {
				return found
			}
		}
	case []interface{}:
		for _, child := range typed {
			if found := callerSignalAuthority(child); found != "" {
				return found
			}
		}
	}
	return ""
}

type signalSourceResult struct {
	kind      string
	outcome   string
	admission core.SignalAdmission
}

func signalSourceResponseKind(admission core.SignalAdmission) string {
	switch admission.Outcome {
	case core.AdmissionRefusedUndeclared:
		return "refused_undeclared"
	case core.AdmissionRefusedConflict:
		return "refused_conflict"
	case core.AdmissionAccepted:
		if admission.Err != nil || admission.RunStatus == core.StatusFailed ||
			admission.RunStatus == core.StatusCancelled ||
			admission.RunStatus == core.StatusBudgetExceeded {
			return "machine_run_failed"
		}
		return "accepted"
	default:
		return "machine_run_failed"
	}
}

func signalSourceResponse(
	responses SignalSourceResponseMappings,
	kind string,
) SignalSourceResponse {
	switch kind {
	case "accepted":
		return responses.Accepted
	case "refused_undeclared":
		return responses.RefusedUndeclared
	case "refused_conflict":
		response := responses.RefusedConflict
		if response.Status == 0 {
			response.Status = http.StatusConflict
		}
		return response
	case "source_validation":
		return responses.SourceValidation
	default:
		return responses.MachineRunFailed
	}
}

func (r *serverRuntime) writeSignalSourceResponse(
	w http.ResponseWriter,
	cfg SignalSourceBinding,
	envelope core.SignalEnvelope,
	result signalSourceResult,
) {
	mapping := signalSourceResponse(cfg.Responses, result.kind)
	admission := result.admission
	body := map[string]interface{}{
		"outcome": result.outcome, "source": envelope.Source,
		"request_id": envelope.RequestID, "run_id": envelope.RunID,
		"signal": envelope.Signal, "state_before": admission.StateBefore,
		"state_after": admission.StateAfter, "run_status": admission.RunStatus,
	}
	if mapping.IncludeDiagnostic {
		diagnostic := result.kind
		switch admission.Stage {
		case "panic", "machine_load_failed", "checkpoint_load_failed", "undeclared", "concurrent_conflict", "stale_expected_state", "no_exact_transition", "accepted", "cancelled", "timeout", "checkpoint_save_failed", "suspended", "succeeded", "budget_exceeded", "command_error", "run_failed", "source_validation", "runner_unavailable":
			diagnostic = admission.Stage
		}
		body["diagnostic"] = diagnostic
	}
	if mapping.IncludeOutput && admission.Run.Summary != "" {
		body["output"] = redactSignalSourceText(admission.Run.Summary, envelope, cfg.Sensitive)
	}
	writeJSON(w, mapping.Status, body)
}

func redactSignalSourceText(text string, envelope core.SignalEnvelope, sensitive []string) string {
	var payload map[string]interface{}
	if json.Unmarshal(envelope.Payload, &payload) != nil {
		return ""
	}
	for _, field := range sensitive {
		if value, ok := payload[field]; ok {
			if raw := fmt.Sprint(value); raw != "" {
				text = strings.ReplaceAll(text, raw, "[REDACTED]")
			}
		}
	}
	return text
}

func mustSignalSourceTimeout(value string) time.Duration {
	timeout, _ := time.ParseDuration(value)
	return timeout
}

func signalSourceRequestID(header string) string {
	if value := strings.TrimSpace(header); value != "" {
		return value
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return "request-" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("request-%d", time.Now().UnixNano())
}

type signalSourceTrace struct {
	envelope    core.SignalEnvelope
	outcome     string
	status      int
	stage       string
	stateBefore core.State
	stateAfter  core.State
	runStatus   core.RunStatus
	elapsed     time.Duration
	err         error
}

func startSignalSourceSpan(
	ctx context.Context,
	req *http.Request,
	source string,
	route string,
	requestID string,
) (context.Context, func(signalSourceTrace)) {
	if spanContext, err := telemetry.ParseTraceparent(req.Header.Get("traceparent")); err == nil &&
		spanContext.IsValid() {
		ctx = oteltrace.ContextWithRemoteSpanContext(ctx, spanContext)
	}
	ctx, span := otel.Tracer("agent-core/rest/signal_source").Start(ctx, "signal_source "+route,
		oteltrace.WithAttributes(
			attribute.String("signal.source", boundedSignalSourceTrace(source)),
			attribute.String("signal.route", boundedSignalSourceTrace(route)),
			core.AttrRequestID.String(boundedSignalSourceTrace(requestID)),
		))
	return ctx, func(trace signalSourceTrace) {
		span.SetAttributes(
			attribute.String("run.id", boundedSignalSourceTrace(trace.envelope.RunID)),
			attribute.String("signal.name", boundedSignalSourceTrace(string(trace.envelope.Signal))),
			attribute.String("signal.state_before", boundedSignalSourceTrace(string(trace.stateBefore))),
			attribute.String("signal.state_after", boundedSignalSourceTrace(string(trace.stateAfter))),
			attribute.String("signal.admission.outcome", trace.outcome),
			attribute.String("run.status", string(trace.runStatus)),
			attribute.String("signal.stage", trace.stage),
			attribute.Int("http.response.status_code", trace.status),
			attribute.Int64("signal.elapsed_ms", trace.elapsed.Milliseconds()),
		)
		if trace.err != nil {
			span.SetAttributes(attribute.String("error.type", fmt.Sprintf("%T", trace.err)))
		}
		span.End()
	}
}

func signalSourceAdmissionTrace(
	envelope core.SignalEnvelope,
	admission core.SignalAdmission,
	status int,
	elapsed time.Duration,
) signalSourceTrace {
	return signalSourceTrace{
		envelope: envelope, outcome: string(admission.Outcome), status: status,
		stage: admission.Stage, stateBefore: admission.StateBefore,
		stateAfter: admission.StateAfter, runStatus: admission.RunStatus,
		elapsed: elapsed, err: admission.Err,
	}
}

func boundedSignalSourceTrace(value string) string {
	if len(value) <= signalSourceTraceLimit {
		return value
	}
	return value[:signalSourceTraceLimit]
}
