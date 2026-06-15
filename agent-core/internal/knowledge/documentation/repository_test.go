// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRepositoryListMatchesDocsIndexBehavior(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeDocFixture(t, root, "VISION.yaml", "title: Vision\n")
	writeDocFixture(t, root, "notes/guide.yaml", "title: Guide\n")
	writeDocFixture(t, root, "specs/config-formats/runtime.yaml", "title: Runtime\n")
	writeDocFixture(t, root, "specs/semantic-models/tool.yaml", "title: Tool\n")
	writeDocFixture(t, root, "specs/software-requirements/srd001-core.yaml", "title: Core\n")
	writeDocFixture(t, root, "specs/test-suites/test-rel00.0.yml", "title: Tests\n")
	writeDocFixture(t, root, "specs/use-cases/rel00.0-uc001.yaml", "title: Use\n")
	writeDocFixture(t, root, "ignored.txt", "ignored\n")

	docs, err := NewRepository(root).List()
	require.NoError(t, err)
	require.Equal(t, []Entry{
		{Path: "specs/config-formats/runtime.yaml", Name: "runtime", Category: "config-format"},
		{Path: "notes/guide.yaml", Name: "guide", Category: "notes"},
		{Path: "VISION.yaml", Name: "VISION", Category: "overview"},
		{Path: "specs/semantic-models/tool.yaml", Name: "tool", Category: "semantic-model"},
		{Path: "specs/software-requirements/srd001-core.yaml", Name: "srd001-core", Category: "srd"},
		{Path: "specs/test-suites/test-rel00.0.yml", Name: "test-rel00.0", Category: "test-suite"},
		{Path: "specs/use-cases/rel00.0-uc001.yaml", Name: "rel00.0-uc001", Category: "use-case"},
	}, docs)
}

func TestRepositoryGetParsesYAMLAndToleratesParseErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeDocFixture(t, root, "valid.yaml", "title: Valid\ncount: 1\n")
	writeDocFixture(t, root, "invalid.yaml", "title: [\n")

	valid, err := NewRepository(root).Get("valid.yaml")
	require.NoError(t, err)
	require.Equal(t, "valid.yaml", valid.Path)
	require.Equal(t, "title: Valid\ncount: 1\n", valid.Raw)
	require.NotNil(t, valid.Content)

	invalid, err := NewRepository(root).Get("invalid.yaml")
	require.NoError(t, err)
	require.Equal(t, "title: [\n", invalid.Raw)
	require.Nil(t, invalid.Content)
}

func TestRepositoryRejectsInvalidAndMissingDocumentPaths(t *testing.T) {
	t.Parallel()

	repo := NewRepository(t.TempDir())
	_, err := repo.Get("")
	require.ErrorIs(t, err, ErrPathRequired)
	_, err = repo.Get("../outside.yaml")
	require.ErrorIs(t, err, ErrInvalidPath)
	_, err = repo.Get("missing.yaml")
	require.ErrorIs(t, err, ErrNotFound)
	_, err = NewRepository("").Get("missing.yaml")
	require.ErrorIs(t, err, ErrNotConfigured)
}

func TestRepositoryListEmptyForMissingOrUnconfiguredRoot(t *testing.T) {
	t.Parallel()

	docs, err := NewRepository("").List()
	require.NoError(t, err)
	require.Empty(t, docs)

	docs, err = NewRepository(filepath.Join(t.TempDir(), "missing")).List()
	require.NoError(t, err)
	require.Empty(t, docs)
}

func TestHandlerPreservesDocsAPIEnvelopeAndErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeDocFixture(t, root, "VISION.yaml", "title: Vision\n")
	mux := http.NewServeMux()
	handler := NewHandler(root)
	mux.HandleFunc("GET /api/v1/docs", handler.List)
	mux.HandleFunc("GET /api/v1/docs/{path...}", handler.Get)

	rec := getDocsRoute(t, mux, "/api/v1/docs")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"data"`)
	require.Contains(t, rec.Body.String(), `"VISION.yaml"`)

	rec = getDocsRoute(t, mux, "/api/v1/docs/VISION.yaml")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"raw":"title: Vision\n"`)

	rec = getDocsRoute(t, mux, "/api/v1/docs/%2e%2e/outside.yaml")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid path", decodeError(t, rec.Body.Bytes()))
}

func TestHandlerServesCuratorSearchValidationAndPatchReview(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeDocFixture(t, root, "VISION.yaml", "id: vision\ntitle: Vision\n")
	writeDocFixture(t, root, "specs/use-cases/rel03.0-uc999-demo.yaml", "title: Demo\n")
	handler := NewServer(HostConfig{DocsDir: root}).Handler()

	search := postDocsJSON(t, handler, "/api/v1/docs/search", `{"query":"vision"}`)
	require.Equal(t, http.StatusOK, search.Code)
	require.Contains(t, search.Body.String(), `"count":1`)

	validate := postDocsJSON(t, handler, "/api/v1/docs/validate", `{"paths":["VISION.yaml"],"strict":true}`)
	require.Equal(t, http.StatusUnprocessableEntity, validate.Code)
	require.Contains(t, validate.Body.String(), `"status":"findings"`)
	require.Contains(t, validate.Body.String(), `"missing_required_field"`)

	suggest := postDocsJSON(t, handler, "/api/v1/docs/suggestions", `{"path":"VISION.yaml","instruction":"Add required fields."}`)
	require.Equal(t, http.StatusAccepted, suggest.Code)
	patchID := decodeField(t, suggest.Body.Bytes(), "patch_id")
	require.NotEmpty(t, patchID)
	require.Contains(t, suggest.Body.String(), "Approval required before any file write.")

	approve := postDocsJSON(t, handler, "/api/v1/docs/patches/"+patchID+"/approve", `{"decided_by":"tester","note":"reviewed"}`)
	require.Equal(t, http.StatusOK, approve.Code)
	require.Contains(t, approve.Body.String(), `"status":"approved_pending_apply"`)
	require.Contains(t, approve.Body.String(), `"applied":false`)

	reopen := postDocsJSON(t, handler, "/api/v1/docs/patches/"+patchID+"/reopen", `{"decided_by":"tester","reason":"more review"}`)
	require.Equal(t, http.StatusOK, reopen.Code)
	require.Contains(t, reopen.Body.String(), `"status":"pending_review"`)
}

func writeDocFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func getDocsRoute(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func postDocsJSON(t *testing.T, handler http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeError(t *testing.T, data []byte) string {
	t.Helper()
	var response apiResponse
	require.NoError(t, json.Unmarshal(data, &response))
	require.NotEmpty(t, response.Error)
	return response.Error
}

func decodeField(t *testing.T, data []byte, field string) string {
	t.Helper()
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &response))
	value, ok := response[field].(string)
	require.True(t, ok, "field %q should be a string", field)
	return value
}
