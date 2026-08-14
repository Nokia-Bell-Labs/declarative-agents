// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry/genai"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// Collection indexes REST definitions loaded for one profile.
type Collection struct {
	Clients          map[string]Client
	Servers          map[string]Server
	Auth             map[string]AuthProfile
	Limits           map[string]LimitProfile
	RetryPolicies    map[string]RetryPolicy
	ResponseMappings map[string]ResponseMapping
}

// ClientOperationResolver resolves trusted REST client operations.
type ClientOperationResolver interface {
	ResolveClientOperation(ClientToolConfig) (ClientOperationDefinition, error)
}

// ClientOperationDefinition is a resolved client operation and trusted policy.
type ClientOperationDefinition struct {
	RestRef          string
	Resource         string
	OperationName    string
	Client           Client
	Operation        Operation
	Auth             AuthProfile
	Limits           LimitProfile
	Retry            RetryPolicy
	ResponseMappings map[string]ResponseMapping
}

// ServerDefinition is a resolved server plus its referenced limit profile.
type ServerDefinition struct {
	Name                 string
	Server               Server
	Limits               LimitProfile
	Auth                 map[string]AuthProfile
	Credentials          CredentialResolver
	MachineRequestRunner MachineRequestRunner
	SignalSourceRunner   SignalSourceRunner
	Monitor              MonitorState
	RunID                string
}

// MonitorState provides read-only state for monitor REST endpoints.
type MonitorState struct {
	Store    *monitor.Store
	Recorder monitor.RuntimeRecorder
	Machine  *core.MachineSpec
	Tools    []catalog.ToolDef
	// CommandState backs the opt-in command_state view. It exposes the
	// redaction-cleared declared output of profile-named steps and is nil when no
	// live source is wired (srd033-monitor-rest-api R7).
	CommandState core.CommandStateSource
}

// MachineRequestRunner runs one request-scoped machine.
type MachineRequestRunner interface {
	RunMachineRequest(context.Context, MachineRequestRun) (MachineRequestResult, error)
}

// SignalSourceRunner admits into the host program, never a nested request machine.
type SignalSourceRunner interface {
	RequestSignal(context.Context, core.SignalEnvelope) core.SignalAdmission
}

// MachineRequestRun is the accepted HTTP request visible to a request machine.
type MachineRequestRun struct {
	Server          string                  `json:"server"`
	Route           string                  `json:"route"`
	Method          string                  `json:"method"`
	Path            string                  `json:"path"`
	RequestID       string                  `json:"request_id,omitempty"`
	Payload         map[string]interface{}  `json:"payload,omitempty"`
	Config          MachineRequest          `json:"-"`
	MonitorRecorder monitor.RuntimeRecorder `json:"-"`
	RunID           string                  `json:"-"`
	ConversationID  string                  `json:"-"`
}

// MachineRequestResult records the short-lived machine outcome.
type MachineRequestResult struct {
	Server         string                 `json:"server,omitempty"`
	Route          string                 `json:"route,omitempty"`
	Machine        string                 `json:"machine,omitempty"`
	TerminalSignal string                 `json:"terminal_signal"`
	Output         map[string]interface{} `json:"output,omitempty"`
	Run            core.RunResult         `json:"run"`
	// TraceID is the connected trace's id (hex), so the response caller can fetch
	// the turn's cross-agent trace waterfall from the trace backend (GH-312).
	TraceID string `json:"trace_id,omitempty"`
}

// NewCollection creates an empty REST definition collection.
func NewCollection() Collection {
	return Collection{
		Clients:          map[string]Client{},
		Servers:          map[string]Server{},
		Auth:             map[string]AuthProfile{},
		Limits:           map[string]LimitProfile{},
		RetryPolicies:    map[string]RetryPolicy{},
		ResponseMappings: map[string]ResponseMapping{},
	}
}

