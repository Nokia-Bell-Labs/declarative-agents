// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/rest"
)

func TestStandaloneServerServesDocsAPIAndSPA(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeDocFixture(t, root, "VISION.yaml", "title: Vision\n")
	server := NewServer(HostConfig{
		DocsDir: root,
		Assets: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")},
			"asset.js":   &fstest.MapFile{Data: []byte("console.log('docs')")},
		},
	})
	handler := server.Handler()

	rec := getDocsRoute(t, handler, "/api/v1/docs")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"path":"VISION.yaml"`)

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

func TestStandaloneServerRunsActionsThroughWorkflowRunner(t *testing.T) {
	t.Parallel()

	handler := NewServer(HostConfig{
		Workflow: stubWorkflowRunner{},
		Assets: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")},
		},
	}).Handler()

	rec := postDocsJSON(t, handler, "/api/v1/actions", `{"type":"doc_validate","params":{"paths":["VISION.yaml"]}}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"tool":"doc_validate"`)
	require.Contains(t, rec.Body.String(), `"signal":"RESTResponded"`)
}

func TestProfileWorkflowRunnerDispatchesConfiguredRESTTool(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeDocFixture(t, root, "VISION.yaml", "title: Vision\n")
	apiServer := httptest.NewServer(NewServer(HostConfig{
		DocsDir: root,
		Assets: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("<html>docs app</html>")},
		},
	}).Handler())
	defer apiServer.Close()

	collection, err := rest.LoadDefinitions([]string{curatorRestPath(t)}, nil)
	require.NoError(t, err)
	client := collection.Clients["documentation"]
	client.BaseURL = apiServer.URL
	collection.Clients["documentation"] = client
	collection.Limits["local_docs_api"] = rest.LimitProfile{}

	defs, err := catalog.LoadToolDefs(curatorDeclarationsPath(t))
	require.NoError(t, err)
	runner, err := NewProfileWorkflowRunnerFromDefs(collection, defs)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/actions", strings.NewReader(`{"type":"doc_get","params":{"path":"VISION.yaml"}}`))
	result, err := runner.Run(req)

	require.NoError(t, err)
	require.Equal(t, "doc_get", result.Tool)
	require.Equal(t, "RESTResourceRead", result.Signal)
	data := result.Data.(map[string]interface{})
	require.Equal(t, "title: Vision\n", data["raw"])
}

type stubWorkflowRunner struct{}

func (stubWorkflowRunner) Run(r *http.Request) (ActionResponse, error) {
	defer r.Body.Close()
	return ActionResponse{
		Data: map[string]interface{}{"status": "valid"},
		Tool: "doc_validate", Signal: "RESTResponded",
	}, nil
}

func curatorDeclarationsPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRootFromDocsTest(t), "agents", "knowledge-manager", "documentation-curator", "declarations.yaml")
}

func curatorRestPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRootFromDocsTest(t), "agents", "knowledge-manager", "documentation-curator", "rest.yaml")
}

func repoRootFromDocsTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}
