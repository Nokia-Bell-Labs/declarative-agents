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
	"os"
	"path/filepath"
	"strings"
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
	requireUXActionRoutesMatchREST(t, ux, collection.Servers["docs_runtime_requests"].Endpoints)
	requireUXActionsSelected(t, ux, toolNames(defs), machineActionNames(machine))
}

func TestMachineRequestFactoriesUseSelectedInits(t *testing.T) {
	t.Parallel()
	builtins := toolregistry.NewBuiltinRegistry()
	registerMachineRequestFactories(builtins, map[string]bool{
		"list_resource":      true,
		"doc_index_response": true,
	}, core.NewRegistry(), rest.Collection{})

	_, ok := builtins.Resolve("list_resource")
	require.True(t, ok)
	_, ok = builtins.Resolve("doc_index_response")
	require.True(t, ok)
	_, ok = builtins.Resolve("read_resource")
	require.False(t, ok)
	_, ok = builtins.Resolve("doc_detail_response")
	require.False(t, ok)
}

func TestMachineRequestFactoriesRegisterSelectedRESTClients(t *testing.T) {
	t.Parallel()
	builtins := toolregistry.NewBuiltinRegistry()
	registerMachineRequestFactories(builtins, map[string]bool{
		rest.InitClientInvoke: true,
	}, core.NewRegistry(), rest.Collection{})

	_, ok := builtins.Resolve(rest.InitClientInvoke)
	require.True(t, ok)
}

func TestStandaloneServerProxiesValidateActionThroughRequestMachine(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocFixture(t, docsDir, "VISION.yaml", "title: Vision\n")
	docs := NewHandler(docsDir)
	internalAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/docs/validate", r.URL.Path)
		docs.Validate(w, r)
	}))
	t.Cleanup(internalAPI.Close)
	profilePath := curatorProfileWithRESTAddress(t, internalAPI.URL)
	handler := NewServer(HostConfig{
		DocsDir: docsDir, ProfilePath: profilePath,
		Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")}},
	}).Handler()
	t.Cleanup(func() {
		closer, ok := handler.(interface{ Close() error })
		require.True(t, ok)
		require.NoError(t, closer.Close())
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/actions/validate",
		strings.NewReader(`{"paths":["VISION.yaml"],"strict":false}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"status":"findings"`)
	trace := responseTrace(t, rec.Body.Bytes())
	require.Equal(t, "docs_runtime_requests", trace["server"])
	require.Equal(t, "validate_action", trace["route"])
	require.Equal(t, "docs-runtime-request", trace["machine"])
	require.Equal(t, "RESTResponded", trace["terminal_signal"])
}

func TestStandaloneServerProxiesPatchActionsThroughRequestMachines(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	writeDocFixture(t, docsDir, "VISION.yaml", "title: Vision\n")
	docs := NewHandler(docsDir)
	internalMux := http.NewServeMux()
	internalMux.HandleFunc("POST /api/v1/docs/suggestions", docs.Suggest)
	internalMux.HandleFunc("POST /api/v1/docs/patches/{patch_id}/approve", docs.Approve)
	internalMux.HandleFunc("POST /api/v1/docs/patches/{patch_id}/reject", docs.Reject)
	internalAPI := httptest.NewServer(internalMux)
	t.Cleanup(internalAPI.Close)
	profilePath := curatorProfileWithRESTAddress(t, internalAPI.URL)
	handler := NewServer(HostConfig{
		DocsDir: docsDir, ProfilePath: profilePath,
		Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")}},
	}).Handler()
	t.Cleanup(func() {
		closer, ok := handler.(interface{ Close() error })
		require.True(t, ok)
		require.NoError(t, closer.Close())
	})

	approvePatchID := suggestPatchThroughRequestMachine(t, handler, "Clarify the vision")
	approve := postDocsJSON(t, handler, "/api/v1/actions/patches/"+approvePatchID+"/approve",
		`{"decided_by":"reviewer","note":"ready"}`)
	require.Equal(t, http.StatusOK, approve.Code, approve.Body.String())
	requireActionMachineTrace(t, approve.Body.Bytes(), "approve_action", "RESTResponded")
	approveData := actionResponseData(t, approve.Body.Bytes())
	require.Equal(t, approvePatchID, approveData["patch_id"])
	require.Equal(t, "approved_pending_apply", approveData["status"])
	require.Equal(t, "reviewer", approveData["decided_by"])

	rejectPatchID := suggestPatchThroughRequestMachine(t, handler, "Remove ambiguity")
	reject := postDocsJSON(t, handler, "/api/v1/actions/patches/"+rejectPatchID+"/reject",
		`{"decided_by":"reviewer","reason":"needs evidence"}`)
	require.Equal(t, http.StatusOK, reject.Code, reject.Body.String())
	requireActionMachineTrace(t, reject.Body.Bytes(), "reject_action", "RESTResponded")
	rejectData := actionResponseData(t, reject.Body.Bytes())
	require.Equal(t, rejectPatchID, rejectData["patch_id"])
	require.Equal(t, "rejected", rejectData["status"])
	require.Equal(t, "reviewer", rejectData["decided_by"])

	for _, action := range []struct {
		name string
		body string
	}{
		{name: "approve", body: `{"decided_by":"reviewer","note":"missing patch"}`},
		{name: "reject", body: `{"decided_by":"reviewer","reason":"missing patch"}`},
	} {
		t.Run("missing_"+action.name, func(t *testing.T) {
			missing := postDocsJSON(t, handler,
				"/api/v1/actions/patches/patch-does-not-exist/"+action.name,
				action.body)
			require.Equal(t, http.StatusNotFound, missing.Code, missing.Body.String())
			require.Contains(t, missing.Body.String(), `"error":"patch_missing"`)
			requireActionMachineTrace(t, missing.Body.Bytes(), action.name+"_action", "RESTMissing")
		})
	}
}

func TestStandaloneServerRejectsLegacyActionEnvelope(t *testing.T) {
	t.Parallel()
	handler := NewServer(HostConfig{
		Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")}},
	}).Handler()

	rec := postDocsJSON(t, handler, "/api/v1/actions",
		`{"type":"doc_validate","params":{"paths":["VISION.yaml"]}}`)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func suggestPatchThroughRequestMachine(t *testing.T, handler http.Handler, instruction string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"path": "VISION.yaml", "instruction": instruction, "context": "runtime coverage",
	})
	require.NoError(t, err)
	rec := postDocsJSON(t, handler, "/api/v1/actions/suggest", string(body))
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	requireActionMachineTrace(t, rec.Body.Bytes(), "suggest_action", "RESTAccepted")
	data := actionResponseData(t, rec.Body.Bytes())
	patchID, _ := data["patch_id"].(string)
	require.NotEmpty(t, patchID)
	require.Equal(t, "VISION.yaml", data["path"])
	return patchID
}

