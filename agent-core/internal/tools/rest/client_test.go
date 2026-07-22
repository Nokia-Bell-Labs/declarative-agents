// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

func TestRESTClient_SyncResourceWords(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(issueHandler))
	defer upstream.Close()
	def := clientDefinition(t, upstream.URL, issueClient())

	requireClientSignal(t, def, InitClientGet, "get", params("1"), "RESTResourceRead")
	requireClientSignal(t, def, InitClientSet, "set", params("1", "new"), "RESTResourceWritten")
	requireClientSignal(t, def, InitClientGet, "get", params("missing"), "RESTMissing")
	requireClientSignal(t, def, InitClientSet, "set", params("domain", "bad"), "RESTDomainFailed")
	requireClientSignal(t, def, InitClientGet, "get", params("boom"), string(core.CommandError))
}

func TestRESTClient_RejectsAuthorityOverride(t *testing.T) {
	t.Parallel()

	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer upstream.Close()
	def := clientDefinition(t, upstream.URL, issueClient())

	result := clientCommand(t, def, InitClientGet, "get", map[string]interface{}{
		"owner": "acme", "repo": "agent-core", "number": "1", "url": "https://evil.example",
	}).Execute()

	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "failure_stage")
	require.Zero(t, requests)
}

func TestRESTClient_RenderCatchAllPathParam(t *testing.T) {
	t.Parallel()

	path := renderPath("/api/v1/docs/{path...}", map[string]interface{}{
		"path": "specs/use-cases/rel03.0-uc007-machine-request-documentation-ux.yaml",
	})

	require.Equal(t, "/api/v1/docs/specs/use-cases/rel03.0-uc007-machine-request-documentation-ux.yaml", path)
}

func TestRESTClient_MutatingOperationsRequireEffects(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidateDefinition(mutatingDefinition(validWriteOperation())))

	missingEffects := validWriteOperation()
	missingEffects.SideEffects = nil
	require.ErrorContains(t, ValidateDefinition(mutatingDefinition(missingEffects)), "side_effects")

	irreversible := validWriteOperation()
	irreversible.Reversibility = Reversibility{Classification: "irreversible"}
	require.ErrorContains(t, ValidateDefinition(mutatingDefinition(irreversible)), "confirmation")

	compensating := validWriteOperation()
	compensating.Compensation = map[string]interface{}{"operation": "restore_issue"}
	require.NoError(t, ValidateDefinition(mutatingDefinition(compensating)))
}

func TestRESTClient_CompensationUndoAndReceipt(t *testing.T) {
	t.Parallel()
	requireRESTClientCompensationUndoReceipt(t)
}

func TestRESTClient_CompensationUndoMemento(t *testing.T) {
	t.Parallel()
	requireRESTClientCompensationUndoReceipt(t)
}

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

func TestRESTClient_CompensationExecutorRunsFromReceipt(t *testing.T) {
	t.Parallel()
	var restored bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/repos/acme/agent-core/issues/ISS-1" {
			restored = true
			require.Equal(t, http.MethodPatch, req.Method)
			writeJSON(w, http.StatusOK, map[string]interface{}{"title": "restored", "id": "ISS-1"})
			return
		}
		issueHandler(w, req)
	}))
	defer upstream.Close()
	def := clientDefinition(t, upstream.URL, issueClient())
	write := clientCommand(t, def, InitClientSet, "set", params("1", "new"))
	res := write.Execute()
	require.Equal(t, core.Signal("RESTResourceWritten"), res.Signal)
	require.NotEmpty(t, res.Receipt)

	cp := &core.InMemoryCheckpoint{}
	require.NoError(t, cp.Save(core.Position{}, core.Execution{{CommandName: write.Name(), Receipt: res.Receipt}}))
	_, exec, err := cp.Load()
	require.NoError(t, err)
	require.Len(t, exec, 1)

	result := restCompensationExecutor(t, def).CompensateFromReceipt(context.Background(), exec[0].CommandName, exec[0].Receipt)

	require.Equal(t, core.Signal("RESTResourceWritten"), result.Signal, result.Output)
	require.True(t, restored)
}

func TestRESTClient_CompensationExecutorReportsMissingOperation(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(issueHandler))
	defer upstream.Close()
	def := clientDefinition(t, upstream.URL, issueClient())
	write := clientCommand(t, def, InitClientSet, "set", params("1", "new"))
	res := write.Execute()
	require.Equal(t, core.Signal("RESTResourceWritten"), res.Signal)
	receipt := replaceRESTCompensationOperation(t, res.Receipt, "missing")

	result := restCompensationExecutor(t, def).CompensateFromReceipt(context.Background(), write.Name(), receipt)

	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "compensation_lookup")
}

