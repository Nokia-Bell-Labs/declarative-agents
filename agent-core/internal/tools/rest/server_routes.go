// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
)

const (
	bindingEmitSignal       = "emit_signal"
	bindingReadState        = "read_state"
	bindingInvokeHandler    = "invoke_handler"
	bindingStreamEvents     = "stream_events"
	bindingHealth           = "health"
	bindingStaticMetadata   = "static_metadata"
	bindingMachineRequest   = "machine_request"
	bindingSignalSource     = "signal_source"
	bindingLifecycleControl = "lifecycle_control"
	bindingMonitorProxy     = "monitor_proxy"
	bindingMock             = "mock"
	bindingMockLog          = "mock_log"
)

const (
	// lifecycleExitRouteName is the reserved endpoint name under which agent-core
	// injects the canonical lifecycle-control exit route, chosen to match the
	// route filter existing control awaits already use (GH-1264).
	lifecycleExitRouteName = "exit"
	// lifecycleExitPath is the canonical lifecycle-control exit path.
	lifecycleExitPath = "/api/lifecycle/exit"
	// lifecycleExitSignal is the grammar signal the injected exit endpoint emits.
	lifecycleExitSignal = "ExitRequested"
	// lifecycleActionEnqueueSignal names the fixed-signal enqueue mode used by
	// the injected endpoint.
	lifecycleActionEnqueueSignal = "enqueue_signal"
)

// handledServerBindings is the closed set of endpoint bindings handleEndpoint
// dispatches. A binding outside this set falls to the 501 default handler, so
// config validation rejects it up front (validateEndpoint) rather than letting
// --validate-config approve a route that can only fail at runtime (#510). This
// set is the single source of truth; keep it in sync with the switch in
// handleEndpoint.
var handledServerBindings = map[string]bool{
	bindingEmitSignal:       true,
	bindingDynamicSignal:    true,
	bindingLifecycleControl: true,
	bindingReadState:        true,
	bindingInvokeHandler:    true,
	bindingStreamEvents:     true,
	bindingHealth:           true,
	bindingStaticMetadata:   true,
	bindingMachineRequest:   true,
	bindingSignalSource:     true,
	bindingStaticAssets:     true,
	bindingRedirect:         true,
	bindingMonitorProxy:     true,
	bindingMock:             true,
	bindingMockLog:          true,
}

var allowedUndeclaredHeaders = map[string]bool{
	"accept": true, "accept-encoding": true, "accept-language": true,
	"cache-control": true, "connection": true,
	"content-length": true, "content-type": true,
	"cookie": true, "dnt": true,
	"sec-gpc":           true,
	"host":              true,
	"if-modified-since": true, "if-none-match": true,
	"origin": true,
	"pragma": true, "priority": true,
	"range":                       true,
	"referer":                     true,
	"sec-ch-prefers-color-scheme": true,
	"sec-ch-ua":                   true, "sec-ch-ua-arch": true, "sec-ch-ua-bitness": true,
	"sec-ch-ua-full-version": true, "sec-ch-ua-full-version-list": true,
	"sec-ch-ua-mobile": true, "sec-ch-ua-model": true, "sec-ch-ua-platform": true,
	"sec-ch-ua-wow64": true,
	"sec-fetch-dest":  true, "sec-fetch-mode": true, "sec-fetch-site": true,
	"sec-fetch-storage-access": true, "sec-fetch-user": true,
	"upgrade-insecure-requests": true,
	"user-agent":                true,
	"x-request-id":              true,
	"traceparent":               true,
	"tracestate":                true,
	"forwarded":                 true,
	"x-forwarded-for":           true,
	"x-forwarded-host":          true,
	"x-forwarded-port":          true,
	"x-forwarded-prefix":        true,
	"x-forwarded-proto":         true,
	"x-forwarded-server":        true,
	"x-real-ip":                 true,
}