func requireActionMachineTrace(t *testing.T, body []byte, route, terminalSignal string) {
	t.Helper()
	trace := responseTrace(t, body)
	require.Equal(t, "docs_runtime_requests", trace["server"])
	require.Equal(t, route, trace["route"])
	require.Equal(t, "docs-runtime-request", trace["machine"])
	require.Equal(t, terminalSignal, trace["terminal_signal"])
}

func actionResponseData(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var response struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &response))
	require.NotNil(t, response.Data)
	return response.Data
}

func curatorProfileWithRESTAddress(t *testing.T, baseURL string) string {
	t.Helper()
	profilePath := writeDocsRuntimeProfile(t)
	fixturePath := curatorRestPath(t)
	data, err := os.ReadFile(fixturePath)
	require.NoError(t, err)
	port := strings.TrimPrefix(baseURL, "http://127.0.0.1:")
	data = []byte(strings.ReplaceAll(string(data), "18081", port))
	restPath := filepath.Join(filepath.Dir(profilePath), "rest.yaml")
	require.NoError(t, os.WriteFile(restPath, data, 0o644))
	openAPIData, err := os.ReadFile(filepath.Join(filepath.Dir(fixturePath), "openapi.yaml"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(profilePath), "openapi.yaml"), openAPIData, 0o644))
	profileData, err := os.ReadFile(profilePath)
	require.NoError(t, err)
	profileData = []byte(strings.Replace(string(profileData), fixturePath, restPath, 1))
	require.NoError(t, os.WriteFile(profilePath, profileData, 0o644))
	return profilePath
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
