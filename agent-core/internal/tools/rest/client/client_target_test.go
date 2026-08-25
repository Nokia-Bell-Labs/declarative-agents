// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// TestRESTClient_HostSelectorComposesSelectedAuthority proves a per-item target
// discovered as a bare host or IP reaches the composed authority through one
// normal REST dispatch (srd028 R14.6).
func TestRESTClient_HostSelectorComposesSelectedAuthority(t *testing.T) {
	t.Parallel()

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests++
		require.Equal(t, "/monitor/state", req.URL.Path)
		writeJSON(w, http.StatusOK, map[string]interface{}{"current_state": "Serving"})
	}))
	defer server.Close()

	host, port := splitServerAuthority(t, server.URL)
	def := hostSelectorDefinition(t, "http://127.0.0.1:1", port, AuthProfile{Type: authNone})
	op := resolveThreadingOp(t, def, "agent_monitor", "read_state")
	cmd := threadingCommand(op, core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(discoveredPodState(host))

	result := cmd.Execute()
	require.Equal(t, core.Signal("Polled"), result.Signal, result.Output)
	require.Equal(t, 1, requests)
	require.Contains(t, result.Output, `"selected_authority":"http://`+host+":"+port)
}

// TestRESTClient_HostSelectorRejectsUnsafeValuesBeforeIO proves a resolved host
// carrying transport authority is rejected before any request (srd028 R14.3).
func TestRESTClient_HostSelectorRejectsUnsafeValuesBeforeIO(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		output  string
		message string
	}{
		{name: "missing label", output: "", message: `no prior step labeled "discover_pods"`},
		{name: "non string", output: `{"ip":42}`, message: "resolved to float64, want string"},
		{name: "empty", output: `{"ip":" "}`, message: "resolved to an empty host"},
		{name: "whole URL", output: `{"ip":"http://10.0.0.1"}`, message: "must be a bare host or IP"},
		{name: "path", output: `{"ip":"10.0.0.1/admin"}`, message: "must be a bare host or IP"},
		{name: "userinfo", output: `{"ip":"user:secret@10.0.0.1"}`, message: "must be a bare host or IP"},
		{name: "query", output: `{"ip":"10.0.0.1?x=1"}`, message: "must be a bare host or IP"},
		{name: "carries port", output: `{"ip":"10.0.0.1:9999"}`, message: "must not carry a port"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("request must not be sent for an unsafe selected host")
			}))
			defer server.Close()

			def := hostSelectorDefinition(t, "http://127.0.0.1:1", "18202", AuthProfile{Type: authNone})
			op := resolveThreadingOp(t, def, "agent_monitor", "read_state")
			cmd := threadingCommand(op, core.Result{})
			execution := core.Execution{}
			if tc.output != "" {
				execution = append(execution, core.Entry{
					CommandName: "discover_pods",
					Result:      commandStateDigest(tc.output),
				})
			}
			cmd.(core.CommandStateAware).SetCommandState(core.NewCommandStateView(execution))

			result := cmd.Execute()
			require.Equal(t, core.CommandError, result.Signal, result.Output)
			require.ErrorContains(t, result.Err, tc.message)
			require.Contains(t, result.Output, `"failure_stage":"target_resolution"`)
		})
	}
}

// TestRESTClient_PortSelectorResolvesPerItemPort proves a fleet whose targets do
// not share one port reaches each target's own port, taken from a labeled prior
// output as a string or a JSON number (srd028 R14.6).
func TestRESTClient_PortSelectorResolvesPerItemPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		port func(port string) string
	}{
		{name: "label string", port: func(port string) string { return `"` + port + `"` }},
		{name: "json number", port: func(port string) string { return port }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var requests int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				requests++
				require.Equal(t, "/monitor/state", req.URL.Path)
				writeJSON(w, http.StatusOK, map[string]interface{}{"current_state": "Serving"})
			}))
			defer server.Close()

			host, port := splitServerAuthority(t, server.URL)
			def := portSelectorDefinition(t, []int{atoiPort(t, port)})
			op := resolveThreadingOp(t, def, "agent_monitor", "read_state")
			cmd := threadingCommand(op, core.Result{})
			cmd.(core.CommandStateAware).SetCommandState(discoveredPodPortState(host, tc.port(port)))

			result := cmd.Execute()
			require.Equal(t, core.Signal("Polled"), result.Signal, result.Output)
			require.Equal(t, 1, requests)
			require.Contains(t, result.Output, `"selected_authority":"http://`+host+":"+port)
		})
	}
}