func (r *serverRuntime) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	route := r.matchEndpoint(req)
	if !route.pathFound {
		http.NotFound(w, req)
		return
	}
	// The most specific matching path owns the resource, so a method it does
	// not declare is refused here even when a less specific catch-all would
	// accept it (srd029 R6.8).
	if route.name == "" {
		w.Header().Set("Allow", strings.Join(route.allowed, ", "))
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name, endpoint, vars := route.name, route.endpoint, route.vars
	// A mock endpoint is a mount point, not one declared route: its fixture
	// decides which methods and paths answer, so it precedes the declared-method
	// check and reads the request body directly (srd039 R2.1).
	if endpoint.Binding == bindingMock {
		r.serveMock(w, req)
		return
	}
	req, authorized := r.authorizeEndpoint(w, req, endpoint)
	if !authorized {
		return
	}
	// monitor_proxy forwards arbitrary monitor query strings to a declared upstream
	// and reads the request directly, so it bypasses declared-query/body validation.
	if endpoint.Binding == bindingMonitorProxy {
		r.handleEndpoint(w, req, name, endpoint, nil)
		return
	}
	payload, err := r.readEndpointPayload(req, endpoint, vars)
	if err != nil {
		r.writeEndpointRequestError(w, req, name, endpoint, err)
		return
	}
	r.handleEndpoint(w, req, name, endpoint, payload)
}

func (r *serverRuntime) readEndpointPayload(
	req *http.Request,
	endpoint restdef.Endpoint,
	vars map[string]string,
) (map[string]interface{}, error) {
	payload, err := readRequestPayload(req, endpoint, r.def.Limits.MaxRequestBytes)
	if err != nil {
		return nil, err
	}
	if err := addPathValues(payload, endpoint.Request.Path, vars); err != nil {
		return nil, err
	}
	return payload, nil
}

func (r *serverRuntime) writeEndpointRequestError(
	w http.ResponseWriter,
	req *http.Request,
	name string,
	endpoint restdef.Endpoint,
	err error,
) {
	if endpoint.Binding == bindingSignalSource {
		r.handleSignalSourceValidationError(w, req, name, endpoint, err)
		return
	}
	writeRequestError(w, err)
}

func (r *serverRuntime) authorizeEndpoint(
	w http.ResponseWriter, req *http.Request, endpoint restdef.Endpoint,
) (*http.Request, bool) {
	if endpoint.Binding != bindingLifecycleControl {
		return req, true
	}
	ref := endpoint.LifecycleControl.RequireAuthRef
	if err := authorizeLifecycleRequest(req, r.def, ref); err != nil {
		writeLifecycleAuthError(w, err)
		return req, false
	}
	return withoutLifecycleCredential(req, r.def, ref), true
}

type routeMatch struct {
	name     string
	endpoint restdef.Endpoint
	vars     map[string]string
	score    int
	catchAll bool
}

// routeSelection is the outcome of matching one request. pathFound with an
// empty name means the most specific path declares no endpoint for the method,
// and allowed lists the methods it does declare.
type routeSelection struct {
	pathFound bool
	name      string
	endpoint  restdef.Endpoint
	vars      map[string]string
	allowed   []string
}

// matchEndpoint selects by path specificity first and method second (srd029
// R6.8): only the most specific matching paths are candidates, and among them
// the endpoint declaring the request's method wins. A mock mount answers every
// method, so it stands in when no candidate declares the method.
func (r *serverRuntime) matchEndpoint(req *http.Request) routeSelection {
	var top []routeMatch
	for name, endpoint := range r.def.Server.Endpoints {
		vars, ok := matchPath(endpoint.Path, req.URL.Path)
		if !ok {
			continue
		}
		candidate := routeMatch{
			name: name, endpoint: endpoint, vars: vars,
			score:    literalSegmentScore(endpoint.Path),
			catchAll: pathHasCatchAll(endpoint.Path),
		}
		switch {
		case len(top) == 0 || moreSpecificPath(candidate, top[0]):
			top = []routeMatch{candidate}
		case !moreSpecificPath(top[0], candidate):
			top = append(top, candidate)
		}
	}
	if len(top) == 0 {
		return routeSelection{}
	}
	sort.Slice(top, func(i, j int) bool { return top[i].name < top[j].name })
	selected := func(match routeMatch) routeSelection {
		return routeSelection{pathFound: true, name: match.name, endpoint: match.endpoint, vars: match.vars}
	}
	for _, match := range top {
		if match.endpoint.Method == req.Method {
			return selected(match)
		}
	}
	for _, match := range top {
		if match.endpoint.Binding == bindingMock {
			return selected(match)
		}
	}
	return routeSelection{pathFound: true, allowed: declaredMethods(top)}
}

