// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	bindingEmitSignal     = "emit_signal"
	bindingReadState      = "read_state"
	bindingInvokeHandler  = "invoke_handler"
	bindingStreamEvents   = "stream_events"
	bindingHealth         = "health"
	bindingStaticMetadata = "static_metadata"
	bindingMachineRequest = "machine_request"
)

var allowedUndeclaredHeaders = map[string]bool{
	"accept": true, "accept-encoding": true, "connection": true,
	"content-length": true, "content-type": true, "user-agent": true,
	"x-request-id": true,
}

func (r *serverRuntime) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	name, endpoint, vars, pathFound := r.matchEndpoint(req)
	if !pathFound {
		http.NotFound(w, req)
		return
	}
	if req.Method != endpoint.Method {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	payload, err := readRequestPayload(req, endpoint, r.def.Limits.MaxRequestBytes)
	if err != nil {
		writeRequestError(w, err)
		return
	}
	if err := addPathValues(payload, endpoint.Request.Path, vars); err != nil {
		writeRequestError(w, err)
		return
	}
	r.handleEndpoint(w, req, name, endpoint, payload)
}

func (r *serverRuntime) matchEndpoint(req *http.Request) (string, Endpoint, map[string]string, bool) {
	for name, endpoint := range r.def.Server.Endpoints {
		vars, ok := matchPath(endpoint.Path, req.URL.Path)
		if ok {
			return name, endpoint, vars, true
		}
	}
	return "", Endpoint{}, nil, false
}

func (r *serverRuntime) handleEndpoint(
	w http.ResponseWriter,
	req *http.Request,
	name string,
	endpoint Endpoint,
	payload map[string]interface{},
) {
	switch endpoint.Binding {
	case bindingEmitSignal:
		r.enqueueSignal(w, req, name, endpoint.Signal, payload, endpoint.Response.Redact)
	case bindingDynamicSignal:
		r.enqueueDynamicSignal(w, req, name, endpoint, payload)
	case bindingReadState:
		writeJSON(w, http.StatusOK, r.stateOutput())
	case bindingInvokeHandler:
		r.invokeHandler(w, req, name, endpoint, payload)
	case bindingStreamEvents:
		r.streamEvents(w)
	case bindingHealth:
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
	case bindingStaticMetadata:
		writeJSON(w, http.StatusOK, r.metadataOutput())
	case bindingMachineRequest:
		r.handleMachineRequest(w, req, name, endpoint, payload)
	default:
		http.Error(w, "endpoint binding is not implemented", http.StatusNotImplemented)
	}
}

func (r *serverRuntime) invokeHandler(
	w http.ResponseWriter,
	req *http.Request,
	name string,
	endpoint Endpoint,
	payload map[string]interface{},
) {
	if endpoint.Signal != "" {
		redactServerPayload(payload, endpoint.Response.Redact)
		event := inboundEvent(r.name, name, req.Method, endpoint.Signal, payload, req.Header.Get("X-Request-ID"))
		if !r.offerEvent(event) {
			http.Error(w, "REST server queue is full", http.StatusTooManyRequests)
			return
		}
	}
	if endpoint.Signal == "" && len(endpoint.Response.Output) == 0 {
		http.Error(w, "handler is not configured", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, serverResponseOutput(endpoint.Response, payload))
}

func (r *serverRuntime) enqueueDynamicSignal(
	w http.ResponseWriter,
	req *http.Request,
	name string,
	endpoint Endpoint,
	payload map[string]interface{},
) {
	signal := signalFromRequest(req, payload)
	if !allowedSignal(signal, endpoint.AllowedSignals) {
		http.Error(w, "signal is not allowed", http.StatusBadRequest)
		return
	}
	r.enqueueSignal(w, req, name, signal, payload, endpoint.Response.Redact)
}

func (r *serverRuntime) enqueueSignal(
	w http.ResponseWriter,
	req *http.Request,
	route string,
	signal string,
	payload map[string]interface{},
	redact []string,
) {
	if signal == "" {
		http.Error(w, "endpoint signal is not configured", http.StatusInternalServerError)
		return
	}
	redactServerPayload(payload, redact)
	event := inboundEvent(r.name, route, req.Method, signal, payload, req.Header.Get("X-Request-ID"))
	if !r.offerEvent(event) {
		http.Error(w, "REST server queue is full", http.StatusTooManyRequests)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]interface{}{"accepted": true, "signal": signal})
}

