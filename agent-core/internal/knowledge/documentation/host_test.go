// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
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