// moreSpecificPath reports whether candidate's path outranks best's: more
// literal segments win, and on a tie an exact route beats a trailing catch-all.
// Paths that rank equally are one candidate set, split by method.
func moreSpecificPath(candidate, best routeMatch) bool {
	if candidate.score != best.score {
		return candidate.score > best.score
	}
	return candidate.catchAll != best.catchAll && !candidate.catchAll
}

func declaredMethods(matches []routeMatch) []string {
	seen := map[string]bool{}
	var methods []string
	for _, match := range matches {
		if method := match.endpoint.Method; method != "" && !seen[method] {
			seen[method] = true
			methods = append(methods, method)
		}
	}
	sort.Strings(methods)
	return methods
}

func literalSegmentScore(path string) int {
	score := 0
	for _, seg := range pathSegments(path) {
		if !strings.HasPrefix(seg, "{") {
			score++
		}
	}
	return score
}

func pathHasCatchAll(path string) bool {
	segs := pathSegments(path)
	if len(segs) == 0 {
		return false
	}
	_, ok := catchAllParam(segs[len(segs)-1])
	return ok
}

func (r *serverRuntime) handleEndpoint(
	w http.ResponseWriter,
	req *http.Request,
	name string,
	endpoint restdef.Endpoint,
	payload map[string]interface{},
) {
	switch endpoint.Binding {
	case bindingEmitSignal:
		r.enqueueSignal(w, req, name, endpoint.Signal, payload, endpoint.Response.Redact)
	case bindingDynamicSignal:
		r.enqueueDynamicSignal(w, req, name, endpoint, payload)
	case bindingLifecycleControl:
		r.enqueueLifecycleControl(w, req, name, endpoint, payload)
	case bindingReadState:
		r.writeReadState(w, name, endpoint)
	case bindingInvokeHandler:
		r.invokeHandler(w, req, name, endpoint, payload)
	case bindingStreamEvents:
		r.streamEvents(w, endpoint)
	case bindingHealth:
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
	case bindingStaticMetadata:
		r.writeStaticMetadata(w, endpoint)
	case bindingMachineRequest:
		r.handleMachineRequest(w, req, name, endpoint, payload)
	case bindingSignalSource:
		r.handleSignalSource(w, req, name, endpoint, payload)
	case bindingStaticAssets:
		r.serveStaticAssets(w, req, endpoint)
	case bindingRedirect:
		r.writeRedirect(w, endpoint)
	case bindingMonitorProxy:
		r.proxyMonitor(w, req, endpoint)
	case bindingMockLog:
		r.writeMockLog(w)
	default:
		http.Error(w, "endpoint binding is not implemented", http.StatusNotImplemented)
	}
}

