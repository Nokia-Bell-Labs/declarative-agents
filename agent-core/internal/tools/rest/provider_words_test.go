// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/credentials"
)

// The provider words the embed and rerank stages run (srd058 R1.4, R4.1),
// built over each shipped library's REST fragment against a fake provider:
// success emits the kind's signal and every error status the shared failure
// taxonomy, so a stage routes the same signals whichever library is bound.

type providerWord struct {
	units, word, fragment, args, input, success string
}

var providerWords = map[string]providerWord{
	"embed_query": {"embed-query-words.yaml", "embed_query", "embed-query-fragment.yaml",
		"input_selector: $.text, body_source: previous_result", `{"text":"what is here"}`, "QueryEmbedded"},
	"embed_document": {"embed-document-words.yaml", "embed_document", "embed-document-fragment.yaml",
		"input_selector: $.text, body_source: previous_result", `{"text":"a document"}`, "DocumentEmbedded"},
	"rerank": {"rerank-words.yaml", "rerank", "rerank-fragment.yaml",
		"query_selector: $.query, documents_selector: $.documents, body_source: previous_result",
		`{"query":"q","documents":["a","b"]}`, "Reranked"},
}

func buildProviderWord(t *testing.T, provider string, word providerWord, baseURL string) core.Builder {
	t.Helper()
	agentCore, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	corepath.SetLibraryRoots(map[string]string{"providers": providerLibrary(agentCore, provider)})
	t.Cleanup(func() { corepath.SetLibraryRoots(nil) })
	path := filepath.Join(t.TempDir(), "provider-rest.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`unit: probe-provider-rest
instantiate:
- fragment: /opt/providers/`+word.fragment+`
  args: {base_url: "`+baseURL+`", `+word.args+`}
rest: {version: v1}
`), 0o644))
	collection, err := LoadDefinitions([]string{path}, nil)
	require.NoError(t, err)
	defs, err := catalog.LoadToolDefs(filepath.Join(agentCore, "tools", "units", word.units))
	require.NoError(t, err)
	var def catalog.ToolDef
	for _, candidate := range defs {
		if candidate.Name == word.word {
			def = candidate
		}
	}
	require.Equal(t, word.word, def.Name)
	builder, err := newClientBuilder(def, InitClientInvoke, FactoryDeps{
		Definitions: collection, CredentialResolver: credentials.Static{
			"COHERE_API_KEY": "test-key", "OPENAI_API_KEY": "test-key",
		},
	})
	require.NoError(t, err, "the word's emits cover the %s fragment's signals", provider)
	return builder
}

// providerLibrary is a shipped library under tools/providers, or the synthetic
// OpenAI-shaped one under testdata, which ships with no Go (srd058 R5.3).
func providerLibrary(agentCore, provider string) string {
	if provider == "openai-shaped" {
		return filepath.Join(agentCore, "testdata", "providers", provider)
	}
	return filepath.Join(agentCore, "tools", "providers", provider)
}

func TestProviderFailureSignalsRouted(t *testing.T) {
	cases := map[string][]string{
		"ollama":        {"embed_query", "embed_document"},
		"cohere":        {"embed_query", "embed_document", "rerank"},
		"openai-shaped": {"embed_query", "embed_document"},
	}
	statuses := map[int]string{
		http.StatusUnauthorized:       "ProviderUnauthorized",
		http.StatusTooManyRequests:    "ProviderThrottled",
		http.StatusServiceUnavailable: "ProviderUnavailable",
	}
	for provider, words := range cases {
		for _, name := range words {
			word := providerWords[name]
			var status atomic.Int32
			status.Store(http.StatusOK)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(int(status.Load()))
				_, _ = w.Write([]byte(`{"embedding":[0.5],"embeddings":{"float":[[0.5]]},"data":[{"embedding":[0.5]}],"results":[{"index":1,"relevance_score":0.9}]}`))
			}))
			builder := buildProviderWord(t, provider, word, server.URL)

			result := builder.Build(core.Result{Output: word.input}).Execute()
			require.Equal(t, core.Signal(word.success), result.Signal, "%s %s on 200: %s", provider, name, result.Output)
			for code, signal := range statuses {
				status.Store(int32(code))
				result = builder.Build(core.Result{Output: word.input}).Execute()
				require.Equal(t, core.Signal(signal), result.Signal, "%s %s on %d", provider, name, code)
			}
			server.Close()
		}
	}
}

// The synthetic library's embedding sits in the first member of the data
// array, and the response selector indexes it (srd038 R2.20), so the word
// publishes the flat vector itself with no Go written for the provider.
func TestSyntheticProviderEmbeddingIsIndexedOutOfItsArray(t *testing.T) {
	var body map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/embeddings", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		_, _ = w.Write([]byte(`{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.25,0.75]}]}`))
	}))
	defer server.Close()
	builder := buildProviderWord(t, "openai-shaped", providerWords["embed_query"], server.URL)

	result := builder.Build(core.Result{Output: `{"text":"what is here"}`}).Execute()

	require.Equal(t, core.Signal("QueryEmbedded"), result.Signal, result.Output)
	require.Equal(t, "what is here", body["input"])
	var output struct {
		Mapped struct {
			Embedding []float64 `json:"embedding"`
		} `json:"mapped"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Equal(t, []float64{0.25, 0.75}, output.Mapped.Embedding)
}
