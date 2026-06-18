// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
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

// ServerResolver resolves trusted REST server definitions.
type ServerResolver interface {
	ResolveServer(name string) (ServerDefinition, error)
}

// DefinitionResolver composes client and server definition lookups.
type DefinitionResolver interface {
	ClientOperationResolver
	ServerResolver
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
	MachineRequestRunner MachineRequestRunner
	Monitor              MonitorState
}

// MonitorState provides read-only state for monitor REST endpoints.
type MonitorState struct {
	Store   *monitor.Store
	Machine *core.MachineSpec
	Tools   []catalog.ToolDef
}

// MachineRequestRunner runs one request-scoped machine.
type MachineRequestRunner interface {
	RunMachineRequest(context.Context, MachineRequestRun) (MachineRequestResult, error)
}

// MachineRequestRun is the accepted HTTP request visible to a request machine.
type MachineRequestRun struct {
	Server            string                 `json:"server"`
	Route             string                 `json:"route"`
	Method            string                 `json:"method"`
	Path              string                 `json:"path"`
	RequestID         string                 `json:"request_id,omitempty"`
	Payload           map[string]interface{} `json:"payload,omitempty"`
	Config            MachineRequest         `json:"-"`
	MonitorRecorder   monitor.RuntimeRecorder `json:"-"`
}

// MachineRequestResult records the short-lived machine outcome.
type MachineRequestResult struct {
	Server         string                 `json:"server,omitempty"`
	Route          string                 `json:"route,omitempty"`
	Machine        string                 `json:"machine,omitempty"`
	TerminalSignal string                 `json:"terminal_signal"`
	Output         map[string]interface{} `json:"output,omitempty"`
	Run            core.RunResult         `json:"run"`
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

// ClientOperation resolves a configured client operation.
func (c Collection) ClientOperation(cfg ClientToolConfig) (Operation, error) {
	resolved, err := c.ResolveClientOperation(cfg)
	if err != nil {
		return Operation{}, err
	}
	return resolved.Operation, nil
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

// Server resolves a configured server definition.
func (c Collection) Server(name string) (Server, error) {
	resolved, err := c.ResolveServer(name)
	if err != nil {
		return Server{}, err
	}
	return resolved.Server, nil
}

// ResolveServer returns a server with the limit profile it references.
func (c Collection) ResolveServer(name string) (ServerDefinition, error) {
	server, ok := c.Servers[name]
	if !ok {
		return ServerDefinition{}, fmt.Errorf("REST server %q is not defined", name)
	}
	return ServerDefinition{Name: name, Server: server, Limits: c.Limits[server.LimitsRef]}, nil
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
	params := core.LoopParams{
		MachineSpec:     req.Config.MachineSpec,
		Registry:        req.Config.Registry,
		InitFunc:        req.Config.InitFunc,
		ToolAction:      req.Config.ToolAction,
		InitialSignal:   initialSignal,
		InitialResult:   requestSeed(req, initialSignal),
		Budget:          req.Config.Budget,
		CommandTimeout:  parseDuration(req.Config.CommandTimeout, 0),
		Trace:           tracing.NoopTracer{},
		AgentName:       machineRequestAgentName(req),
		Directory:       ".",
		MonitorRecorder: req.MonitorRecorder,
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

func machineRequestAgentName(req MachineRequestRun) string {
	if req.RequestID != "" {
		return "machine_request:" + req.RequestID
	}
	if req.Server != "" && req.Route != "" {
		return "machine_request:" + req.Server + "/" + req.Route
	}
	return "machine_request"
}

func machineRequestTerminalStatus(cfg MachineRequest) func(core.State) core.RunStatus {
	return func(state core.State) core.RunStatus {
		if mapping, ok := cfg.Response.TerminalSignals[string(state)]; ok {
			if mapping.Status >= 200 && mapping.Status < 400 {
				return core.StatusSucceeded
			}
			return core.StatusFailed
		}
		switch state {
		case core.State("Succeeded"), core.State("Done"), core.State("Passed"):
			return core.StatusSucceeded
		case core.State("BudgetExceeded"):
			return core.StatusBudgetExceeded
		default:
			return core.StatusFailed
		}
	}
}

func machineRequestInitialSignal(cfg MachineRequest) core.Signal {
	if cfg.InitialSignal == "" {
		return core.Seed
	}
	return core.Signal(cfg.InitialSignal)
}

func requestSeed(req MachineRequestRun, signal core.Signal) core.Result {
	data, err := json.Marshal(req)
	if err != nil {
		return core.Result{Signal: signal, Output: "{}"}
	}
	return core.Result{Signal: signal, Output: string(data)}
}

func machineRequestResult(req MachineRequestRun, rr core.RunResult, last core.Result) (MachineRequestResult, error) {
	if last.Signal == "" {
		return MachineRequestResult{}, fmt.Errorf("response_missing: request machine produced no response signal")
	}
	output := map[string]interface{}{}
	if last.Output != "" {
		if err := json.Unmarshal([]byte(last.Output), &output); err != nil {
			return MachineRequestResult{}, fmt.Errorf("response_invalid: %w", err)
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
	result, err := r.runner.RunMachineRequest(ctx, MachineRequestRun{
		Server: r.name, Route: name, Method: req.Method, Path: req.URL.Path,
		RequestID:       req.Header.Get("X-Request-ID"),
		Payload:         machineRequestPayload(endpoint.MachineRequest.Request, payload),
		Config:          endpoint.MachineRequest,
		MonitorRecorder: r.requestMonitor,
	})
	if err != nil {
		writeMachineRequestError(w, err)
		return
	}
	r.writeMachineResponse(w, endpoint, result)
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
	mapping, ok := endpoint.MachineRequest.Response.TerminalSignals[result.TerminalSignal]
	if !ok {
		writeMachineRequestError(w, fmt.Errorf("response_missing: terminal signal %q is not mapped", result.TerminalSignal))
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
	body["trace"] = map[string]interface{}{
		"server":          result.Server,
		"route":           result.Route,
		"machine":         result.Machine,
		"terminal_signal": result.TerminalSignal,
		"iterations":      result.Run.Iterations,
		"status":          result.Run.Status,
	}
	return body
}

func machineSelectorValue(selector string, output map[string]interface{}) interface{} {
	if !strings.HasPrefix(selector, "$.") {
		return selector
	}
	return nestedValue(output, strings.Split(strings.TrimPrefix(selector, "$."), "."))
}

func nestedValue(value interface{}, path []string) interface{} {
	if len(path) == 0 {
		return value
	}
	obj, _ := value.(map[string]interface{})
	if obj == nil {
		return nil
	}
	return nestedValue(obj[path[0]], path[1:])
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
