// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// The compiled provider adapters are gone (srd058 R5.2): the legacy generator
// and planner adapters srd009 removed, and the Ollama and Cohere packages the
// chat dialects replaced. invoke_llm names no provider package.
func TestOllamaMigrationRemovesLegacyAdapterPaths(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..", "..")
	for _, path := range []string{
		"internal/llm/adapter.go",
		"cmd/planner/ollama.go",
		"internal/model/llm/ollama",
		"internal/model/llm/cohere",
	} {
		_, err := os.Stat(filepath.Join(root, path))
		require.ErrorIs(t, err, os.ErrNotExist, path)
	}
	for _, file := range []string{"invoke.go", "invoke_provider.go", "dialect_client.go"} {
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		require.NotContains(t, string(data), "internal/model/llm/ollama", file)
		require.NotContains(t, string(data), "internal/model/llm/cohere", file)
	}
}

// Building invoke_llm performs no network I/O for either shipped provider, and
// the configured call budget sets the HTTP client's timeout (srd009 AC2 as the
// dialect path now meets it).
func TestInvokeLLMBuildsWithoutNetworkIO(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	for _, provider := range []string{"ollama", "cohere"} {
		builder, err := NewInvokeLLMBuilder(catalog.ToolDef{Name: "invoke_llm", Config: map[string]interface{}{
			"provider": provider, "provider_url": server.URL + "/", "model": "m",
			"manifest_state": "Composing", "llm_timeout": 370,
		}}, InvokeLLMFactoryDeps{Ctx: context.Background()})
		require.NoError(t, err, provider)

		client, ok := builder.Client.(*dialectClient)
		require.True(t, ok, provider)
		require.Equal(t, 370*time.Second, client.http.Timeout, provider)
		require.False(t, strings.HasSuffix(client.baseURL, "/"), "a trailing slash is trimmed")
	}
	require.Zero(t, calls.Load(), "construction probes nothing")
}
