// Copyright (c) 2026 Nokia. All rights reserved.

package ollama

import (
	"github.com/stretchr/testify/require"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewAdapter_TrailingSlashInURL(t *testing.T) {
	t.Parallel()
	a, err := NewAdapter("http://127.0.0.1:11434/", "llama3")
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:11434", a.baseURL)
}

func TestNewAdapter_OptionsConfigureClientWithoutNetworkIO(t *testing.T) {
	t.Parallel()
	client := &http.Client{Timeout: 37 * time.Second}

	a, err := NewAdapter("http://127.0.0.1:1", "missing-model", WithHTTPClient(client))

	require.NoError(t, err)
	require.Same(t, client, a.client)
	require.Equal(t, 37*time.Second, a.client.Timeout)
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
