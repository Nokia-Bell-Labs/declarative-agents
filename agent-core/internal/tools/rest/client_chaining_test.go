// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// TestRESTClient_ChainThreadsVectorIntoQueryBody proves a REST-client ->
// REST-client chain modeled on embed then query: the first word's output vector
// threads into the second word's request body, and the single-token raw
// passthrough renders the flat vector as a nested array (srd028 R12.1, R12.4,
// R12.5; AC2, AC11).
func TestRESTClient_ChainThreadsVectorIntoQueryBody(t *testing.T) {
	t.Parallel()

	embedding := []interface{}{0.11, 0.22, 0.33}
	embed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		require.Equal(t, "/api/embed", req.URL.Path)
		writeJSON(w, http.StatusOK, map[string]interface{}{"embedding": embedding})
	}))
	defer embed.Close()

	var queryBody map[string]interface{}
	var queryPath string
	query := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		queryPath = req.URL.Path
		require.NoError(t, json.NewDecoder(req.Body).Decode(&queryBody))
		writeJSON(w, http.StatusOK, map[string]interface{}{"ids": []interface{}{"doc-7"}})
	}))
	defer query.Close()

	def := threadingDefinition(t, embed.URL, query.URL)

	embedResult := threadingCommand(
		resolveThreadingOp(t, def, "ollama", "embed_query"),
		seedParameters(t, map[string]interface{}{"model": "all-minilm", "input": "corpus-a"}),
	).Execute()
	require.Equal(t, core.Signal("RESTResponded"), embedResult.Signal, embedResult.Output)

	queryResult := threadingCommand(
		resolveThreadingOp(t, def, "chroma", "query_collection"),
		embedResult,
	).Execute()
	require.Equal(t, core.Signal("RESTResponded"), queryResult.Signal, queryResult.Output)

	require.Equal(t, "/query/corpus-a", queryPath)
	require.Equal(t, []interface{}{embedding}, queryBody["query_embeddings"])
	require.EqualValues(t, 5, queryBody["n_results"])
}