// TestRESTClient_RedactionRunsBeforePersistence proves invariant (3): response
// redaction runs inside mapClientResponse before Execute returns the Result, so
// the Result the loop hands to the checkpoint Save — and therefore the
// tool_outputs forward plane and any later command-state $from read — never sees
// a redacted field (srd038-command-state-store R5, srd036-dolt-state-persistence
// R5.1). The Result returned by Execute is exactly what a persisting caller
// checkpoints, so asserting the field is already gone here proves the ordering.
func TestRESTClient_RedactionRunsBeforePersistence(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"title":"ok","secret":"body-secret"}`))
	}))
	defer upstream.Close()

	// issueClient's get operation redacts body.secret.
	def := clientDefinition(t, upstream.URL, issueClient())

	result := clientCommand(t, def, InitClientGet, "get", params("1")).Execute()
	require.Equal(t, core.Signal("RESTResourceRead"), result.Signal)

	// The redacted value is absent from the persisted Result output and the
	// [REDACTED] marker is present in its place; nothing downstream can recover it.
	require.NotContains(t, result.Output, "body-secret")
	require.Contains(t, result.Output, "[REDACTED]")
}

func TestRESTTools_TracingRedactionAndErrors(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"title":"ok","secret":"body-secret"}`))
	}))
	defer upstream.Close()
	def := clientDefinition(t, upstream.URL, issueClient())
	client := def.Clients["github"]
	client.AuthRef = "token"
	def.Clients["github"] = client
	def.Auth = map[string]AuthProfile{"token": {
		Type: authHeaderToken, Header: "X-Token", TokenRef: "token_ref",
	}}

	result := clientCommandWithCredentials(t, def, InitClientGet, "get", params("1"), authCredentials()).Execute()
	require.Equal(t, core.Signal("RESTResourceRead"), result.Signal)
	require.NotContains(t, result.Output, "synthetic-token")
	require.NotContains(t, result.Output, "body-secret")
	require.Contains(t, result.Output, "[REDACTED]")

	badDef := clientDefinition(t, upstream.URL, issueClient())
	op := badDef.Clients["github"].Resources["issue"].Operations["get"]
	op.Success.Status = []int{201}
	badDef.Clients["github"].Resources["issue"].Operations["get"] = op
	result = clientCommand(t, badDef, InitClientGet, "get", params("1")).Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "status_mapping")
}

func TestRESTRedactionPolicy_UnifiesOutputErrorsAndMonitorLabels(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"title": "ok", "secret": "synthetic-token"})
	}))
	defer upstream.Close()
	def := clientDefinition(t, upstream.URL, issueClient())
	client := def.Clients["github"]
	client.AuthRef = "token"
	def.Clients["github"] = client
	def.Auth = map[string]AuthProfile{"token": {
		Type: authHeaderToken, Header: "X-Token", TokenRef: "token_ref",
	}}

	result := clientCommandWithCredentials(t, def, InitClientGet, "get", params("1"), authCredentials()).Execute()
	require.Equal(t, core.Signal("RESTResourceRead"), result.Signal)
	require.NotContains(t, result.Output, "synthetic-token")
	require.Contains(t, result.Output, redactedValue)

	redactedErr := redactError(fmt.Errorf("network leaked synthetic-token"), resolvedClientOperation(t, def), authCredentials())
	require.NotContains(t, redactedErr.Error(), "synthetic-token")
	require.Contains(t, redactedErr.Error(), redactedValue)

	labels := safeLabels(map[string]string{"operation": "get", "credential": "synthetic-token", "profile": "monitor"})
	require.Equal(t, "get", labels["operation"])
	require.Equal(t, "monitor", labels["profile"])
	require.NotContains(t, labels, "credential")
}

func TestRESTClient_ResolvesAuthCredentialRefs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		auth AuthProfile
		want func(*http.Request) bool
	}{
		{name: "bearer", auth: AuthProfile{Type: authBearer, TokenRef: "github_token"}, want: bearerAuthSent},
		{name: "header token", auth: AuthProfile{Type: authHeaderToken, Header: "X-Token", TokenRef: "github_token"}, want: headerTokenSent},
		{name: "query token", auth: AuthProfile{Type: authQueryToken, Query: "access_token", TokenRef: "github_token"}, want: queryTokenSent},
		{name: "basic", auth: AuthProfile{Type: authBasic, UsernameRef: "user_ref", PasswordRef: "password_ref"}, want: basicAuthSent},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var accepted bool
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				accepted = tc.want(req)
				require.NotContains(t, req.Header.Get("Authorization"), "github_token")
				writeJSON(w, http.StatusOK, map[string]interface{}{"title": "ok"})
			}))
			defer upstream.Close()
			def := authenticatedDefinition(t, upstream.URL, tc.auth)

			result := clientCommandWithCredentials(t, def, InitClientGet, "get", params("1"), authCredentials()).Execute()

			require.Equal(t, core.Signal("RESTResourceRead"), result.Signal, result.Output)
			require.True(t, accepted)
			require.NotContains(t, result.Output, "synthetic-token")
			require.NotContains(t, result.Output, "synthetic-password")
		})
	}
}