// Add merges a validated REST definition into the collection.
func (c Collection) Add(def Definition) error {
	for name, profile := range def.Auth {
		if _, exists := c.Auth[name]; exists {
			return fmt.Errorf("duplicate REST auth %q", name)
		}
		c.Auth[name] = profile
	}
	for name, limits := range def.Limits {
		if _, exists := c.Limits[name]; exists {
			return fmt.Errorf("duplicate REST limits %q", name)
		}
		c.Limits[name] = limits
	}
	for name, retry := range def.RetryPolicies {
		if _, exists := c.RetryPolicies[name]; exists {
			return fmt.Errorf("duplicate REST retry policy %q", name)
		}
		c.RetryPolicies[name] = retry
	}
	for name, mapping := range def.ResponseMappings {
		if _, exists := c.ResponseMappings[name]; exists {
			return fmt.Errorf("duplicate REST response mapping %q", name)
		}
		c.ResponseMappings[name] = mapping
	}
	for name, client := range def.Clients {
		if _, exists := c.Clients[name]; exists {
			return fmt.Errorf("duplicate REST client %q", name)
		}
		c.Clients[name] = client
	}
	for name, server := range def.Servers {
		if _, exists := c.Servers[name]; exists {
			return fmt.Errorf("duplicate REST server %q", name)
		}
		c.Servers[name] = server
	}
	return nil
}

// ResolveClientOperation returns a client operation with trusted policy config.
func (c Collection) ResolveClientOperation(cfg ClientToolConfig) (ClientOperationDefinition, error) {
	client, ok := c.Clients[cfg.RestRef]
	if !ok {
		return ClientOperationDefinition{}, fmt.Errorf("REST client %q is not defined", cfg.RestRef)
	}
	operation, err := c.resolveOperation(client, cfg)
	if err != nil {
		return ClientOperationDefinition{}, err
	}
	return ClientOperationDefinition{
		RestRef: cfg.RestRef, Resource: cfg.Resource, OperationName: cfg.Operation,
		Client: client, Operation: operation, Auth: c.Auth[client.AuthRef],
		Limits: c.Limits[client.LimitsRef], Retry: c.RetryPolicies[client.RetryRef],
		ResponseMappings: c.ResponseMappings,
	}, nil
}

func (c Collection) resolveOperation(client Client, cfg ClientToolConfig) (Operation, error) {
	if cfg.Resource == "" {
		return operationByName(client.Operations, cfg.Operation, "client "+cfg.RestRef)
	}
	resource, ok := client.Resources[cfg.Resource]
	if !ok {
		return Operation{}, fmt.Errorf("REST resource %q is not defined on client %q", cfg.Resource, cfg.RestRef)
	}
	operation, err := operationByName(resource.Operations, cfg.Operation, "resource "+cfg.Resource)
	if err != nil {
		return Operation{}, err
	}
	if operation.Path == "" {
		operation.Path = resource.Path
	}
	return operation, nil
}

// ResolveServer returns a server with the limit profile it references.
func (c Collection) ResolveServer(name string) (ServerDefinition, error) {
	server, ok := c.Servers[name]
	if !ok {
		return ServerDefinition{}, fmt.Errorf("REST server %q is not defined", name)
	}
	return ServerDefinition{
		Name: name, Server: server, Limits: c.Limits[server.LimitsRef], Auth: c.Auth,
	}, nil
}

func operationByName(operations map[string]Operation, name, owner string) (Operation, error) {
	operation, ok := operations[name]
	if !ok {
		return Operation{}, fmt.Errorf("REST operation %q is not defined on %s", name, owner)
	}
	return operation, nil
}

func machineRequestRunner(runner MachineRequestRunner) MachineRequestRunner {
	if runner != nil {
		return runner
	}
	return defaultMachineRequestRunner{}
}

type defaultMachineRequestRunner struct{}