// monitorProxyClient forwards short monitor reads and SSE reconnect requests to a
// declared upstream. It does not follow redirects, so a compromised upstream
// cannot bounce the proxy elsewhere.
var monitorProxyClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// proxyMonitor reverse-proxies a GET to the declared upstream monitor for the
// {agent} path param, so a browser SPA served by this server can read every mesh
// agent's monitor state and SSE stream through one origin. Only the declared
// upstreams are reachable; the caller supplies the agent key and the path suffix,
// never a host, and only GET is forwarded so the proxy cannot drive mutations.
func (r *serverRuntime) proxyMonitor(w http.ResponseWriter, req *http.Request, endpoint restdef.Endpoint) {
	cfg := endpoint.MonitorProxy
	if cfg == nil {
		http.Error(w, "monitor_proxy is not configured", http.StatusInternalServerError)
		return
	}
	if req.Method != http.MethodGet {
		http.Error(w, "monitor_proxy forwards GET only", http.StatusMethodNotAllowed)
		return
	}
	vars, ok := matchPath(endpoint.Path, req.URL.Path)
	if !ok {
		http.NotFound(w, req)
		return
	}
	agent := ""
	suffix := ""
	for _, seg := range pathSegments(endpoint.Path) {
		if n, ok := catchAllParam(seg); ok {
			suffix = vars[n]
		} else if n, ok := simpleParam(seg); ok && agent == "" {
			agent = vars[n]
		}
	}
	base, ok := cfg.Upstreams[agent]
	if !ok {
		http.Error(w, "unknown monitor agent", http.StatusNotFound)
		return
	}
	target := strings.TrimRight(base, "/") + "/" + strings.TrimLeft(suffix, "/")
	if req.URL.RawQuery != "" {
		target += "?" + req.URL.RawQuery
	}
	upstreamReq, err := http.NewRequestWithContext(req.Context(), http.MethodGet, target, nil)
	if err != nil {
		http.Error(w, "monitor_proxy target invalid", http.StatusBadGateway)
		return
	}
	if accept := req.Header.Get("Accept"); accept != "" {
		upstreamReq.Header.Set("Accept", accept)
	}
	resp, err := monitorProxyClient.Do(upstreamReq)
	if err != nil {
		http.Error(w, "monitor upstream unreachable", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			return
		}
	}
}

func (r *serverRuntime) writeRedirect(w http.ResponseWriter, endpoint restdef.Endpoint) {
	cfg := endpoint.Redirect
	if cfg == nil {
		http.Error(w, "redirect is not configured", http.StatusInternalServerError)
		return
	}
	status := cfg.Status
	if status == 0 {
		status = http.StatusFound
	}
	w.Header().Set("Location", cfg.Location)
	w.WriteHeader(status)
}

func (r *serverRuntime) serveStaticAssets(w http.ResponseWriter, req *http.Request, endpoint restdef.Endpoint) {
	cfg := endpoint.StaticAssets
	if cfg == nil {
		http.Error(w, "static_assets is not configured", http.StatusInternalServerError)
		return
	}
	vars, ok := matchPath(endpoint.Path, req.URL.Path)
	if !ok {
		http.NotFound(w, req)
		return
	}
	rel := ""
	for _, seg := range pathSegments(endpoint.Path) {
		if n, ok := catchAllParam(seg); ok {
			rel = vars[n]
			break
		}
	}
	idx := cfg.Index
	if idx == "" {
		idx = "index.html"
	}
	f, info, err := openStaticAssetFile(http.Dir(filepath.Clean(cfg.Root)), rel, idx, cfg.SPA)
	if err != nil {
		http.NotFound(w, req)
		return
	}
	defer func() { _ = f.Close() }()
	// Without a cache policy browsers cache heuristically on Last-Modified,
	// and a SPA keeps running its old bundle after a redeploy until a hard
	// reload. no-cache forces revalidation, which ServeContent answers with
	// cheap 304s, so a normal reload always runs the deployed page (GH-1939).
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, req, info.Name(), info.ModTime(), f)
}

func openStaticAssetFile(d http.Dir, rel, idx string, spa bool) (http.File, os.FileInfo, error) {
	key := strings.TrimPrefix(path.Clean("/"+rel), "/")
	if key == "." || key == "" {
		return openStaticLeafFile(d, idx)
	}
	f, err := d.Open(key)
	if err != nil {
		if spa {
			return openStaticLeafFile(d, idx)
		}
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		if spa {
			return openStaticLeafFile(d, idx)
		}
		return nil, nil, err
	}
	if !info.IsDir() {
		return f, info, nil
	}
	_ = f.Close()
	if f2, info2, err := openStaticLeafFile(d, path.Join(key, idx)); err == nil {
		return f2, info2, nil
	}
	if spa {
		return openStaticLeafFile(d, idx)
	}
	return nil, nil, os.ErrNotExist
}