func TestRESTClient_MissingCredentialReferenceFailsAuthResolution(t *testing.T) {
	t.Parallel()

	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer upstream.Close()
	def := authenticatedDefinition(t, upstream.URL, AuthProfile{Type: authBearer, TokenRef: "missing_token"})

	result := clientCommandWithCredentials(t, def, InitClientGet, "get", params("1"), authCredentials()).Execute()

	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "auth_resolution")
	require.NotContains(t, result.Output, "synthetic-token")
	require.Zero(t, requests)
}

func TestRESTClient_RedirectAllowlistPolicy(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"title": "ok"})
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, target.URL+"/repos/acme/agent-core/issues/1", http.StatusFound)
	}))
	defer redirect.Close()

	def := clientDefinition(t, redirect.URL, issueClient())
	setRedirectPolicy(def, RedirectPolicy{Mode: redirectAllowlist, AllowHosts: []string{targetURLHost(target)}})
	requireClientSignal(t, def, InitClientGet, "get", params("1"), "RESTResourceRead")

	blocked := clientDefinition(t, redirect.URL, issueClient())
	setRedirectPolicy(blocked, RedirectPolicy{Mode: redirectAllowlist, AllowHosts: []string{"example.invalid"}})
	result := clientCommand(t, blocked, InitClientGet, "get", params("1")).Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "network_io")
}

func TestRESTClient_RequestAndResponseSizeLimits(t *testing.T) {
	t.Parallel()

	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writeJSON(w, http.StatusOK, map[string]interface{}{"title": strings.Repeat("x", 32)})
	}))
	defer upstream.Close()

	requestLimited := clientDefinition(t, upstream.URL, issueClient())
	setRequestLimit(requestLimited, 8)
	result := clientCommand(t, requestLimited, InitClientSet, "set", params("1", strings.Repeat("x", 32))).Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "request_rendering")
	require.Zero(t, requests)

	responseLimited := clientDefinition(t, upstream.URL, issueClient())
	setResponseLimit(responseLimited, 8)
	result = clientCommand(t, responseLimited, InitClientGet, "get", params("1")).Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "size_limit")
	require.NotContains(t, result.Output, strings.Repeat("x", 16))
}

func TestRESTClient_CIDRAllowlistPolicy(t *testing.T) {
	t.Parallel()

	requireCIDRAllowlistPolicy(t)
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

func TestRESTClient_ResponseSchemaAndDomainErrorOutput(t *testing.T) {
	t.Parallel()

	requireResponseSchemaAndDomainErrorOutput(t)
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

func TestRESTClientRecordsMonitorMetrics(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"title": "ok"})
	}))
	defer upstream.Close()
	rec := &restMetricRecorder{}
	cmd := clientCommand(t, clientDefinition(t, upstream.URL, issueClient()), InitClientGet, "get", params("1"))
	cmd.(core.MonitorRecorderAware).SetMonitorRecorder(rec)

	result := cmd.Execute()

	require.Equal(t, core.Signal("RESTResourceRead"), result.Signal, result.Output)
	requireRestMetric(t, rec.samples, "rest.http_status_code", 200)
	requireRestMetric(t, rec.samples, "rest.retry_count", 0)
	requirePositiveRestMetric(t, rec.samples, "rest.response_bytes")
	for _, sample := range rec.samples {
		require.Equal(t, "get", sample.Attributes["operation"])
		require.NotContains(t, sample.Attributes, "url")
		require.NotContains(t, sample.Attributes, "authorization")
	}
}

func TestRESTClientMetricConfigCanDisableSamples(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"title": "ok"})
	}))
	defer upstream.Close()
	rec := &restMetricRecorder{}
	cmd := clientCommandWithMetrics(
		t,
		clientDefinition(t, upstream.URL, issueClient()),
		InitClientGet,
		"get",
		params("1"),
		core.MetricConfig{Disabled: true},
	)
	cmd.(core.MonitorRecorderAware).SetMonitorRecorder(rec)

	result := cmd.Execute()

	require.Equal(t, core.Signal("RESTResourceRead"), result.Signal, result.Output)
	require.Empty(t, rec.samples)
}

func TestRESTClientMetricsCarryDispatchEnvelope(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"title": "ok"})
	}))
	defer upstream.Close()
	cmd := clientCommand(t, clientDefinition(t, upstream.URL, issueClient()), InitClientGet, "get", params("1"))

	samples := runRESTMetricLoop(t, cmd, core.Signal("RESTResourceRead"))

	requireRestMetric(t, samples, "rest.http_status_code", 200)
	requireRESTEnvelope(t, samples, "rest.http_status_code", cmd.Name())
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
