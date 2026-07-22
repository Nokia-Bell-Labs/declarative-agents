// Copyright (c) 2026 Nokia. All rights reserved.

package ollama

import (
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewAdapter_ModelFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(tagsHandler([]string{"llama3:latest", "mistral:7b"}))
	defer srv.Close()

	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.NoError(t, err)
	require.NotNil(t, a)
	require.Equal(t, "llama3", a.Model())
}

func TestNewAdapter_ModelFoundExactTag(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(tagsHandler([]string{"llama3:latest", "llama3:8b"}))
	defer srv.Close()

	a, err := NewAdapter(srv.URL, "llama3:8b", WithHTTPClient(srv.Client()))
	require.NoError(t, err)
	require.NotNil(t, a)
}

func TestNewAdapter_ModelNotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(tagsHandler([]string{"mistral:7b"}))
	defer srv.Close()

	_, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.Error(t, err)
	require.Contains(t, err.Error(), `model "llama3" is not available locally`)
	require.Contains(t, err.Error(), "ollama pull llama3")
}

func TestNewAdapter_CaseInsensitiveMatch(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(tagsHandler([]string{"Llama3:Latest"}))
	defer srv.Close()

	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.NoError(t, err)
	require.NotNil(t, a)
}

func TestNewAdapter_ConnectionRefused(t *testing.T) {
	t.Parallel()
	_, err := NewAdapter("http://127.0.0.1:1", "llama3", WithHTTPClient(&http.Client{}))
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to connect to Ollama")
}

func TestNewAdapter_BadHTTPStatus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.Error(t, err)
	require.Contains(t, err.Error(), "status 500")
}

func TestNewAdapter_MalformedJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	_, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse /api/tags response")
}

func TestNewAdapter_EmptyModelList(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(tagsHandler([]string{}))
	defer srv.Close()

	_, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not available locally")
}

func TestNewAdapter_TrailingSlashInURL(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(tagsHandler([]string{"llama3:latest"}))
	defer srv.Close()

	a, err := NewAdapter(srv.URL+"/", "llama3", WithHTTPClient(srv.Client()))
	require.NoError(t, err)
	require.NotNil(t, a)
}

func TestNewAdapter_OptionsConfigureClientAndSkipModelCheck(t *testing.T) {
	t.Parallel()
	client := &http.Client{Timeout: 37 * time.Second}

	a, err := NewAdapter("http://127.0.0.1:1", "missing-model", WithHTTPClient(client), WithSkipModelCheck())

	require.NoError(t, err)
	require.Same(t, client, a.client)
	require.Equal(t, 37*time.Second, a.client.Timeout)
	require.True(t, a.skipModelCheck)
}

func TestOllamaMigrationRemovesLegacyAdapterPaths(t *testing.T) {
	t.Parallel()
	root := filepath.Clean("../../../../")

	legacyPaths := []string{
		filepath.Join(root, "internal/llm/adapter.go"),
		filepath.Join(root, "cmd/planner/ollama.go"),
	}
	for _, path := range legacyPaths {
		_, err := os.Stat(path)
		require.ErrorIs(t, err, os.ErrNotExist, path)
	}

	assertFileContains(t, filepath.Join(root, "internal/tools/llm/invoke.go"), "internal/model/llm/ollama")
}

func TestMatchModel(t *testing.T) {
	t.Parallel()
	standard := []modelEntry{{Name: "llama3:latest"}, {Name: "llama3:8b"}}
	tests := []struct {
		name    string
		request string
		models  []modelEntry
		want    bool
	}{
		{name: "bare name matches any tag", request: "llama3", models: standard, want: true},
		{name: "exact tag", request: "llama3:8b", models: standard, want: true},
		{name: "tag mismatch", request: "llama3:70b", models: standard},
		{name: "bare case insensitive", request: "LLAMA3", models: []modelEntry{{Name: "Llama3:Latest"}}, want: true},
		{name: "tag case insensitive", request: "llama3:latest", models: []modelEntry{{Name: "Llama3:Latest"}}, want: true},
		{name: "bare entry", request: "llama3", models: []modelEntry{{Name: "llama3"}}, want: true},
		{name: "different model", request: "qwen", models: standard},
		{name: "nil inventory", request: "llama3", models: nil},
		{name: "empty inventory", request: "llama3", models: []modelEntry{}},
		{name: "empty request", request: "", models: standard},
		{name: "whitespace request", request: " ", models: standard},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, matchModel(tt.request, tt.models))
		})
	}
}