func openStaticLeafFile(d http.Dir, name string) (http.File, os.FileInfo, error) {
	f, err := d.Open(name)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	if info.IsDir() {
		_ = f.Close()
		return nil, nil, os.ErrNotExist
	}
	return f, info, nil
}

func (r *serverRuntime) invokeHandler(
	w http.ResponseWriter,
	req *http.Request,
	name string,
	endpoint restdef.Endpoint,
	payload map[string]interface{},
) {
	if endpoint.Signal != "" {
		redactServerPayload(payload, endpoint.Response.Redact)
		event := inboundEvent(r.name, name, req.Method, endpoint.Signal, payload, req.Header.Get("X-Request-ID"))
		if !r.offerEvent(event, endpoint.Queue) {
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
	endpoint restdef.Endpoint,
	payload map[string]interface{},
) {
	signal := dynamicSignalFromRequest(req, payload, endpoint)
	if !allowedSignal(signal, endpoint.AllowedSignals) {
		http.Error(w, "signal is not allowed", http.StatusBadRequest)
		return
	}
	r.enqueueSignal(w, req, name, signal, payload, endpoint.Response.Redact)
}

func dynamicSignalFromRequest(req *http.Request, payload map[string]interface{}, endpoint restdef.Endpoint) string {
	if endpoint.SignalField == "" {
		return signalFromRequest(req, payload)
	}
	value, ok := payloadPathString(payload, endpoint.SignalField)
	if !ok {
		return ""
	}
	if len(endpoint.SignalMapping) == 0 {
		return value
	}
	return endpoint.SignalMapping[value]
}

func payloadPathString(payload map[string]interface{}, field string) (string, bool) {
	var current interface{} = payload
	for _, part := range strings.Split(field, ".") {
		object, ok := current.(map[string]interface{})
		if !ok {
			return "", false
		}
		current, ok = object[part]
		if !ok {
			return "", false
		}
	}
	value, ok := current.(string)
	return value, ok
}

func (r *serverRuntime) enqueueLifecycleControl(
	w http.ResponseWriter,
	req *http.Request,
	name string,
	endpoint restdef.Endpoint,
	payload map[string]interface{},
) {
	signal := lifecycleSignal(endpoint)
	if endpoint.LifecycleControl.Action == "inject_signal" {
		signal = signalFromRequest(req, payload)
		if !allowedSignal(signal, endpoint.LifecycleControl.AllowedSignals) {
			http.Error(w, "signal is not allowed", http.StatusBadRequest)
			return
		}
	}
	if err := validateLifecyclePayload(endpoint.LifecycleControl, payload); err != nil {
		writeRequestError(w, err)
		return
	}
	r.enqueueSignal(w, req, name, signal, payload, endpoint.Response.Redact)
}

func writeLifecycleAuthError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if authErr, ok := err.(lifecycleAuthError); ok {
		status = authErr.status
	}
	http.Error(w, err.Error(), status)
}

func lifecycleSignal(endpoint restdef.Endpoint) string {
	if endpoint.LifecycleControl.Signal != "" {
		return endpoint.LifecycleControl.Signal
	}
	return endpoint.Signal
}

func validateLifecyclePayload(control restdef.LifecycleControl, payload map[string]interface{}) error {
	if len(control.TargetSchema) == 0 {
		return nil
	}
	body, _ := payload["body"].(map[string]interface{})
	return validateBodySchema(control.TargetSchema, body)
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
	queue := r.def.Server.Endpoints[route].Queue
	if len(queue.PayloadShape) > 0 {
		if err := validateBodySchema(queue.PayloadShape, payload); err != nil {
			writeRequestError(w, err)
			return
		}
	}
	redactServerPayload(payload, redact)
	event := inboundEvent(r.name, route, req.Method, signal, payload, req.Header.Get("X-Request-ID"))
	event.Queue = queueName(queue, r.def.Server.Queue)
	if !r.offerEvent(event, queue) {
		http.Error(w, "REST server queue is full", http.StatusTooManyRequests)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]interface{}{"accepted": true, "signal": signal})
}

