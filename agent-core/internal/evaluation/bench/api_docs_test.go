// Copyright (c) 2026 Nokia. All rights reserved.

package bench

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDocsAPIRoutesDelegateToDocumentationPackage(t *testing.T) {
	root := t.TempDir()
	writeBenchDocFixture(t, root, "VISION.yaml", "title: Vision\n")
	writeBenchDocFixture(t, root, "specs/software-requirements/srd001-core.yaml", "title: Core\n")
	handler := NewServer(ServerConfig{DocsDir: root}, nil).Handler()

	rec := getBenchRoute(t, handler, "/api/v1/docs")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"path":"VISION.yaml"`)
	require.Contains(t, rec.Body.String(), `"category":"srd"`)

	rec = getBenchRoute(t, handler, "/api/v1/docs/VISION.yaml")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"raw":"title: Vision\n"`)

	rec = getBenchRoute(t, handler, "/api/v1/docs/%2e%2e/outside.yaml")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), `"error":"invalid path"`)
}

func writeBenchDocFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func getBenchRoute(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