func (defaultMachineRequestRunner) RunMachineRequest(
	ctx context.Context,
	req MachineRequestRun,
) (MachineRequestResult, error) {
	if req.Config.MachineSpec == nil {
		return MachineRequestResult{}, fmt.Errorf("machine_config_invalid: machine_request machine spec is not configured")
	}
	var last core.Result
	initialSignal := machineRequestInitialSignal(req.Config)
	seed := requestSeed(req, initialSignal)
	requestRecorder := machineRequestMonitorRecorder(req)
	params := core.LoopParams{
		MachineSpec:      req.Config.MachineSpec,
		Registry:         req.Config.Registry,
		InitFunc:         req.Config.InitFunc,
		ToolAction:       req.Config.ToolAction,
		InitialSignal:    initialSignal,
		InitialResult:    seed,
		InitialExecution: machineRequestSeedExecution(seed, nil),
		Budget:           req.Config.Budget,
		CommandTimeout:   parseDuration(req.Config.CommandTimeout, 0),
		// Run the request-scoped machine under a real tracer rooted at the
		// machine_request span in ctx, so its per-word dispatch and REST-client
		// spans export as children of that span (one connected trace) and the
		// engine injects a valid traceparent into TraceContextAware clients for
		// cross-agent propagation. The process provider is set globally by NewRoot;
		// without it this wraps the no-op global provider and behaves as before.
		Trace:           requestScopedTrace(ctx),
		RunID:           req.RunID,
		RequestID:       req.RequestID,
		ConversationID:  req.ConversationID,
		AgentName:       machineRequestAgentName(req),
		Directory:       ".",
		MonitorRecorder: requestRecorder,
		Hooks: core.LoopHooks{
			TerminalStatus: machineRequestTerminalStatus(req.Config),
			OnResult: func(rr core.RunResult, res core.Result) core.RunResult {
				last = res
				return rr
			},
		},
	}
	rr, err := core.Loop(params, ctx)
	if err != nil {
		return MachineRequestResult{}, fmt.Errorf("machine_config_invalid: %w", err)
	}
	if rr.Status == core.StatusCancelled {
		return MachineRequestResult{}, fmt.Errorf("machine_timeout: request machine timed out")
	}
	return machineRequestResult(req, rr, last)
}

func machineRequestMonitorRecorder(req MachineRequestRun) monitor.RuntimeRecorder {
	scoped, ok := req.MonitorRecorder.(monitor.TrustedEnvelopeRecorder)
	if !ok || req.Config.MachineSpec == nil {
		return req.MonitorRecorder
	}
	return scoped.WithTrustedEnvelope(machineRequestEnvelopePolicy(req.Config, req.RunID))
}

func machineRequestEnvelopePolicy(cfg MachineRequest, runID string) monitor.EnvelopePolicy {
	machine := cfg.MachineSpec
	tools := make(map[string]struct{}, len(machine.Transitions))
	states := stringValueSet(machine.States.Names()...)
	signals := stringValueSet(machine.Signals.Names()...)
	for _, state := range machine.TerminalStates {
		states[state] = struct{}{}
		signals[state] = struct{}{}
	}
	collectTransitionEnvelope(machine.Transitions, tools, states, signals)
	collectRequestSignals(cfg, signals)
	return monitor.EnvelopePolicy{
		RunID:     runID,
		ToolNames: sortedStringSet(tools),
		States:    sortedStringSet(states),
		Signals:   sortedStringSet(signals),
	}
}

func collectTransitionEnvelope(
	transitions []core.TransitionSpec,
	tools, states, signals map[string]struct{},
) {
	for _, transition := range transitions {
		states[transition.State] = struct{}{}
		states[transition.Next] = struct{}{}
		signals[transition.Signal] = struct{}{}
		if transition.Action != "" && transition.Action != "$tool" {
			tools[transition.Action] = struct{}{}
		}
		if transition.ForEach != nil {
			collectForEachEnvelope(*transition.ForEach, tools, signals)
		}
	}
}