func queueName(endpoint, server restdef.QueueConfig) string {
	if endpoint.Name != "" {
		return endpoint.Name
	}
	return server.Name
}

func (r *serverRuntime) offerEvent(event InboundEvent, queue restdef.QueueConfig) bool {
	select {
	case r.queue <- event:
		return true
	default:
		return r.handleQueueOverflow(event, queue)
	}
}

func (r *serverRuntime) handleQueueOverflow(event InboundEvent, queue restdef.QueueConfig) bool {
	switch queueOverflow(queue, r.def.Server.Queue) {
	case queueOverflowDropNewest:
		r.incrementDroppedEvents()
		return true
	case queueOverflowDropOldest:
		return r.dropOldestAndOffer(event)
	default:
		r.incrementDroppedEvents()
		return false
	}
}

func queueOverflow(endpoint, server restdef.QueueConfig) string {
	if endpoint.Overflow != "" {
		return endpoint.Overflow
	}
	if server.Overflow != "" {
		return server.Overflow
	}
	return queueOverflowReject
}

func (r *serverRuntime) dropOldestAndOffer(event InboundEvent) bool {
	select {
	case <-r.queue:
		r.incrementDroppedEvents()
	default:
	}
	select {
	case r.queue <- event:
		return true
	default:
		r.incrementDroppedEvents()
		return false
	}
}

func (r *serverRuntime) incrementDroppedEvents() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.droppedEvents++
}

func inboundEvent(source, route, method, signal string, payload map[string]interface{}, requestID string) InboundEvent {
	return InboundEvent{
		Source: source, Route: route, Method: method,
		Signal: signal, Payload: payload, RequestID: requestID,
	}
}

func (r *serverRuntime) streamEvents(w http.ResponseWriter, endpoint restdef.Endpoint) {
	if endpoint.MonitorView != "" {
		r.streamMonitorEvents(w)
		return
	}
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

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func serverResponseOutput(mapping restdef.ResponseMapping, payload map[string]interface{}) map[string]interface{} {
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
		value, _ := resolveResultSelector(selector, payload)
		return value
	}
	return selector
}

func matchPath(template, path string) (map[string]string, bool) {
	want := pathSegments(template)
	got := pathSegments(path)
	if !pathSegmentCountsMatch(want, got) {
		return nil, false
	}
	return matchSegments(want, got)
}

func matchSegments(want, got []string) (map[string]string, bool) {
	vars := map[string]string{}
	for i, segment := range want {
		if name, ok := catchAllParam(segment); ok {
			vars[name] = strings.Join(got[i:], "/")
			return vars, true
		}
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

func pathSegmentCountsMatch(want, got []string) bool {
	if len(want) == 0 {
		return len(got) == 0
	}
	if _, ok := catchAllParam(want[len(want)-1]); ok {
		return len(got) >= len(want)-1
	}
	return len(want) == len(got)
}

func catchAllParam(segment string) (string, bool) {
	name, ok := strings.CutSuffix(strings.Trim(segment, "{}"), "...")
	return name, ok && name != "" && strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")
}

// simpleParam returns the name of a non-catch-all "{name}" path segment.
func simpleParam(segment string) (string, bool) {
	if !strings.HasPrefix(segment, "{") || !strings.HasSuffix(segment, "}") || strings.HasSuffix(segment, "...}") {
		return "", false
	}
	name := strings.Trim(segment, "{}")
	return name, name != ""
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
