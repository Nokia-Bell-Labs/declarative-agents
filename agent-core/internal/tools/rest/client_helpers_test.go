// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"encoding/json"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func requireRESTClientCompensationUndoReceipt(t *testing.T) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(issueHandler))
	defer upstream.Close()
	def := clientDefinition(t, upstream.URL, issueClient())

	read := clientCommand(t, def, InitClientGet, "get", params("1"))
	readRes := read.Execute()
	require.Equal(t, core.Signal("RESTResourceRead"), readRes.Signal)
	require.Empty(t, readRes.Receipt)
	require.Equal(t, core.ToolDone, read.Undo(core.Result{}).Signal)

	write := clientCommand(t, def, InitClientSet, "set", params("1", "new"))
	writeRes := write.Execute()
	require.Equal(t, core.Signal("RESTResourceWritten"), writeRes.Signal)
	require.NotEmpty(t, writeRes.Receipt)
	require.Equal(t, core.CommandError, write.Undo(core.Result{}).Signal)
	requireRESTCompensationReceipt(t, writeRes.Receipt)
}

func requireCIDRAllowlistPolicy(t *testing.T) {
	t.Helper()

	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeJSON(w, http.StatusOK, map[string]interface{}{"title": "ok"})
	}))
	defer upstream.Close()

	allowedCIDR := loopbackCIDR(targetURLHost(upstream))
	allowed := clientDefinition(t, upstream.URL, issueClient())
	setNetworkPolicy(allowed, NetworkPolicy{CIDRs: []string{allowedCIDR}})
	requireClientSignal(t, allowed, InitClientGet, "get", params("1"), "RESTResourceRead")

	blocked := clientDefinition(t, upstream.URL, issueClient())
	setNetworkPolicy(blocked, NetworkPolicy{CIDRs: []string{"10.0.0.0/8"}})
	result := clientCommand(t, blocked, InitClientGet, "get", params("1")).Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "request_rendering")
	require.Equal(t, 1, requests)
}

func requireResponseSchemaAndDomainErrorOutput(t *testing.T) {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.Contains(req.URL.Path, "domain") {
			http.Error(w, `{"error":"invalid"}`, http.StatusUnprocessableEntity)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"title": 42})
	}))
	defer upstream.Close()
	def := clientDefinition(t, upstream.URL, issueClient())
	op := def.Clients["github"].Resources["issue"].Operations["get"]
	op.Response.Schema = bodySchema("title")
	op.Failures[1].DomainErrorCode = "validation_failed"
	def.Clients["github"].Resources["issue"].Operations["get"] = op

	result := clientCommand(t, def, InitClientGet, "get", params("1")).Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "response_mapping")

	result = clientCommand(t, def, InitClientGet, "get", params("domain")).Execute()
	require.Equal(t, core.Signal("RESTDomainFailed"), result.Signal, result.Output)
	require.Contains(t, result.Output, `"domain_error_code":"validation_failed"`)
}

func issueHandler(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/repos/acme/agent-core/issues/boom" {
		http.Error(w, "boom", http.StatusInternalServerError)
		return
	}
	if req.URL.Path == "/repos/acme/agent-core/issues/missing" {
		http.NotFound(w, req)
		return
	}
	if req.URL.Path == "/repos/acme/agent-core/issues/domain" {
		http.Error(w, `{"error":"domain"}`, http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"title": "ok", "id": "ISS-1", "request_id": "REQ-1"})
}

type restMetricRecorder struct {
	samples []monitor.MetricSample
}

func (r *restMetricRecorder) RecordMetric(_ context.Context, sample monitor.MetricSample) error {
	r.samples = append(r.samples, sample)
	return nil
}

func requireRestMetric(t *testing.T, samples []monitor.MetricSample, name string, value float64) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name == name && sample.Value == value {
			return
		}
	}
	t.Fatalf("missing metric %s=%v in %#v", name, value, samples)
}

func requirePositiveRestMetric(t *testing.T, samples []monitor.MetricSample, name string) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name == name && sample.Value > 0 {
			return
		}
	}
	t.Fatalf("missing positive metric %s in %#v", name, samples)
}

