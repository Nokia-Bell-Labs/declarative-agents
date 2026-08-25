// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"fmt"
	"net/http"

	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	restmock "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/mock"
)

func newMockState(endpoints map[string]restdef.Endpoint) (MockEngine, error) {
	specs := make([]restmock.EndpointSpec, 0, len(endpoints))
	for name, endpoint := range endpoints {
		if endpoint.Binding != bindingMock {
			continue
		}
		specs = append(specs, restmock.EndpointSpec{Name: name, Config: toMockConfig(endpoint.Mock)})
	}
	return restmock.New(specs)
}

func toMockConfig(cfg *restdef.MockConfig) *restmock.Config {
	if cfg == nil {
		return nil
	}
	out := &restmock.Config{Fixtures: cfg.Fixtures, Routes: make([]restmock.Route, 0, len(cfg.Routes))}
	for _, route := range cfg.Routes {
		out.Routes = append(out.Routes, restmock.Route{
			Method: route.Method, Path: route.Path, Responses: toMockResponses(route.Responses),
		})
	}
	return out
}

func toMockResponses(responses []restdef.MockResponse) []restmock.Response {
	out := make([]restmock.Response, 0, len(responses))
	for _, response := range responses {
		out = append(out, restmock.Response{
			Status: response.Status, Headers: response.Headers, Body: response.Body,
		})
	}
	return out
}

func (r *serverRuntime) serveMock(w http.ResponseWriter, req *http.Request) {
	if r.mock == nil {
		http.Error(w, fmt.Sprintf("REST server %q has no mock engine", r.name), http.StatusInternalServerError)
		return
	}
	r.mock.Serve(w, req, r.def.Limits.MaxRequestBytes)
}

func (r *serverRuntime) writeMockLog(w http.ResponseWriter) {
	if r.mock == nil {
		http.Error(w, fmt.Sprintf("REST server %q has no mock engine", r.name), http.StatusInternalServerError)
		return
	}
	r.mock.WriteLog(w)
}
