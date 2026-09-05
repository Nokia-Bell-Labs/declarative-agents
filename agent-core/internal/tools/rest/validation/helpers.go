// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package validation

import (
	"sort"
	"strings"
	"time"

	restclient "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/client"
	restmock "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/mock"
	restmonitor "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/redact"
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
	monitorViewMachine          = restmonitor.ViewMachine
	monitorViewDeclaredMachines = restmonitor.ViewDeclaredMachines
	monitorViewState            = restmonitor.ViewState
	monitorViewTools            = restmonitor.ViewTools
	monitorViewMetrics          = restmonitor.ViewMetrics
	monitorViewEvents           = restmonitor.ViewEvents
	monitorViewCommandState     = restmonitor.ViewCommandState
)

var forbiddenRuntimeAuthorityFields = map[string]bool{
	"auth":            true,
	"auth_ref":        true,
	"base_url":        true,
	"host":            true,
	"method":          true,
	"redirect":        true,
	"redirect_policy": true,
	"url":             true,
}

// handledServerBindings is copied from the parent server router so validation
// can reject unknown bindings without importing rest. Keep in sync with
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

func sortedServerBindings() string {
	names := make([]string, 0, len(handledServerBindings))
	for b := range handledServerBindings {
		names = append(names, b)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func retryDelay(retry RetryPolicy, attempt int) time.Duration {
	return restclient.RetryDelay(retry, attempt)
}

func declaredParamNames(binding RequestBinding) map[string]bool {
	return restclient.DeclaredParamNames(binding)
}

func validRedactionSelector(selector string) bool {
	return redact.ValidSelector(selector)
}

func portInRange(port string) bool {
	return restclient.PortInRange(port)
}

func resolvedResponseMapping(def ClientOperationDefinition, mapping StatusMapping) ResponseMapping {
	return restclient.ResolvedResponseMapping(def, mapping)
}

func parseDuration(value string, fallback time.Duration) time.Duration {
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return duration
}

func lookupHeaderSchema(schema map[string]interface{}, field string) (interface{}, bool) {
	for name, spec := range schema {
		if strings.EqualFold(name, field) {
			return spec, true
		}
	}
	return nil, false
}

func allowedSignal(signal string, allowed []string) bool {
	for _, candidate := range allowed {
		if signal == candidate {
			return true
		}
	}
	return false
}

func lifecycleSignal(endpoint Endpoint) string {
	if endpoint.LifecycleControl.Signal != "" {
		return endpoint.LifecycleControl.Signal
	}
	return endpoint.Signal
}

func pathSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func mockRoutes(name string, cfg *MockConfig) ([]restmock.Route, error) {
	return restmock.LoadRoutes(name, toMockConfig(cfg))
}

func toMockConfig(cfg *MockConfig) *restmock.Config {
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

func toMockResponses(responses []MockResponse) []restmock.Response {
	out := make([]restmock.Response, 0, len(responses))
	for _, response := range responses {
		out = append(out, restmock.Response{
			Status: response.Status, Headers: response.Headers, Body: response.Body,
		})
	}
	return out
}