func requireRESTCompensationReceipt(t *testing.T, receipt string) {
	t.Helper()
	compensation, ok, err := undo.DecodeBoundaryReceipt(receipt)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "restore", compensation.Strategy)
	require.Equal(t, "github", compensation.RestRef)
	require.Equal(t, "issue", compensation.Resource)
	require.Equal(t, "set", compensation.Operation)
	require.Equal(t, "acme", compensation.Parameters["owner"])
	require.Equal(t, "agent-core", compensation.Parameters["repo"])
	require.Equal(t, "ISS-1", compensation.ResourceID)
	require.Equal(t, "REQ-1", compensation.RequestID)
	require.Equal(t, "set", compensation.Compensation["operation"])
	require.Equal(t, "restored", compensation.Compensation["parameters"].(map[string]interface{})["title"])
}

func replaceRESTCompensationOperation(t *testing.T, receipt, operation string) string {
	t.Helper()
	var payload undo.BoundaryCompensationPayload
	require.NoError(t, json.Unmarshal([]byte(receipt), &payload))
	payload.BoundaryCompensation.Compensation["operation"] = operation
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return string(data)
}

func restCompensationExecutor(t *testing.T, def Definition) CompensationExecutor {
	t.Helper()
	collection := NewCollection()
	require.NoError(t, collection.Add(def))
	return CompensationExecutor{Definitions: collection}
}

func runRESTMetricLoop(t *testing.T, cmd core.Command, signal core.Signal) []monitor.MetricSample {
	t.Helper()
	// Keep this fixture package-local so REST assertions name REST commands and signals.
	store := monitor.NewStore(monitor.Limits{Samples: 10})
	params := restMetricLoopParams(cmd, signal, monitor.NewRecorder(store, nil))
	_, err := core.Loop(params, context.Background())
	require.NoError(t, err)
	return store.Snapshot().RecentSamples
}

func restMetricLoopParams(cmd core.Command, signal core.Signal, rec monitor.RuntimeRecorder) core.LoopParams {
	spec := &core.MachineSpec{
		Name:           "rest-metrics",
		InitialState:   "Start",
		MetricLabels:   core.MetricLabels{"use_case": "rel04.0-monitor"},
		States:         core.StateSpecsFromNames("Start", "Working", "Done"),
		TerminalStates: []string{"Done"},
		Signals:        core.SignalSpecsFromNames(string(core.Seed), string(signal)),
		Transitions: []core.TransitionSpec{
			{State: "Start", Signal: string(core.Seed), Next: "Working", Action: cmd.Name(), MetricLabels: core.MetricLabels{"phase": "dispatch"}},
			{State: "Working", Signal: string(signal), Next: "Done"},
		},
	}
	return core.LoopParams{
		MachineSpec:     spec,
		AgentName:       "rest-run",
		Trace:           tracing.NoopTracer{},
		Budget:          core.Budget{MaxIterations: 3},
		MonitorRecorder: rec,
		InitFunc: func(reg *core.Registry) error {
			reg.Register(core.ToolSpec{Name: cmd.Name(), Visibility: core.Internal}, restMetricBuilder{cmd: cmd})
			return nil
		},
		Hooks: core.LoopHooks{TerminalStatus: func(core.State) core.RunStatus { return core.StatusSucceeded }},
	}
}

type restMetricBuilder struct {
	cmd core.Command
}

func (b restMetricBuilder) Build(core.Result) core.Command {
	return b.cmd
}

func requireRESTEnvelope(t *testing.T, samples []monitor.MetricSample, name string, toolName string) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name != name {
			continue
		}
		require.Equal(t, toolName, sample.ToolName)
		require.Equal(t, "rest-run", sample.RunID)
		require.Equal(t, "Working", sample.State)
		require.Equal(t, "RESTResourceRead", sample.Signal)
		require.Equal(t, "success", sample.Status)
		require.Equal(t, "rel04.0-monitor", sample.Attributes["use_case"])
		require.Equal(t, "dispatch", sample.Attributes["phase"])
		return
	}
	t.Fatalf("missing metric %s in %#v", name, samples)
}

func issueClient() Client {
	return Client{Resources: map[string]Resource{"issue": {
		Path: "/repos/{owner}/{repo}/issues/{number}",
		Operations: map[string]Operation{
			"get": issueOperation(http.MethodGet, "RESTResourceRead"),
			"set": issueSetOperation(),
		},
	}}}
}

