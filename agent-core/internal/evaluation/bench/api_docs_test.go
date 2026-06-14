// Copyright (c) 2026 Nokia. All rights reserved.

package bench

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDocsAPIRoutesAreNotMounted(t *testing.T) {
	t.Parallel()

	handler := NewServer(ServerConfig{}, nil).Handler()

	require.Equal(t, http.StatusNotFound, getBenchRoute(t, handler, "/api/v1/docs").Code)
	require.Equal(t, http.StatusNotFound, getBenchRoute(t, handler, "/api/v1/docs/VISION.yaml").Code)
}

func getBenchRoute(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
