// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
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

	result := clientCommand(def, InitClientGet, "get", map[string]interface{}{
		"owner": "acme", "repo": "agent-core", "number": "1", "url": "https://evil.example",
	}).Execute()

	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "failure_stage")
	require.Zero(t, requests)
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

func TestRESTTools_TracingRedactionAndErrors(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"title":"ok","secret":"body-secret"}`))
	}))
	defer upstream.Close()
	client := issueClient()
	client.AuthRef = "token"
	def := clientDefinition(t, upstream.URL, client)
	def.Auth = map[string]AuthProfile{"token": {
		Type: authHeaderToken, Header: "X-Token", TokenRef: "token_ref",
	}}

	result := clientCommandWithCredentials(def, InitClientGet, "get", params("1"), authCredentials()).Execute()
	require.Equal(t, core.Signal("RESTResourceRead"), result.Signal)
	require.NotContains(t, result.Output, "synthetic-token")
	require.NotContains(t, result.Output, "body-secret")
	require.Contains(t, result.Output, "[REDACTED]")

	badDef := clientDefinition(t, upstream.URL, issueClient())
	op := badDef.Clients["github"].Resources["issue"].Operations["get"]
	op.Success.Status = []int{201}
	badDef.Clients["github"].Resources["issue"].Operations["get"] = op
	result = clientCommand(badDef, InitClientGet, "get", params("1")).Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "status_mapping")
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

			result := clientCommandWithCredentials(def, InitClientGet, "get", params("1"), authCredentials()).Execute()

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

	result := clientCommandWithCredentials(def, InitClientGet, "get", params("1"), authCredentials()).Execute()

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
	result := clientCommand(blocked, InitClientGet, "get", params("1")).Execute()
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
	result := clientCommand(requestLimited, InitClientSet, "set", params("1", strings.Repeat("x", 32))).Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "request_rendering")
	require.Zero(t, requests)

	responseLimited := clientDefinition(t, upstream.URL, issueClient())
	setResponseLimit(responseLimited, 8)
	result = clientCommand(responseLimited, InitClientGet, "get", params("1")).Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "size_limit")
	require.NotContains(t, result.Output, strings.Repeat("x", 16))
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
	writeJSON(w, http.StatusOK, map[string]interface{}{"title": "ok", "id": "ISS-1"})
}

func clientCommand(def Definition, init, operation string, input map[string]interface{}) core.Command {
	return clientCommandWithCredentials(def, init, operation, input, nil)
}

func clientCommandWithCredentials(
	def Definition,
	init string,
	operation string,
	input map[string]interface{},
	credentials CredentialResolver,
) core.Command {
	collection := NewCollection()
	_ = collection.Add(def)
	resolved, _ := collection.ResolveClientOperation(ClientToolConfig{
		RestRef: "github", Resource: "issue", Operation: operation,
	})
	params, _ := json.Marshal(map[string]interface{}{"tool": init, "parameters": input})
	return ClientBuilder{
		ToolName: init, Init: init, Operation: resolved, Credentials: credentials,
	}.Build(core.Result{Output: string(params)})
}

func requireClientSignal(t *testing.T, def Definition, init, operation string, input map[string]interface{}, signal string) {
	t.Helper()
	result := clientCommand(def, init, operation, input).Execute()
	require.Equal(t, core.Signal(signal), result.Signal, result.Output)
	require.Contains(t, result.Output, `"operation":"`+operation+`"`)
}

func clientDefinition(t *testing.T, baseURL string, client Client) Definition {
	t.Helper()
	client.BaseURL = baseURL
	client.AuthRef = "none"
	def := Definition{
		Version: "v1",
		Auth: map[string]AuthProfile{
			"none": {Type: authNone},
		},
		Limits:  map[string]LimitProfile{"test": {}},
		Clients: map[string]Client{"github": client},
	}
	require.NoError(t, ValidateDefinition(def))
	return def
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