func authenticatedDefinition(t *testing.T, baseURL string, auth AuthProfile) Definition {
	t.Helper()
	def := clientDefinition(t, baseURL, issueClient())
	client := def.Clients["github"]
	client.AuthRef = "auth"
	def.Clients["github"] = client
	def.Auth = map[string]AuthProfile{"auth": auth}
	return def
}

func authCredentials() StaticCredentials {
	return StaticCredentials{
		"github_token": "synthetic-token",
		"user_ref":     "synthetic-user",
		"password_ref": "synthetic-password",
		"token_ref":    "synthetic-token",
	}
}

func bearerAuthSent(req *http.Request) bool {
	return req.Header.Get("Authorization") == "Bearer synthetic-token"
}

func headerTokenSent(req *http.Request) bool {
	return req.Header.Get("X-Token") == "synthetic-token"
}

func queryTokenSent(req *http.Request) bool {
	return req.URL.Query().Get("access_token") == "synthetic-token"
}

func basicAuthSent(req *http.Request) bool {
	username, password, ok := req.BasicAuth()
	return ok && username == "synthetic-user" && password == "synthetic-password"
}

func issueOperation(method, signal string) Operation {
	return Operation{
		Method: method,
		Params: RequestBinding{Path: map[string]interface{}{
			"owner": map[string]interface{}{}, "repo": map[string]interface{}{}, "number": map[string]interface{}{},
		}},
		Success:  StatusMapping{Status: []int{200}, Signal: signal},
		Failures: []StatusMapping{{Status: []int{404}, Signal: "RESTMissing"}, {Status: []int{422}, Signal: "RESTDomainFailed"}},
		Response: ResponseMapping{
			Output: map[string]string{"title": "$.title"}, Redact: []string{"body.secret"},
			ResourceID: "$.id", RequestID: "$.request_id",
		},
		SideEffects:   []SideEffect{{Kind: "external_api", State: "read_only"}},
		Reversibility: Reversibility{Classification: "reversible", Undo: "noop"},
	}
}

func issueSetOperation() Operation {
	op := issueOperation(http.MethodPatch, "RESTResourceWritten")
	op.Params.BodySchema = bodySchema("title")
	op.Body = map[string]interface{}{"title": "{{ params.title }}"}
	op.SideEffects = []SideEffect{{Kind: "external_api", State: "issue_updated"}}
	op.Reversibility = Reversibility{Classification: "compensatable", Undo: "restore"}
	op.Compensation = map[string]interface{}{
		"operation":  "set",
		"parameters": map[string]interface{}{"title": "restored"},
	}
	return op
}

func params(number string, title ...string) map[string]interface{} {
	values := map[string]interface{}{"owner": "acme", "repo": "agent-core", "number": number}
	if len(title) > 0 {
		values["title"] = title[0]
	}
	return values
}

func mutatingDefinition(operation Operation) Definition {
	return Definition{
		Version: "v1",
		Clients: map[string]Client{"github": {
			BaseURL: "https://api.example", Resources: map[string]Resource{"issue": {
				Path: "/issue/{number}", Operations: map[string]Operation{"set": operation},
			}},
		}},
	}
}

func setRedirectPolicy(def Definition, policy RedirectPolicy) {
	setClientLimit(def, func(limit *LimitProfile) { limit.Redirect = policy })
}

func setRequestLimit(def Definition, limit int) {
	setClientLimit(def, func(profile *LimitProfile) { profile.MaxRequestBytes = limit })
}

func setResponseLimit(def Definition, limit int) {
	setClientLimit(def, func(profile *LimitProfile) { profile.MaxResponseBytes = limit })
}

func setNetworkPolicy(def Definition, policy NetworkPolicy) {
	setClientLimit(def, func(profile *LimitProfile) { profile.Network = policy })
}

func setClientLimit(def Definition, mutate func(*LimitProfile)) {
	profile := def.Limits["test"]
	mutate(&profile)
	def.Limits["test"] = profile
	client := def.Clients["github"]
	client.LimitsRef = "test"
	def.Clients["github"] = client
}

func targetURLHost(server *httptest.Server) string {
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	return req.URL.Hostname()
}

func loopbackCIDR(host string) string {
	if strings.Contains(host, ":") {
		return "::1/128"
	}
	return "127.0.0.0/8"
}
