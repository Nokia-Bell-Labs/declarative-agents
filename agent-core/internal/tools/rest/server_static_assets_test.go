// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	"github.com/stretchr/testify/require"
)

func TestStaticAssets_literalRouteWinsOverCatchAll(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "file"), []byte("file-body"), 0o644))

	srv := restdef.Server{
		Address: "127.0.0.1:0",
		Queue:   restdef.QueueConfig{Name: "static_prec", Capacity: 4, Timeout: "20ms"},
		Endpoints: map[string]restdef.Endpoint{
			"literal": {
				Method: "GET", Path: "/api/x", Binding: bindingHealth,
			},
			"catchall": {
				Method: "GET", Path: "/api/{path...}",
				Binding: bindingStaticAssets,
				StaticAssets: &restdef.StaticAssetsConfig{
					Root: root,
				},
				Request: restdef.RequestBinding{Path: map[string]interface{}{
					"path": map[string]interface{}{"type": "string"},
				}},
			},
		},
	}
	state, baseURL := launchRESTServer(t, srv, restdef.LimitProfile{})
	defer stopRESTServer(t, state, "static_prec")

	body := requestBody(t, http.MethodGet, baseURL+"/api/x", "", http.StatusOK)
	var health map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &health))
	require.Equal(t, "ok", health["status"])

	fileBody := requestBody(t, http.MethodGet, baseURL+"/api/nested/file", "", http.StatusOK)
	require.Equal(t, "file-body", fileBody)
}

func TestStaticAssets_SPAServesIndexForUnknownPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>spa</html>"), 0o644))

	srv := restdef.Server{
		Address: "127.0.0.1:0",
		Queue:   restdef.QueueConfig{Name: "static_spa", Capacity: 4, Timeout: "20ms"},
		Endpoints: map[string]restdef.Endpoint{
			"ui": {
				Method: "GET", Path: "/ui/{path...}",
				Binding: bindingStaticAssets,
				StaticAssets: &restdef.StaticAssetsConfig{
					Root: root,
					SPA:  true,
				},
				Request: restdef.RequestBinding{Path: map[string]interface{}{
					"path": map[string]interface{}{"type": "string"},
				}},
			},
		},
	}
	state, baseURL := launchRESTServer(t, srv, restdef.LimitProfile{})
	defer stopRESTServer(t, state, "static_spa")

	out := requestBody(t, http.MethodGet, baseURL+"/ui/no/such/route", "", http.StatusOK)
	require.Equal(t, "<html>spa</html>", out)
}

func TestStaticAssets_SPARootServesIndex(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>root</html>"), 0o644))

	srv := restdef.Server{
		Address: "127.0.0.1:0",
		Queue:   restdef.QueueConfig{Name: "static_root", Capacity: 4, Timeout: "20ms"},
		Endpoints: map[string]restdef.Endpoint{
			"ui": {
				Method: "GET", Path: "/ui/{path...}",
				Binding: bindingStaticAssets,
				StaticAssets: &restdef.StaticAssetsConfig{
					Root: root,
					SPA:  true,
				},
				Request: restdef.RequestBinding{Path: map[string]interface{}{
					"path": map[string]interface{}{"type": "string"},
				}},
			},
		},
	}
	state, baseURL := launchRESTServer(t, srv, restdef.LimitProfile{})
	defer stopRESTServer(t, state, "static_root")

	for _, p := range []string{"/ui/", "/ui"} {
		out := requestBody(t, http.MethodGet, baseURL+p, "", http.StatusOK)
		require.Equal(t, "<html>root</html>", out, "path %q should serve the index", p)
	}
}

