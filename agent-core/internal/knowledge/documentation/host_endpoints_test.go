// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"encoding/json"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestStandaloneServerServesDocsAPIAndSPA(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocFixture(t, docsDir, "VISION.yaml", "title: Vision\n")
	server := NewServer(HostConfig{
		DocsDir: docsDir, ProfilePath: curatorProfilePath(t),
		Assets: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")},
			"asset.js":   &fstest.MapFile{Data: []byte("console.log('docs')")},
		},
	})
	handler := server.Handler()

	rec := getDocsRoute(t, handler, "/api/v1/docs")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"path":"VISION.yaml"`)
	require.Contains(t, rec.Body.String(), `"trace"`)

	rec = getDocsRoute(t, handler, "/api/v1/docs/VISION.yaml")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"raw":"title: Vision\n"`)
	require.Contains(t, rec.Body.String(), `"trace"`)

	rec = getDocsRoute(t, handler, "/docs/VISION.yaml")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "docs app")

	rec = getDocsRoute(t, handler, "/asset.js")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "console.log")
}

func TestStandaloneServerHealth(t *testing.T) {
	t.Parallel()

	handler := NewServer(HostConfig{Assets: fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")},
	}}).Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"ok"`)
}

func TestStandaloneServerStartServesDocsAPI(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocFixture(t, docsDir, "VISION.yaml", "title: Vision\n")
	running, err := NewServer(HostConfig{
		Addr: "127.0.0.1:0", DocsDir: docsDir, ProfilePath: curatorProfilePath(t),
		Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")}},
	}).Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, running.Close()) })

	body := getHTTPBody(t, "http://"+running.Addr+"/api/v1/docs")

	require.Contains(t, body, `"path":"VISION.yaml"`)
	require.Contains(t, body, `"trace"`)
}

func TestStandaloneServerConformanceUsesRESTMachineRequestRoutes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocFixture(t, docsDir, "SPECIFICATIONS.yaml", "id: specs\n")
	handler := NewServer(HostConfig{
		DocsDir: docsDir, ProfilePath: curatorProfilePath(t),
		Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")}},
	}).Handler()

	rec := getDocsRoute(t, handler, "/api/v1/docs/SPECIFICATIONS.yaml")

	require.Equal(t, http.StatusOK, rec.Code)
	trace := responseTrace(t, rec.Body.Bytes())
	require.Equal(t, "docs_runtime_requests", trace["server"])
	require.Equal(t, "document", trace["route"])
	require.Equal(t, "docs-runtime-request", trace["machine"])
	require.Equal(t, "DocumentDetailReady", trace["terminal_signal"])
}

func TestStandaloneServerMachineRequestServesMarkdownDetail(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocFixture(t, docsDir, "bench-documentation-ux-inventory.md", "# Bench Documentation UX Inventory\n\nMarkdown body.\n")
	handler := NewServer(HostConfig{
		DocsDir: docsDir, ProfilePath: curatorProfilePath(t),
		Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")}},
	}).Handler()

	rec := getDocsRoute(t, handler, "/api/v1/docs/bench-documentation-ux-inventory.md")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"raw":"# Bench Documentation UX Inventory\n\nMarkdown body.\n"`)
	require.Contains(t, rec.Body.String(), `"data":"Markdown body."`)
	trace := responseTrace(t, rec.Body.Bytes())
	require.Equal(t, "DocumentDetailReady", trace["terminal_signal"])
}

func TestStandaloneServerAcceptsBrowserHeadersForDocsGET(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocFixture(t, docsDir, "SPECIFICATIONS.yaml", "id: specs\n")
	handler := NewServer(HostConfig{
		DocsDir: docsDir, ProfilePath: curatorProfilePath(t),
		Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")}},
	}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs", nil)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", "http://127.0.0.1:18081/docs")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"path":"SPECIFICATIONS.yaml"`)
	require.Contains(t, rec.Body.String(), `"trace"`)
}

func TestStandaloneServerServesProfileUXConfig(t *testing.T) {
	t.Parallel()
	handler := NewServer(HostConfig{
		ProfilePath: curatorProfilePath(t),
		Assets:      fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")}},
	}).Handler()

	rec := getDocsRoute(t, handler, "/api/v1/ux")

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]UXConfig
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "Docs Runtime UI", body["data"].Title)
	require.Equal(t, "doc_list", uxRoutesByID(body["data"].Routes)["docs_index"].Action)
	require.Equal(t, "doc_get", uxRoutesByID(body["data"].Routes)["docs_detail"].Action)
}

func TestLoadCuratorUXConfigRequiresProfileLocalConfig(t *testing.T) {
	t.Parallel()
	_, err := LoadCuratorUXConfig(filepath.Join(t.TempDir(), "profile.yaml"))

	require.ErrorContains(t, err, "ui/ux.yaml")
}

func TestCuratorUXConfigMatchesRouteAndActionContracts(t *testing.T) {
	t.Parallel()
	profile, err := catalog.LoadProfile(curatorProfilePath(t))
	require.NoError(t, err)
	ux, err := LoadCuratorUXConfig(curatorProfilePath(t))
	require.NoError(t, err)
	collection, err := rest.LoadDefinitions(profile.RestDefinitions, profile.RestConfigDirs)
	require.NoError(t, err)
	defs, err := loadCuratorProfileDefs(profile)
	require.NoError(t, err)
	machine, err := core.LoadMachineSpec(filepath.Join(filepath.Dir(curatorProfilePath(t)), "request-machine.yaml"))
	require.NoError(t, err)

	requireUXRoutesMatchREST(t, ux, collection.Servers["docs_runtime_requests"].Endpoints)
	requireUXActionsSelected(t, ux, toolNames(defs), machineActionNames(machine))
}

func TestMachineRequestFactoriesUseSelectedInits(t *testing.T) {
	t.Parallel()
	builtins := toolregistry.NewBuiltinRegistry()
	registerMachineRequestFactories(builtins, map[string]bool{
		"list_resource":      true,
		"doc_index_response": true,
	}, core.NewRegistry())

	_, ok := builtins.Resolve("list_resource")
	require.True(t, ok)
	_, ok = builtins.Resolve("doc_index_response")
	require.True(t, ok)
	_, ok = builtins.Resolve("read_resource")
	require.False(t, ok)
	_, ok = builtins.Resolve("doc_detail_response")
	require.False(t, ok)
}

func TestStandaloneServerServesContextFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeDocFixture(t, root, "configs/sample.yaml", "name: sample\n")
	writeDocFixture(t, root, "pkg/demo/demo.go", "package demo\n")
	handler := NewServer(HostConfig{
		ConfigsDir: filepath.Join(root, "configs"),
		SourceDir:  root,
		Assets: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")},
		},
	}).Handler()

	config := getDocsRoute(t, handler, "/api/v1/configs/sample.yaml")
	require.Equal(t, http.StatusOK, config.Code)
	require.Contains(t, config.Body.String(), `"raw":"name: sample\n"`)

	source := getDocsRoute(t, handler, "/api/v1/source/pkg/demo/demo.go")
	require.Equal(t, http.StatusOK, source.Code)
	require.Contains(t, source.Body.String(), `"language":"go"`)
}
