// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"errors"
	"fmt"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

// TestRESTClient_RedactionRunsBeforePersistence proves that REST preserves its
// immediate marker contract while typed paths keep sensitive fields out of live
// and rehydrated command-state views (srd038 R5, srd036 R3).
func TestRESTClient_RedactionRunsBeforePersistence(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"title":"ok","secret":"body-secret"}`))
	}))
	defer upstream.Close()

	// issueClient's get operation redacts body.secret.
	def := clientDefinition(t, upstream.URL, issueClient())
	op := def.Clients["github"].Resources["issue"].Operations["get"]
	op.Response.Output["secret_copy"] = "$.secret"
	def.Clients["github"].Resources["issue"].Operations["get"] = op

	result := clientCommand(t, def, InitClientGet, "get", params("1")).Execute()
	require.Equal(t, core.Signal("RESTResourceRead"), result.Signal)

	// REST callers retain the marker-based response contract, including mapped
	// copies, without carrying the secret value in redaction metadata.
	require.NotContains(t, result.Output, "body-secret")
	require.Contains(t, result.Output, "[REDACTED]")
	require.Equal(t, core.OutputRedactionVersion1, result.Redaction.Version)
	require.ElementsMatch(t, []core.OutputRedactionPath{
		{"body", "secret"},
		{"mapped", "secret_copy"},
	}, result.Redaction.Paths)

	digest := core.DigestResult(result)
	require.NotContains(t, digest.Output, `"secret"`)
	require.NotContains(t, digest.Output, "secret_copy")
	execution := core.Execution{{
		CommandName: result.CommandName,
		Label:       "fetch",
		Signal:      result.Signal,
		Result:      digest,
	}}

	assertSecretPathUnavailable(t, core.NewCommandStateView(execution))

	checkpoint := &core.InMemoryCheckpoint{}
	require.NoError(t, checkpoint.Save(core.Position{}, execution))
	_, loaded, err := checkpoint.Load()
	require.NoError(t, err)
	require.NotContains(t, loaded[0].Result.Output, "body-secret")
	require.NotContains(t, loaded[0].Result.Output, `"secret"`)
	require.NotContains(t, loaded[0].Result.Output, "secret_copy")
	assertSecretPathUnavailable(t, core.NewCommandStateView(loaded))
}

func assertSecretPathUnavailable(t *testing.T, view core.CommandStateView) {
	t.Helper()
	value, err := core.ResolveFromSelector(view, "$from(fetch).body.secret")
	require.Nil(t, value)
	var missing *core.UnresolvedPathError
	require.True(t, errors.As(err, &missing), err)
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
	require.Contains(t, result.Redaction.Paths, core.OutputRedactionPath{"body", "secret"})
	require.Contains(t, result.Redaction.Paths, core.OutputRedactionPath{"headers", "x-token"})

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

func TestRESTClient_ResponseSchemaAndDomainErrorOutput(t *testing.T) {
	t.Parallel()

	requireResponseSchemaAndDomainErrorOutput(t)
}
