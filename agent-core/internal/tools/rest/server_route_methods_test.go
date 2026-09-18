// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	restvalidation "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/validation"
	"github.com/stretchr/testify/require"
)

// routeMethodsServer declares one resource with GET and POST on the same path,
// a POST-only path, and a GET catch-all beside them, the shape a UI host with
// API routes takes.
func routeMethodsServer() restdef.Server {
	return restdef.Server{
		Address:  "127.0.0.1:0",
		Queue:    restdef.QueueConfig{Name: "routes", Capacity: 8, Timeout: "20ms"},
		Shutdown: restdef.ShutdownConfig{Timeout: "200ms", DrainPolicy: "drain_then_stop"},
		Endpoints: map[string]restdef.Endpoint{
			"items_list":   {Method: "GET", Path: "/api/items", Binding: bindingHealth},
			"items_create": signalEndpoint("POST", "/api/items", "ItemCreated"),
			"chat":         signalEndpoint("POST", "/api/chat", "ChatRequested"),
			"ui":           withPathParams(restdef.Endpoint{Method: "GET", Path: "/{path...}", Binding: bindingHealth}, "path"),
		},
	}
}

// withPathParams declares each named path parameter as a string, as the loader
// requires of every parameter a path template names.
func withPathParams(endpoint restdef.Endpoint, names ...string) restdef.Endpoint {
	endpoint.Request.Path = map[string]interface{}{}
	for _, name := range names {
		endpoint.Request.Path[name] = map[string]interface{}{"type": "string"}
	}
	return endpoint
}

// requestAllow sends one request and returns its status and Allow header.
func requestAllow(t *testing.T, method, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewBufferString(`{}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, resp.Header.Get("Allow")
}

// TestRESTServer_SelectsSamePathEndpointByMethod covers srd029 R6.8: endpoints
// sharing a path are told apart by method, a method the most specific path does
// not declare answers 405 with Allow, and a less specific catch-all does not
// absorb that method.
func TestRESTServer_SelectsSamePathEndpointByMethod(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, routeMethodsServer(), restdef.LimitProfile{})
	defer stopRESTServer(t, state, "routes")

	requestStatus(t, http.MethodGet, baseURL+"/api/items", "", http.StatusOK)
	postStatus(t, baseURL+"/api/items", `{}`, http.StatusAccepted)
	requireAwaitSignal(t, state, "routes", "ItemCreated")

	status, allow := requestAllow(t, http.MethodPut, baseURL+"/api/items")
	require.Equal(t, http.StatusMethodNotAllowed, status)
	require.Equal(t, "GET, POST", allow)

	status, allow = requestAllow(t, http.MethodGet, baseURL+"/api/chat")
	require.Equal(t, http.StatusMethodNotAllowed, status, "the catch-all must not answer a GET the POST-only path owns")
	require.Equal(t, "POST", allow)

	requestStatus(t, http.MethodGet, baseURL+"/anything/else", "", http.StatusOK)
	requireAwaitSignal(t, state, "routes", "AwaitTimedOut")
}

// TestRESTDefinition_RejectsSameMethodOnSamePath covers srd029 R6.9: two
// endpoints declaring one method on one path shape fail to load, naming both,
// while the same path under different methods loads.
func TestRESTDefinition_RejectsSameMethodOnSamePath(t *testing.T) {
	t.Parallel()

	definition := func(endpoints map[string]restdef.Endpoint) restdef.Definition {
		server := routeMethodsServer()
		server.Endpoints = endpoints
		return restdef.Definition{Version: "v1", Servers: map[string]restdef.Server{"routes": server}}
	}

	require.NoError(t, restvalidation.ValidateDefinition(definition(routeMethodsServer().Endpoints)))

	err := restvalidation.ValidateDefinition(definition(map[string]restdef.Endpoint{
		"by_id":  withPathParams(signalEndpoint("POST", "/api/items/{id}", "First"), "id"),
		"by_key": withPathParams(signalEndpoint("POST", "/api/items/{key}", "Second"), "key"),
	}))
	require.ErrorContains(t, err, `endpoints "by_id" and "by_key" both declare POST /api/items/{key}`)

	require.NoError(t, restvalidation.ValidateDefinition(definition(map[string]restdef.Endpoint{
		"by_id":   withPathParams(signalEndpoint("POST", "/api/items/{id}", "First"), "id"),
		"catchup": withPathParams(signalEndpoint("POST", "/api/items/{rest...}", "Second"), "rest"),
	})), "a parameter and a catch-all are different shapes")
}