// TestRESTClient_PortSelectorRejectsUnusablePorts proves a resolved port that is
// malformed, out of range, or outside the client's declared ports allowlist is
// rejected before any request (srd028 R14.6, R2.6).
func TestRESTClient_PortSelectorRejectsUnusablePorts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		port    string
		allowed []int
		message string
	}{
		{name: "non numeric", port: `"monitor"`, message: `want a port in 1-65535`},
		{name: "out of range", port: `"70000"`, message: `want a port in 1-65535`},
		{name: "zero", port: `"0"`, message: `want a port in 1-65535`},
		{name: "fractional number", port: `18082.5`, message: "want a whole port number"},
		{name: "wrong type", port: `true`, message: "want a port string or number"},
		{name: "outside allowlist", port: `"18099"`, allowed: []int{18082}, message: `port "18099" is not allowed`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("request must not be sent for an unusable resolved port")
			}))
			defer server.Close()

			def := portSelectorDefinition(t, tc.allowed)
			op := resolveThreadingOp(t, def, "agent_monitor", "read_state")
			cmd := threadingCommand(op, core.Result{})
			cmd.(core.CommandStateAware).SetCommandState(discoveredPodPortState("127.0.0.1", tc.port))

			result := cmd.Execute()
			require.Equal(t, core.CommandError, result.Signal, result.Output)
			require.ErrorContains(t, result.Err, tc.message)
		})
	}
}
func TestRESTClient_HostSelectorRejectionTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(op *Operation)
		message string
	}{
		{
			name:    "host selector without source",
			mutate:  func(op *Operation) { op.BaseURLSource = "" },
			message: "base_url_host_selector requires base_url_source command_state",
		},
		{
			name:    "current-value selector",
			mutate:  func(op *Operation) { op.BaseURLHostSelector = "$.ip" },
			message: "must be a $from(label).path selector under base_url_source command_state",
		},
		{
			name:    "both selector forms",
			mutate:  func(op *Operation) { op.BaseURLSelector = "$from(discover_pods).base_url" },
			message: "declares both base_url_selector and base_url_host_selector",
		},
		{
			name:    "unsupported scheme",
			mutate:  func(op *Operation) { op.BaseURLScheme = "file" },
			message: `unsupported base_url_scheme "file"`,
		},
		{
			name:    "non numeric port",
			mutate:  func(op *Operation) { op.BaseURLPort = "monitor" },
			message: `invalid base_url_port "monitor"`,
		},
		{
			name:    "port out of range",
			mutate:  func(op *Operation) { op.BaseURLPort = "70000" },
			message: `invalid base_url_port "70000"`,
		},
		{
			name: "composed fields without host selector",
			mutate: func(op *Operation) {
				op.BaseURLHostSelector = ""
				op.BaseURLSelector = "$from(discover_pods).base_url"
			},
			message: "require base_url_host_selector",
		},
		{
			name: "both port forms",
			mutate: func(op *Operation) {
				op.BaseURLPortSelector = "$from(discover_pods).port"
			},
			message: "declares both base_url_port and base_url_port_selector",
		},
		{
			name: "current-value port selector",
			mutate: func(op *Operation) {
				op.BaseURLPort = ""
				op.BaseURLPortSelector = "$.port"
			},
			message: "base_url_port_selector \"$.port\" must be a $from(label).path selector",
		},
		{
			name: "port selector without host selector",
			mutate: func(op *Operation) {
				op.BaseURLHostSelector = ""
				op.BaseURLPort = ""
				op.BaseURLSelector = "$from(discover_pods).base_url"
				op.BaseURLPortSelector = "$from(discover_pods).port"
			},
			message: "require base_url_host_selector",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			def := hostSelectorDefinition(t, "http://127.0.0.1:1", "18202", AuthProfile{Type: authNone})
			client := def.Clients["agent_monitor"]
			op := client.Operations["read_state"]
			tc.mutate(&op)
			client.Operations["read_state"] = op
			def.Clients["agent_monitor"] = client

			require.ErrorContains(t, ValidateDefinition(def), tc.message)
		})
	}
}

