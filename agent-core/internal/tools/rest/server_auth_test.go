// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInjectedLifecycleExitEnforcesDeclaredBearerAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("integration-grade: production REST server launch binds a real loopback listener")
	}
	t.Parallel()
	server := bareLifecycleServer("authenticated_exit")
	server.LifecycleExit.AuthRef = "control"
	def := ServerDefinition{
		Name: "authenticated_exit", Server: server,
		Auth: map[string]AuthProfile{
			"control": {Type: authBearer, TokenRef: "CONTROL_TOKEN"},
		},
		Credentials: StaticCredentials{"CONTROL_TOKEN": "synthetic-secret"},
	}
	state := NewServerState()
	output, err := state.Launch(def)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = state.Stop("authenticated_exit") })
	baseURL := "http://" + output["address"].(string)

	postLifecycleRequest(t, baseURL, "", http.StatusUnauthorized)
	_, signal, err := state.AwaitAny(AwaitAnyOptions{
		Sources: []AwaitSource{{Server: "authenticated_exit"}},
		Timeout: 10 * time.Millisecond,
	})
	require.NoError(t, err)
	require.Equal(t, "AwaitTimedOut", signal, "an unauthenticated request must not enqueue")

	postLifecycleRequest(t, baseURL, "Bearer synthetic-secret", http.StatusAccepted)
	event, signal, err := state.AwaitAny(AwaitAnyOptions{
		Sources: []AwaitSource{{Server: "authenticated_exit"}},
		Timeout: time.Second,
	})
	require.NoError(t, err)
	require.Equal(t, "ExitRequested", signal)
	require.Equal(t, "exit", event.Route)
	headers, _ := event.Payload["headers"].(map[string]interface{})
	require.NotContains(t, headers, "authorization")
}

func TestUnauthenticatedLifecycleExitIsLoopbackOnly(t *testing.T) {
	t.Parallel()
	req, err := http.NewRequest(http.MethodPost, "http://example/api/lifecycle/exit", nil)
	require.NoError(t, err)
	req.RemoteAddr = "192.0.2.10:1234"
	err = authorizeLifecycleRequest(req, ServerDefinition{}, "")
	var authErr lifecycleAuthError
	require.ErrorAs(t, err, &authErr)
	require.Equal(t, http.StatusForbidden, authErr.status)

	req.RemoteAddr = "127.0.0.1:1234"
	require.NoError(t, authorizeLifecycleRequest(req, ServerDefinition{}, ""))
}

func TestLifecycleAuthRefMustResolveAtLoad(t *testing.T) {
	t.Parallel()
	server := bareLifecycleServer("bad_auth")
	server.LifecycleExit.AuthRef = "missing"
	err := ValidateDefinition(Definition{
		Version: "v1", Servers: map[string]Server{"bad_auth": server},
	})
	require.ErrorContains(t, err, `unknown auth profile "missing"`)
}

func TestEnvironmentCredentialsResolveReferenceNames(t *testing.T) {
	resolver := EnvironmentCredentials{}

	t.Run("present", func(t *testing.T) {
		t.Setenv("DECLARATIVE_AGENT_CONTROL_TOKEN", "synthetic-secret")
		value, err := resolver.ResolveCredential("DECLARATIVE_AGENT_CONTROL_TOKEN")
		require.NoError(t, err)
		require.Equal(t, "synthetic-secret", value)
	})

	t.Run("missing", func(t *testing.T) {
		t.Setenv("DECLARATIVE_AGENT_OTHER_TOKEN", "must-not-leak")
		_, err := resolver.ResolveCredential("DECLARATIVE_AGENT_MISSING_TOKEN_1609")
		var resolutionErr credentialResolutionError
		require.ErrorAs(t, err, &resolutionErr)
		require.Equal(t, "DECLARATIVE_AGENT_MISSING_TOKEN_1609", resolutionErr.Ref)
		require.NotContains(t, err.Error(), "must-not-leak")
	})

	t.Run("empty", func(t *testing.T) {
		t.Setenv("DECLARATIVE_AGENT_EMPTY_TOKEN", "")
		_, err := resolver.ResolveCredential("DECLARATIVE_AGENT_EMPTY_TOKEN")
		var resolutionErr credentialResolutionError
		require.ErrorAs(t, err, &resolutionErr)
		require.Equal(t, "DECLARATIVE_AGENT_EMPTY_TOKEN", resolutionErr.Ref)
		require.NotContains(t, err.Error(), "synthetic-secret")
	})
}

func postLifecycleRequest(t *testing.T, baseURL, authorization string, want int) {
	t.Helper()
	req, err := http.NewRequest(
		http.MethodPost, baseURL+"/api/lifecycle/exit",
		bytes.NewBufferString(`{"reason":"operator"}`),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, want, resp.StatusCode)
}
