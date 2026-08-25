// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"encoding/json"
	"net/http"
	"testing"

	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	"github.com/stretchr/testify/require"
)

func TestRedirect_GETReturnsStatusAndLocation(t *testing.T) {
	t.Parallel()
	srv := restdef.Server{
		Address: "127.0.0.1:0",
		Queue:   restdef.QueueConfig{Name: "redir_ok", Capacity: 4, Timeout: "20ms"},
		Endpoints: map[string]restdef.Endpoint{
			"root": {
				Method: "GET", Path: "/", Binding: bindingRedirect,
				Redirect: &restdef.RedirectConfig{Location: "/ui/", Status: http.StatusMovedPermanently},
			},
		},
	}
	state, baseURL := launchRESTServer(t, srv, restdef.LimitProfile{})
	defer stopRESTServer(t, state, "redir_ok")

	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(baseURL + "/")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusMovedPermanently, resp.StatusCode)
	require.Equal(t, "/ui/", resp.Header.Get("Location"))
}

func TestRedirect_default302WhenStatusOmitted(t *testing.T) {
	t.Parallel()
	srv := restdef.Server{
		Address: "127.0.0.1:0",
		Queue:   restdef.QueueConfig{Name: "redir_def", Capacity: 4, Timeout: "20ms"},
		Endpoints: map[string]restdef.Endpoint{
			"root": {
				Method: "GET", Path: "/here", Binding: bindingRedirect,
				Redirect: &restdef.RedirectConfig{Location: "/there"},
			},
		},
	}
	state, baseURL := launchRESTServer(t, srv, restdef.LimitProfile{})
	defer stopRESTServer(t, state, "redir_def")

	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(baseURL + "/here")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, "/there", resp.Header.Get("Location"))
}

func TestRedirect_monitorOpenAPIOmitsRedirectRoute(t *testing.T) {
	t.Parallel()
	state := NewServerState()
	srv := monitorServer("redir_openapi")
	srv.Endpoints["root"] = restdef.Endpoint{
		Method: "GET", Path: "/", Binding: bindingRedirect,
		Redirect: &restdef.RedirectConfig{Location: "/ui/"},
	}
	def := ServerDefinition{Name: "redir_openapi", Server: srv, Monitor: seededMonitorState()}
	_, baseURL := launchRESTServerDefinition(t, state, def)
	defer stopRESTServer(t, state, "redir_openapi")

	body := requestBody(t, http.MethodGet, baseURL+"/monitor/openapi", "", http.StatusOK)
	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &doc))
	paths, _ := doc["paths"].(map[string]interface{})
	require.NotNil(t, paths)
	_, hasRoot := paths["/"]
	require.False(t, hasRoot, "redirect route must not appear in monitor OpenAPI paths")
	requireMonitorOpenAPIPaths(t, doc)
}

func TestValidateDefinition_redirectErrors(t *testing.T) {
	t.Parallel()
	base := func(ep restdef.Endpoint) restdef.Definition {
		return restdef.Definition{
			Version: "v1",
			Servers: map[string]restdef.Server{
				"s": {
					Address:   "127.0.0.1:0",
					Endpoints: map[string]restdef.Endpoint{"e": ep},
				},
			},
		}
	}
	tests := []struct {
		name    string
		def     restdef.Definition
		wantErr string
	}{
		{
			name: "empty location",
			def: base(restdef.Endpoint{
				Method: "GET", Path: "/", Binding: bindingRedirect,
				Redirect: &restdef.RedirectConfig{Location: "  "},
			}),
			wantErr: "non-empty location",
		},
		{
			name: "unsupported status",
			def: base(restdef.Endpoint{
				Method: "GET", Path: "/", Binding: bindingRedirect,
				Redirect: &restdef.RedirectConfig{Location: "/x", Status: 200},
			}),
			wantErr: "301, 302, 303, 307, or 308",
		},
		{
			name: "wrong method",
			def: base(restdef.Endpoint{
				Method: "POST", Path: "/", Binding: bindingRedirect,
				Redirect: &restdef.RedirectConfig{Location: "/x"},
			}),
			wantErr: "requires GET method",
		},
		{
			name: "redirect block with wrong binding",
			def: base(restdef.Endpoint{
				Method: "GET", Path: "/", Binding: bindingHealth,
				Redirect: &restdef.RedirectConfig{Location: "/x"},
			}),
			wantErr: "redirect config but binding",
		},
		{
			name: "signal conflict",
			def: base(restdef.Endpoint{
				Method: "GET", Path: "/", Binding: bindingRedirect, Signal: "Seed",
				Redirect: &restdef.RedirectConfig{Location: "/x"},
			}),
			wantErr: "must not set signal",
		},
		{
			name: "queue conflict",
			def: base(restdef.Endpoint{
				Method: "GET", Path: "/", Binding: bindingRedirect,
				Redirect: &restdef.RedirectConfig{Location: "/x"},
				Queue:    restdef.QueueConfig{Name: "q"},
			}),
			wantErr: "must not set queue",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.ErrorContains(t, ValidateDefinition(tc.def), tc.wantErr)
		})
	}
}