func (r *serverRuntime) offerEvent(event InboundEvent) bool {
	select {
	case r.queue <- event:
		return true
	default:
		r.mu.Lock()
		r.droppedEvents++
		r.mu.Unlock()
		return false
	}
}

func inboundEvent(source, route, method, signal string, payload map[string]interface{}, requestID string) InboundEvent {
	return InboundEvent{
		Source: source, Route: route, Method: method,
		Signal: signal, Payload: payload, RequestID: requestID,
	}
}

func (r *serverRuntime) streamEvents(w http.ResponseWriter) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}
	r.incrementStreams()
	defer r.decrementStreams()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	select {
	case event := <-r.queue:
		writeSSEEvent(w, flusher, "message", event)
	case <-r.stopped:
		writeSSEEvent(w, flusher, "server_stopped", InboundEvent{Source: r.name, Signal: "ServerStopped"})
	case <-time.After(r.awaitTimeout()):
		writeSSEEvent(w, flusher, "timeout", InboundEvent{Source: r.name, Signal: "AwaitTimedOut"})
	}
}

func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, eventType string, event InboundEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
	flusher.Flush()
}

func readRequestPayload(req *http.Request, endpoint Endpoint, maxBytes int) (map[string]interface{}, error) {
	payload := map[string]interface{}{}
	if err := addQueryValues(payload, endpoint.Request.Query, req.URL.Query()); err != nil {
		return nil, err
	}
	if err := addHeaderValues(payload, endpoint.Request.Headers, req.Header); err != nil {
		return nil, err
	}
	if maxBytes > 0 {
		req.Body = http.MaxBytesReader(nil, req.Body, int64(maxBytes))
	}
	if len(endpoint.Request.BodySchema) == 0 {
		return payload, nil
	}
	body := map[string]interface{}{}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, err
	}
	if err := validateBodySchema(endpoint.Request.BodySchema, body); err != nil {
		return nil, err
	}
	payload["body"] = body
	for key, value := range body {
		payload[key] = value
	}
	return payload, nil
}

func addPathValues(payload map[string]interface{}, schema map[string]interface{}, vars map[string]string) error {
	path := map[string]interface{}{}
	for name, value := range vars {
		typed, err := validateStringValue("path param", name, schema[name], value)
		if err != nil {
			return err
		}
		path[name] = typed
		payload[name] = typed
	}
	payload["path"] = path
	return nil
}

func addQueryValues(payload map[string]interface{}, schema map[string]interface{}, values map[string][]string) error {
	query := map[string]interface{}{}
	for name, raw := range values {
		if _, ok := schema[name]; !ok {
			return fmt.Errorf("query param %q is not declared", name)
		}
		typed, err := validateStringValue("query param", name, schema[name], firstValue(raw))
		if err != nil {
			return err
		}
		query[name] = typed
		payload[name] = typed
	}
	payload["query"] = query
	return nil
}

func addHeaderValues(payload map[string]interface{}, schema map[string]interface{}, values http.Header) error {
	headers := map[string]interface{}{}
	for name, raw := range values {
		field := strings.ToLower(name)
		spec, declared := lookupHeaderSchema(schema, field)
		if !declared {
			if allowedUndeclaredHeaders[field] {
				continue
			}
			return fmt.Errorf("header %q is not declared", field)
		}
		typed, err := validateStringValue("header", field, spec, firstValue(raw))
		if err != nil {
			return err
		}
		headers[field] = typed
		payload[field] = typed
	}
	payload["headers"] = headers
	return nil
}

func lookupHeaderSchema(schema map[string]interface{}, field string) (interface{}, bool) {
	for name, spec := range schema {
		if strings.EqualFold(name, field) {
			return spec, true
		}
	}
	return nil, false
}

func firstValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func validateStringValue(kind, name string, spec interface{}, value string) (interface{}, error) {
	rules, _ := spec.(map[string]interface{})
	switch want, _ := rules["type"].(string); want {
	case "", "string":
		return value, nil
	case "integer":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("%s %q must be integer", kind, name)
		}
		return parsed, nil
	case "number":
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("%s %q must be number", kind, name)
		}
		return parsed, nil
	case "boolean":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("%s %q must be boolean", kind, name)
		}
		return parsed, nil
	default:
		return value, nil
	}
}