func hostSelectorDefinition(t *testing.T, configuredURL, port string, auth AuthProfile) Definition {
	t.Helper()
	def := Definition{
		Version: "v1",
		Auth:    map[string]AuthProfile{"selected": auth},
		Limits:  map[string]LimitProfile{"test": {}},
		Clients: map[string]Client{"agent_monitor": {
			BaseURL: configuredURL, AuthRef: "selected", LimitsRef: "test",
			Operations: map[string]Operation{"read_state": {
				Method:              http.MethodGet,
				Path:                "/monitor/state",
				BaseURLSource:       bodySourceCommandState,
				BaseURLHostSelector: "$from(discover_pods).ip",
				BaseURLScheme:       "http",
				BaseURLPort:         port,
				Params:              RequestBinding{BodySource: bodySourceNone},
				Success:             StatusMapping{Status: []int{200}, Signal: "Polled"},
				SideEffects:         []SideEffect{{Kind: "external_api", State: "read_only"}},
				Reversibility:       Reversibility{Classification: "reversible", Undo: "noop"},
			}},
		}},
	}
	require.NoError(t, ValidateDefinition(def))
	return def
}

// portSelectorDefinition declares a monitor read whose host and port both come
// from the discovery step, with allowed narrowing the client's port allowlist.
func portSelectorDefinition(t *testing.T, allowed []int) Definition {
	t.Helper()
	def := Definition{
		Version: "v1",
		Auth:    map[string]AuthProfile{"none": {Type: authNone}},
		Limits:  map[string]LimitProfile{"test": {Network: NetworkPolicy{Ports: allowed}}},
		Clients: map[string]Client{"agent_monitor": {
			BaseURL: "http://127.0.0.1:1", AuthRef: "none", LimitsRef: "test",
			Operations: map[string]Operation{"read_state": {
				Method:              http.MethodGet,
				Path:                "/monitor/state",
				BaseURLSource:       bodySourceCommandState,
				BaseURLHostSelector: "$from(discover_pods).ip",
				BaseURLPortSelector: "$from(discover_pods).port",
				BaseURLScheme:       "http",
				Params:              RequestBinding{BodySource: bodySourceNone},
				Success:             StatusMapping{Status: []int{200}, Signal: "Polled"},
				SideEffects:         []SideEffect{{Kind: "external_api", State: "read_only"}},
				Reversibility:       Reversibility{Classification: "reversible", Undo: "noop"},
			}},
		}},
	}
	require.NoError(t, ValidateDefinition(def))
	return def
}

// discoveredPodPortState views a discovery step that published both the per-item
// host and its port, the shape a pod label carries. rawPort is JSON, so a test
// can present the port as a string or a number.
func discoveredPodPortState(host, rawPort string) core.CommandStateView {
	return core.NewCommandStateView(core.Execution{{
		CommandName: "discover_pods",
		Result:      commandStateDigest(`{"ip":"` + host + `","port":` + rawPort + `}`),
	}})
}

func atoiPort(t *testing.T, port string) int {
	t.Helper()
	number, err := strconv.Atoi(port)
	require.NoError(t, err)
	return number
}

// discoveredPodState views one prior discovery step whose output carries the
// per-item host, the shape a for_each iterator binding presents.
func discoveredPodState(host string) core.CommandStateView {
	return core.NewCommandStateView(core.Execution{{
		CommandName: "discover_pods",
		Result:      commandStateDigest(`{"ip":"` + host + `"}`),
	}})
}

func splitServerAuthority(t *testing.T, rawURL string) (host, port string) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	return parsed.Hostname(), parsed.Port()
}