func collectForEachEnvelope(spec core.ForEachSpec, tools, signals map[string]struct{}) {
	tools["for_each.join"] = struct{}{}
	for _, signal := range spec.ContinueOn {
		signals[signal] = struct{}{}
	}
	for _, signal := range spec.AbortOn {
		signals[signal] = struct{}{}
	}
	for _, signal := range []string{
		spec.Join.Signals.AllSuccess,
		spec.Join.Signals.Partial,
		spec.Join.Signals.Failed,
		spec.Join.Signals.Empty,
	} {
		signals[signal] = struct{}{}
	}
}

func collectRequestSignals(cfg MachineRequest, signals map[string]struct{}) {
	for signal := range cfg.Response.TerminalSignals {
		signals[signal] = struct{}{}
	}
	for signal := range cfg.Response.TerminalStates {
		signals[signal] = struct{}{}
	}
	signals[string(machineRequestInitialSignal(cfg))] = struct{}{}
	for _, signal := range []core.Signal{
		core.CommandError, core.ToolFailed, core.BudgetExhausted,
	} {
		signals[string(signal)] = struct{}{}
	}
}

func stringValueSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = struct{}{}
		}
	}
	return set
}

func sortedStringSet(set map[string]struct{}) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		if value != "" {
			values = append(values, value)
		}
	}
	sort.Strings(values)
	return values
}

// requestScopedTrace wraps the process tracer provider rooted at the
// machine_request span in ctx, so request-scoped spans join the connected trace.
// When the global provider is the no-op default (telemetry not initialized), the
// wrapped tracer is a no-op and dispatch behaves exactly as it did under NoopTracer.
func requestScopedTrace(ctx context.Context) tracing.Tracer {
	return telemetry.TraceAdapter{T: telemetry.NewTraceFromProvider(otel.GetTracerProvider(), "agent-core/machine_request", ctx)}
}

func machineRequestAgentName(req MachineRequestRun) string {
	if req.RequestID != "" {
		return "machine_request:" + req.RequestID
	}
	if req.Server != "" && req.Route != "" {
		return "machine_request:" + req.Server + "/" + req.Route
	}
	return "machine_request"
}

// machineRequestTerminalStatus uses declared machine outcome policy first.
// HTTP response mappings remain a compatibility fallback for older machines.
//
// The terminal-state map answers this directly. The terminal-signal map is
// consulted next only for the machines that name a state after the signal
// reaching it, where the two keys coincide; that lookup predates the state map
// and stays so those configurations keep their classification (GH-615).
func machineRequestTerminalStatus(cfg MachineRequest) func(core.State) core.RunStatus {
	return func(state core.State) core.RunStatus {
		if status, ok := core.DeclaredTerminalStatus(cfg.MachineSpec, state); ok {
			return status
		}
		if mapping, ok := cfg.Response.TerminalStates[string(state)]; ok {
			return runStatusForHTTP(mapping.Status)
		}
		if mapping, ok := cfg.Response.TerminalSignals[string(state)]; ok {
			return runStatusForHTTP(mapping.Status)
		}
		return core.TerminalStatusForState(cfg.MachineSpec, state)
	}
}

// runStatusForHTTP reads a mapped status code as run success or failure. An
// unset status defaults to 200 at write time, so it reads as success here.
func runStatusForHTTP(status int) core.RunStatus {
	if status == 0 || (status >= 200 && status < 400) {
		return core.StatusSucceeded
	}
	return core.StatusFailed
}

func machineRequestInitialSignal(cfg MachineRequest) core.Signal {
	if cfg.InitialSignal == "" {
		return core.Seed
	}
	return core.Signal(cfg.InitialSignal)
}