func validateBodySchema(schema map[string]interface{}, payload map[string]interface{}) error {
	props, _ := schema["properties"].(map[string]interface{})
	required, _ := schema["required"].([]interface{})
	for _, raw := range required {
		field, _ := raw.(string)
		if _, ok := payload[field]; !ok {
			return fmt.Errorf("body field %q is required", field)
		}
	}
	for field, spec := range props {
		if err := validateJSONType(field, spec, payload[field]); err != nil {
			return err
		}
	}
	return nil
}

func validateJSONType(field string, spec interface{}, value interface{}) error {
	if value == nil {
		return nil
	}
	rules, _ := spec.(map[string]interface{})
	want, _ := rules["type"].(string)
	if want == "" || jsonTypeMatches(want, value) {
		return nil
	}
	return fmt.Errorf("body field %q must be %s", field, want)
}

func jsonTypeMatches(want string, value interface{}) bool {
	switch want {
	case "string":
		_, ok := value.(string)
		return ok
	case "number", "integer":
		_, ok := value.(float64)
		return ok
	case "object":
		_, ok := value.(map[string]interface{})
		return ok
	default:
		return true
	}
}

func writeRequestError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}

func writeJSON(w http.ResponseWriter, status int, payload map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func serverResponseOutput(mapping ResponseMapping, payload map[string]interface{}) map[string]interface{} {
	if len(mapping.Output) == 0 {
		return map[string]interface{}{"handled": true}
	}
	out := map[string]interface{}{}
	for name, selector := range mapping.Output {
		out[name] = responseValue(selector, payload)
	}
	redactMappedOutput(out, mapping.Redact)
	return out
}

func responseValue(selector string, payload map[string]interface{}) interface{} {
	switch selector {
	case "true":
		return true
	case "false":
		return false
	}
	if strings.HasPrefix(selector, "$.") {
		return payload[strings.TrimPrefix(selector, "$.")]
	}
	return selector
}

func redactServerPayload(payload map[string]interface{}, selectors []string) {
	for _, selector := range selectors {
		field := selectorField(selector)
		switch {
		case strings.HasPrefix(selector, "body."):
			redactNested(payload["body"], field)
		case strings.HasPrefix(selector, "$."):
			redactNested(payload["body"], field)
		case strings.HasPrefix(selector, "headers."):
			redactNested(payload["headers"], field)
		case strings.HasPrefix(selector, "query."):
			redactNested(payload["query"], field)
		}
		if field != "" {
			payload[field] = "[REDACTED]"
		}
	}
}

func redactMappedOutput(output map[string]interface{}, selectors []string) {
	for _, selector := range selectors {
		if field := selectorField(selector); field != "" {
			output[field] = "[REDACTED]"
		}
	}
}

func selectorField(selector string) string {
	switch {
	case strings.HasPrefix(selector, "body."):
		return strings.TrimPrefix(selector, "body.")
	case strings.HasPrefix(selector, "$."):
		return strings.TrimPrefix(selector, "$.")
	case strings.HasPrefix(selector, "headers."):
		return strings.TrimPrefix(selector, "headers.")
	case strings.HasPrefix(selector, "query."):
		return strings.TrimPrefix(selector, "query.")
	default:
		return ""
	}
}

func matchPath(template, path string) (map[string]string, bool) {
	want := pathSegments(template)
	got := pathSegments(path)
	if len(want) != len(got) {
		return nil, false
	}
	return matchSegments(want, got)
}

func matchSegments(want, got []string) (map[string]string, bool) {
	vars := map[string]string{}
	for i, segment := range want {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			vars[strings.Trim(segment, "{}")] = got[i]
			continue
		}
		if segment != got[i] {
			return nil, false
		}
	}
	return vars, true
}

func pathSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func signalFromRequest(req *http.Request, payload map[string]interface{}) string {
	if signal := req.URL.Query().Get("signal"); signal != "" {
		return signal
	}
	signal, _ := payload["signal"].(string)
	return signal
}

func allowedSignal(signal string, allowed []string) bool {
	for _, candidate := range allowed {
		if signal == candidate {
			return true
		}
	}
	return false
}

func (r *serverRuntime) stateOutput() map[string]interface{} {
	return map[string]interface{}{"server": r.name, "address": r.listener.Addr().String()}
}

func (r *serverRuntime) metadataOutput() map[string]interface{} {
	endpoints := make([]string, 0, len(r.def.Server.Endpoints))
	for name := range r.def.Server.Endpoints {
		endpoints = append(endpoints, name)
	}
	return map[string]interface{}{"server": r.name, "endpoints": endpoints}
}
