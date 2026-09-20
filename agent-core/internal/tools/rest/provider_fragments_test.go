// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

// The shipped provider libraries' REST fragments (srd058 R1.3, R1.4, R5.1).
// Each produces one operation under a fixed name on its own client, emits the
// shared success signal and failure taxonomy, and passes REST validation when
// instantiated. The install and library roots are process-scoped, so these
// tests do not run in parallel.

type providerFragment struct {
	provider, file, client, operation, success string
	args                                       string
}

var shippedProviderFragments = []providerFragment{
	{"ollama", "embed-query-fragment.yaml", "query_embedder", "embed_query", "QueryEmbedded", "input_selector: $from(ask).question, id_selector: $from(ask).id, body_source: command_state"},
	{"ollama", "embed-document-fragment.yaml", "document_embedder", "embed_document", "DocumentEmbedded", "input_selector: $.raw, body_source: previous_result"},
	{"cohere", "embed-query-fragment.yaml", "query_embedder", "embed_query", "QueryEmbedded", "input_selector: $from(ask).question, id_selector: $from(ask).id, body_source: command_state"},
	{"cohere", "embed-document-fragment.yaml", "document_embedder", "embed_document", "DocumentEmbedded", "input_selector: $.raw, body_source: previous_result"},
	{"cohere", "rerank-fragment.yaml", "reranker", "rerank", "Reranked", "query_selector: $from(ask).question, documents_selector: $from(search).documents, top_n: 5"},
}

func instantiateProviderFragment(t *testing.T, fragment providerFragment) Collection {
	t.Helper()
	library, err := filepath.Abs(filepath.Join("..", "..", "..", "tools", "providers", fragment.provider))
	require.NoError(t, err)
	corepath.SetLibraryRoots(map[string]string{"providers": library})
	t.Cleanup(func() { corepath.SetLibraryRoots(nil) })
	dir := t.TempDir()
	path := filepath.Join(dir, "provider-rest.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`unit: probe-provider-rest
expand:
- fragment: /opt/providers/`+fragment.file+`
  args: {`+fragment.args+`}
rest: {version: v1}
`), 0o644))
	collection, err := LoadDefinitions([]string{path}, nil)
	require.NoError(t, err, "%s/%s", fragment.provider, fragment.file)
	return collection
}

func TestProviderFragmentsProduceTheirFixedOperations(t *testing.T) {
	for _, fragment := range shippedProviderFragments {
		collection := instantiateProviderFragment(t, fragment)

		client, ok := collection.Clients[fragment.client]
		require.True(t, ok, "%s/%s produces client %s", fragment.provider, fragment.file, fragment.client)
		require.Equal(t, fragment.client+"_limits", client.LimitsRef, "the library bounds its own client")
		operation, ok := client.Operations[fragment.operation]
		require.True(t, ok, "%s/%s produces operation %s", fragment.provider, fragment.file, fragment.operation)
		require.Equal(t, fragment.success, operation.Success.Signal)
		failures := map[int]string{}
		for _, failure := range operation.Failures {
			for _, status := range failure.Status {
				failures[status] = failure.Signal
			}
		}
		require.Equal(t, map[int]string{
			401: "ProviderUnauthorized", 403: "ProviderUnauthorized", 429: "ProviderThrottled",
			500: "ProviderUnavailable", 502: "ProviderUnavailable", 503: "ProviderUnavailable", 504: "ProviderUnavailable",
		}, failures, "%s/%s declares the shared failure taxonomy", fragment.provider, fragment.file)
	}
}

func TestProviderFragmentsNameCredentialsOnly(t *testing.T) {
	for _, fragment := range shippedProviderFragments {
		collection := instantiateProviderFragment(t, fragment)
		auth := collection.Auth[collection.Clients[fragment.client].AuthRef]
		switch fragment.provider {
		case "ollama":
			require.Equal(t, "none", auth.Type)
		case "cohere":
			require.Equal(t, "bearer", auth.Type)
			require.Equal(t, "COHERE_API_KEY", auth.TokenRef)
		}
	}
}

func TestCohereRerankFragmentTakesItsBudget(t *testing.T) {
	collection := instantiateProviderFragment(t, shippedProviderFragments[4])

	operation := collection.Clients["reranker"].Operations["rerank"]
	require.Equal(t, 5, operation.Body["top_n"], "an integer parameter fills an integer field")
	require.Equal(t, "rerank-v3.5", operation.Body["model"])
}

// A deployment points a library at its provider through the environment the
// library names, so rebinding the library needs no agent edit (srd058 AC1).
func TestProviderFragmentEndpointsFollowTheEnvironment(t *testing.T) {
	t.Setenv("OLLAMA_URL", "http://ollama.mesh.svc:11434")
	t.Setenv("COHERE_API_URL", "https://cohere.gateway.example")

	ollama := instantiateProviderFragment(t, shippedProviderFragments[0])
	cohere := instantiateProviderFragment(t, shippedProviderFragments[2])

	require.Equal(t, "http://ollama.mesh.svc:11434", ollama.Clients["query_embedder"].BaseURL)
	require.Equal(t, "https://cohere.gateway.example", cohere.Clients["query_embedder"].BaseURL)
}

func TestEmbedFragmentsCarryTheTextAndId(t *testing.T) {
	for _, fragment := range shippedProviderFragments[:4] {
		collection := instantiateProviderFragment(t, fragment)

		params := collection.Clients[fragment.client].Operations[fragment.operation].Params
		require.Equal(t, []string{"input", "id"}, params.CarryForward, "%s/%s", fragment.provider, fragment.file)
		require.Contains(t, params.InputMapping, "id")
	}
}