// requestSeed builds the Result that seeds a machine_request run's first word.
// The mapped request input is exposed under a "parameters" key, the key both
// filesystem words (extractStringParam) and REST-client words (runtimeParams)
// read, so either can be the first word. The transport authority (method,
// server, route) is omitted: a request-machine word must not see it, and a
// top-level "method" would trip the REST runtime-input authority guard. The URL
// path and request id are kept because they name no transport authority and let
// adapters derive request context (srd030 R2, R3.2).
func requestSeed(req MachineRequestRun, signal core.Signal) core.Result {
	payload := req.Payload
	if payload == nil {
		payload = map[string]interface{}{}
	}
	seed := map[string]interface{}{"parameters": payload}
	if req.Path != "" {
		seed["path"] = req.Path
	}
	if req.RequestID != "" {
		seed["request_id"] = req.RequestID
	}
	data, err := json.Marshal(seed)
	if err != nil {
		return core.Result{
			Signal:    signal,
			Output:    `{"parameters":{}}`,
			Redaction: machineRequestSeedRedaction(req.Config.Request),
		}
	}
	return core.Result{
		Signal:    signal,
		Output:    string(data),
		Redaction: machineRequestSeedRedaction(req.Config.Request),
	}
}

func machineRequestSeedRedaction(mapping MachineRequestMapping) core.OutputRedaction {
	paths := make([]core.OutputRedactionPath, 0, len(mapping.Sensitive))
	for _, name := range mapping.Sensitive {
		paths = append(paths, core.OutputRedactionPath{"parameters", name})
	}
	return core.OutputRedaction{
		Version: core.OutputRedactionVersion1,
		Paths:   paths,
	}
}

// machineRequestSeedExecution makes trusted request input addressable after
// intervening words as $from(seed).parameters.<field>. Existing execution wins
// on resume so the synthetic forward entry is never duplicated. DigestResult
// applies the same output-redaction boundary as loop dispatch (srd030 R3.8,
// srd038 R5).
func machineRequestSeedExecution(seed core.Result, existing core.Execution) core.Execution {
	if len(existing) > 0 {
		return existing
	}
	return core.Execution{{
		CommandName: "seed",
		Label:       "seed",
		Signal:      seed.Signal,
		Result:      core.DigestResult(seed),
	}}
}

func machineRequestResult(req MachineRequestRun, rr core.RunResult, last core.Result) (MachineRequestResult, error) {
	if last.Signal == "" {
		return MachineRequestResult{}, fmt.Errorf("response_missing: request machine produced no response signal")
	}
	output := map[string]interface{}{}
	if last.Output != "" {
		if err := json.Unmarshal([]byte(last.Output), &output); err != nil {
			// A terminal word that emits plain text rather than a JSON object (for
			// example invoke_llm, whose output is the raw model answer) is wrapped
			// under "output" so a response body selector can map $.output. JSON
			// object outputs unmarshal above unchanged (srd030 R4.3).
			output = map[string]interface{}{"output": last.Output}
		}
	}
	return MachineRequestResult{
		Server: req.Server, Route: req.Route, Machine: req.Config.MachineSpec.Name,
		TerminalSignal: string(last.Signal), Output: output, Run: rr,
	}, nil
}

func (r *serverRuntime) handleMachineRequest(
	w http.ResponseWriter,
	req *http.Request,
	name string,
	endpoint Endpoint,
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

func (r *serverRuntime) machineRequestTimeout(endpoint Endpoint) time.Duration {
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
	endpoint Endpoint,
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

func validateMachineResponseBody(mapping MachineResponseMapping, body map[string]interface{}) error {
	if len(mapping.Schema) == 0 {
		return nil
	}
	if err := validateBodySchema(mapping.Schema, body); err != nil {
		return fmt.Errorf("response_invalid: terminal response schema: %w", err)
	}
	return nil
}

func machineResponseBody(mapping MachineResponseMapping, result MachineRequestResult) map[string]interface{} {
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

func machineRequestPayload(mapping MachineRequestMapping, payload map[string]interface{}) map[string]interface{} {
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
