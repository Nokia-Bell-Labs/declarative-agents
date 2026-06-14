// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	bindingEmitSignal     = "emit_signal"
	bindingReadState      = "read_state"
	bindingHealth         = "health"
	bindingStaticMetadata = "static_metadata"
)

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
	for key, value := range vars {
		payload[key] = value
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
		r.enqueueSignal(w, req, name, endpoint.Signal, payload)
	case bindingDynamicSignal:
		r.enqueueDynamicSignal(w, req, name, endpoint, payload)
	case bindingReadState:
		writeJSON(w, http.StatusOK, r.stateOutput())
	case bindingHealth:
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
	case bindingStaticMetadata:
		writeJSON(w, http.StatusOK, r.metadataOutput())
	default:
		http.Error(w, "endpoint binding is not implemented", http.StatusNotImplemented)
	}
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
	r.enqueueSignal(w, req, name, signal, payload)
}

func (r *serverRuntime) enqueueSignal(
	w http.ResponseWriter,
	req *http.Request,
	route string,
	signal string,
	payload map[string]interface{},
) {
	if signal == "" {
		http.Error(w, "endpoint signal is not configured", http.StatusInternalServerError)
		return
	}
	event := InboundEvent{
		Source: r.name, Route: route, Method: req.Method,
		Signal: signal, Payload: payload, RequestID: req.Header.Get("X-Request-ID"),
	}
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

func readRequestPayload(req *http.Request, endpoint Endpoint, maxBytes int) (map[string]interface{}, error) {
	if maxBytes > 0 {
		req.Body = http.MaxBytesReader(nil, req.Body, int64(maxBytes))
	}
	payload := map[string]interface{}{}
	if len(endpoint.Request.BodySchema) == 0 {
		return payload, nil
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, validateBodySchema(endpoint.Request.BodySchema, payload)
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
