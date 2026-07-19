// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMonitorProxy_ForwardsToDeclaredUpstream(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-From-Upstream", "yes")
		_, _ = fmt.Fprintf(w, `{"path":%q,"query":%q}`, r.URL.Path, r.URL.RawQuery)
	}))
	defer upstream.Close()

	srv := Server{
		Address: "127.0.0.1:0",
		Queue:   QueueConfig{Name: "mp_ok", Capacity: 4, Timeout: "20ms"},
		Endpoints: map[string]Endpoint{
			"proxy": {
				Method: "GET", Path: "/monitor-proxy/{agent}/{path...}", Binding: bindingMonitorProxy,
				MonitorProxy: &MonitorProxyConfig{Upstreams: map[string]string{"rag0": upstream.URL}},
			},
		},
	}
	state, baseURL := launchRESTServer(t, srv, LimitProfile{})
	defer stopRESTServer(t, state, "mp_ok")

	body := requestBody(t, http.MethodGet, baseURL+"/monitor-proxy/rag0/monitor/state?x=1", "", http.StatusOK)
	require.Contains(t, body, `"path":"/monitor/state"`)
	require.Contains(t, body, `"query":"x=1"`)
}

func TestMonitorProxy_UnknownAgentIs404(t *testing.T) {
	t.Parallel()
	srv := Server{
		Address: "127.0.0.1:0",
		Queue:   QueueConfig{Name: "mp_404", Capacity: 4, Timeout: "20ms"},
		Endpoints: map[string]Endpoint{
			"proxy": {
				Method: "GET", Path: "/monitor-proxy/{agent}/{path...}", Binding: bindingMonitorProxy,
				MonitorProxy: &MonitorProxyConfig{Upstreams: map[string]string{"rag0": "http://127.0.0.1:1"}},
			},
		},
	}
	state, baseURL := launchRESTServer(t, srv, LimitProfile{})
	defer stopRESTServer(t, state, "mp_404")

	requestBody(t, http.MethodGet, baseURL+"/monitor-proxy/nope/monitor/state", "", http.StatusNotFound)
}

func TestValidateDefinition_monitorProxyErrors(t *testing.T) {
	t.Parallel()
	base := func(ep Endpoint) Definition {
		return Definition{
			Version: "v1",
			Servers: map[string]Server{
				"s": {Address: "127.0.0.1:0", Endpoints: map[string]Endpoint{"e": ep}},
			},
		}
	}
	tests := []struct {
		name    string
		def     Definition
		wantErr string
	}{
		{
			name: "empty upstreams",
			def: base(Endpoint{
				Method: "GET", Path: "/monitor-proxy/{agent}/{path...}", Binding: bindingMonitorProxy,
				MonitorProxy: &MonitorProxyConfig{},
				Request:      RequestBinding{Path: map[string]interface{}{"agent": map[string]interface{}{"type": "string"}, "path": map[string]interface{}{"type": "string"}}},
			}),
			wantErr: "non-empty upstreams",
		},
		{
			name: "config with wrong binding",
			def: base(Endpoint{
				Method: "GET", Path: "/x", Binding: bindingHealth,
				MonitorProxy: &MonitorProxyConfig{Upstreams: map[string]string{"a": "http://127.0.0.1:1"}},
			}),
			wantErr: "monitor_proxy config but binding",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.ErrorContains(t, ValidateDefinition(tc.def), tc.wantErr)
		})
	}
}
