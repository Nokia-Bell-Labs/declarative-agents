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
	"sync/atomic"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry/genai"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	"go.opentelemetry.io/otel"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func (r *serverRuntime) handleMachineRequest(
	w http.ResponseWriter,
	req *http.Request,
	name string,
	endpoint restdef.Endpoint,
	payload map[string]interface{},
) {
	ctx, cancel := context.WithTimeout(req.Context(), r.machineRequestTimeout(endpoint))
	defer cancel()
	requestID, conversationID := machineRequestIdentity(req.Header.Get("X-Request-ID"))
	ctx, endSpan := startMachineRequestSpan(ctx, req, r.name, name, requestID, conversationID)
	defer endSpan()
	result, err := r.runner.RunMachineRequest(ctx, MachineRequestRun{
		Server: r.name, Route: name, Method: req.Method, Path: req.URL.Path,
		RequestID:       requestID,
		Payload:         machineRequestPayload(endpoint.MachineRequest.Request, payload),
		Config:          endpoint.MachineRequest,
		MonitorRecorder: r.requestMonitor,
		RunID:           r.def.RunID,
		ConversationID:  conversationID,
	})
	if err != nil {
		writeMachineRequestError(w, err)
		return
	}
	if sc := oteltrace.SpanFromContext(ctx).SpanContext(); sc.HasTraceID() {
		result.TraceID = sc.TraceID().String()
	}
	r.writeMachineResponse(w, endpoint, result)
}

// startMachineRequestSpan extracts an incoming W3C traceparent and starts a span
// for the request machine parented on it, so a caller's client span and this
// server span join into one connected trace. An incoming tracestate rides along
// opaquely. An absent or malformed traceparent falls back to a new root span
// rather than failing the request, reusing the srd016 parser (srd016 R5).
func startMachineRequestSpan(
	ctx context.Context,
	req *http.Request,
	server, route, requestID, conversationID string,
) (context.Context, func()) {
	if tp := req.Header.Get("traceparent"); tp != "" {
		if sc, err := telemetry.ParseTraceparent(tp); err == nil {
			if ts := req.Header.Get("tracestate"); ts != "" {
				if parsed, tsErr := oteltrace.ParseTraceState(ts); tsErr == nil {
					sc = sc.WithTraceState(parsed)
				}
			}
			ctx = oteltrace.ContextWithRemoteSpanContext(ctx, sc)
		}
		// A malformed header falls through to a new root span (srd016 R5.3).
	}
	ctx, span := otel.Tracer("agent-core/rest/machine_request").Start(
		ctx,
		"machine_request "+server+"/"+route,
		oteltrace.WithAttributes(
			core.AttrRequestID.String(requestID),
			genai.AttrConversationID.String(conversationID),
		),
	)
	return ctx, func() { span.End() }
}

var machineRequestIDSequence atomic.Uint64

func machineRequestIdentity(requestID string) (string, string) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		var raw [16]byte
		if _, err := rand.Read(raw[:]); err == nil {
			requestID = "request-" + hex.EncodeToString(raw[:])
		} else {
			requestID = fmt.Sprintf(
				"request-%d-%d",
				time.Now().UnixNano(),
				machineRequestIDSequence.Add(1),
			)
		}
	}
	return requestID, requestID
}

func (r *serverRuntime) machineRequestTimeout(endpoint restdef.Endpoint) time.Duration {
	if timeout := parseDuration(endpoint.MachineRequest.Timeout, 0); timeout > 0 {
		return timeout
	}
	if timeout := parseDuration(r.def.Limits.Timeout, 0); timeout > 0 {
		return timeout
	}
	return defaultAwaitTimeout
}

func (r *serverRuntime) writeMachineResponse(
	w http.ResponseWriter,
	endpoint restdef.Endpoint,
	result MachineRequestResult,
) {
	mapping, _, ok := endpoint.MachineRequest.Response.ResponseMapping(
		string(result.Run.FinalState), result.TerminalSignal)
	if !ok {
		writeMachineRequestError(w, fmt.Errorf(
			"response_missing: neither terminal state %q nor terminal signal %q is mapped",
			result.Run.FinalState, result.TerminalSignal))
		return
	}
	status := mapping.Status
	if status == 0 {
		status = http.StatusOK
	}
	if mapping.ContentType != "" {
		w.Header().Set("Content-Type", mapping.ContentType)
	}
	for name, value := range mapping.Headers {
		w.Header().Set(name, value)
	}
	body := machineResponseBody(mapping, result)
	if err := validateMachineResponseBody(mapping, body); err != nil {
		writeMachineRequestError(w, err)
		return
	}
	if r.def.Limits.MaxResponseBytes > 0 && encodedJSONSize(body) > r.def.Limits.MaxResponseBytes {
		writeMachineRequestError(w, fmt.Errorf("response_invalid: response body too large"))
		return
	}
	writeMachineJSON(w, status, body)
}

func validateMachineResponseBody(mapping restdef.MachineResponseMapping, body map[string]interface{}) error {
	if len(mapping.Schema) == 0 {
		return nil
	}
	if err := validateBodySchema(mapping.Schema, body); err != nil {
		return fmt.Errorf("response_invalid: terminal response schema: %w", err)
	}
	return nil
}

func machineResponseBody(mapping restdef.MachineResponseMapping, result MachineRequestResult) map[string]interface{} {
	body := map[string]interface{}{}
	for name, selector := range mapping.Body {
		body[name] = machineSelectorValue(selector, result.Output)
	}
	if len(body) == 0 {
		body["data"] = result.Output
	}
	trace := map[string]interface{}{
		"server":          result.Server,
		"route":           result.Route,
		"machine":         result.Machine,
		"terminal_signal": result.TerminalSignal,
		"iterations":      result.Run.Iterations,
		"status":          result.Run.Status,
	}
	if result.TraceID != "" {
		trace["trace_id"] = result.TraceID
	}
	body["trace"] = trace
	return body
}

func machineSelectorValue(selector string, output map[string]interface{}) interface{} {
	if !strings.HasPrefix(selector, "$.") {
		return selector
	}
	value, _ := resolveResultSelector(selector, output)
	return value
}

func machineRequestPayload(mapping restdef.MachineRequestMapping, payload map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	copyMappedValues(out, payload, "body", mapping.Body)
	copyMappedValues(out, payload, "query", mapping.Query)
	copyMappedValues(out, payload, "path", mapping.Path)
	copyMappedValues(out, payload, "headers", mapping.Headers)
	if len(out) == 0 {
		return payload
	}
	return out
}

func copyMappedValues(out, payload map[string]interface{}, group string, mapping map[string]string) {
	source, _ := payload[group].(map[string]interface{})
	for name, selector := range mapping {
		out[name] = machineSelectorValue(selector, source)
	}
}

func writeMachineRequestError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	msg := err.Error()
	switch {
	case strings.Contains(msg, "machine_timeout"):
		status = http.StatusGatewayTimeout
	case strings.Contains(msg, "response_missing"):
		status = http.StatusBadGateway
	case strings.Contains(msg, "response_invalid"):
		status = http.StatusBadGateway
	case strings.Contains(msg, "machine_config_invalid"):
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, map[string]interface{}{"error": msg})
}

func writeMachineJSON(w http.ResponseWriter, status int, payload map[string]interface{}) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func encodedJSONSize(payload map[string]interface{}) int {
	data, err := json.Marshal(payload)
	if err != nil {
		return 0
	}
	return len(data)
}