func TestStaticAssets_exactRouteWinsOverEmptyCatchAll(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("idx"), 0o644))

	srv := restdef.Server{
		Address: "127.0.0.1:0",
		Queue:   restdef.QueueConfig{Name: "exact_vs_catch", Capacity: 4, Timeout: "20ms"},
		Endpoints: map[string]restdef.Endpoint{
			"index": {Method: "GET", Path: "/docs", Binding: bindingHealth},
			"docs": {
				Method: "GET", Path: "/docs/{path...}",
				Binding: bindingStaticAssets,
				StaticAssets: &restdef.StaticAssetsConfig{
					Root: root,
					SPA:  true,
				},
				Request: restdef.RequestBinding{Path: map[string]interface{}{
					"path": map[string]interface{}{"type": "string"},
				}},
			},
		},
	}
	state, baseURL := launchRESTServer(t, srv, restdef.LimitProfile{})
	defer stopRESTServer(t, state, "exact_vs_catch")

	exact := requestBody(t, http.MethodGet, baseURL+"/docs", "", http.StatusOK)
	require.Contains(t, exact, "ok", "/docs must resolve to the literal route, not the empty-tail catch-all")

	nested := requestBody(t, http.MethodGet, baseURL+"/docs/anything", "", http.StatusOK)
	require.Equal(t, "idx", nested)
}

func TestStaticAssets_missingFile404WithoutSPA(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>idx</html>"), 0o644))

	srv := restdef.Server{
		Address: "127.0.0.1:0",
		Queue:   restdef.QueueConfig{Name: "static_404", Capacity: 4, Timeout: "20ms"},
		Endpoints: map[string]restdef.Endpoint{
			"ui": {
				Method: "GET", Path: "/ui/{path...}",
				Binding: bindingStaticAssets,
				StaticAssets: &restdef.StaticAssetsConfig{
					Root: root,
					SPA:  false,
				},
				Request: restdef.RequestBinding{Path: map[string]interface{}{
					"path": map[string]interface{}{"type": "string"},
				}},
			},
		},
	}
	state, baseURL := launchRESTServer(t, srv, restdef.LimitProfile{})
	defer stopRESTServer(t, state, "static_404")

	requestStatus(t, http.MethodGet, baseURL+"/ui/missing.bin", "", http.StatusNotFound)
}

func TestStaticAssets_monitorOpenAPIOmitsStaticCatchAll(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("x"), 0o644))

	state := NewServerState()
	srv := monitorServer("openapi_mix")
	srv.Endpoints["ui_assets"] = restdef.Endpoint{
		Method: "GET", Path: "/ui/{path...}",
		Binding: bindingStaticAssets,
		StaticAssets: &restdef.StaticAssetsConfig{
			Root: root,
			SPA:  true,
		},
		Request: restdef.RequestBinding{Path: map[string]interface{}{
			"path": map[string]interface{}{"type": "string"},
		}},
	}
	def := ServerDefinition{Name: "openapi_mix", Server: srv, Monitor: seededMonitorState()}
	_, baseURL := launchRESTServerDefinition(t, state, def)
	defer stopRESTServer(t, state, "openapi_mix")

	body := requestBody(t, http.MethodGet, baseURL+"/monitor/openapi", "", http.StatusOK)
	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &doc))
	paths, _ := doc["paths"].(map[string]interface{})
	require.NotNil(t, paths)
	_, hasUI := paths["/ui/{path...}"]
	require.False(t, hasUI, "static_assets catch-all must not appear in monitor OpenAPI paths")
	requireMonitorOpenAPIPaths(t, doc)
}

func TestStaticAssets_metadataIncludesEndpointNames(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644))

	srv := restdef.Server{
		Address: "127.0.0.1:0",
		Queue:   restdef.QueueConfig{Name: "static_meta", Capacity: 4, Timeout: "20ms"},
		Endpoints: map[string]restdef.Endpoint{
			"health":   {Method: "GET", Path: "/health", Binding: bindingHealth},
			"metadata": {Method: "GET", Path: "/metadata", Binding: bindingStaticMetadata},
			"assets": {
				Method: "GET", Path: "/files/{path...}",
				Binding:      bindingStaticAssets,
				StaticAssets: &restdef.StaticAssetsConfig{Root: root},
				Request: restdef.RequestBinding{Path: map[string]interface{}{
					"path": map[string]interface{}{"type": "string"},
				}},
			},
		},
	}
	state, baseURL := launchRESTServer(t, srv, restdef.LimitProfile{})
	defer stopRESTServer(t, state, "static_meta")

	meta := requestBody(t, http.MethodGet, baseURL+"/metadata", "", http.StatusOK)
	require.Contains(t, meta, "assets")
	require.Contains(t, meta, "health")
	require.Contains(t, meta, "metadata")
}
