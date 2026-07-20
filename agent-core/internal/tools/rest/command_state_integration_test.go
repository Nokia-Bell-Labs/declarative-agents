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

// TestRESTClient_CommandStateTracerBulletNonAdjacentWithRestart is the rel07.0
// tracer bullet: a three-word chain (embed -> transform -> query) where the query
// word selects the embed word's output by label past the intervening transform
// word — the non-adjacent case previous_result cannot express — and the run is
// suspended and resumed in a fresh process mid-chain before the query executes.
//
// The restart uses InMemoryCheckpoint as the fresh-process analog (the same
// framing the Dolt integration test uses for a fresh adapter): the execution log
// is persisted, live state is dropped, the log is reloaded, and the command-state
// view is rebuilt from the reloaded log. The Dolt Save/Load code path for the
// same rehydration is covered by TestCommandStateViewRehydratesFromDoltLoad and
// the gated cmd/agent Dolt integration test (rel07.0-uc001; srd038 R1-R3; srd028
// R13).
func TestRESTClient_CommandStateTracerBulletNonAdjacentWithRestart(t *testing.T) {
	t.Parallel()

	embedding := []interface{}{0.11, 0.22, 0.33}
	embed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		require.Equal(t, "/api/embed", req.URL.Path)
		writeJSON(w, http.StatusOK, map[string]interface{}{"embedding": embedding})
	}))
	defer embed.Close()

	var transformHit bool
	transform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		transformHit = true
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "transformed"})
	}))
	defer transform.Close()

	var queryBody map[string]interface{}
	var queryPath string
	query := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		queryPath = req.URL.Path
		require.NoError(t, json.NewDecoder(req.Body).Decode(&queryBody))
		writeJSON(w, http.StatusOK, map[string]interface{}{"ids": []interface{}{"doc-7"}})
	}))
	defer query.Close()

	def := tracerDefinition(t, embed.URL, transform.URL, query.URL)

	// Word 1: embed_query produces the vector and carries the input text forward.
	embedResult := threadingCommand(
		resolveThreadingOp(t, def, "ollama", "embed_query"),
		seedParameters(t, map[string]interface{}{"model": "all-minilm", "input": "corpus-a"}),
	).Execute()
	require.Equal(t, core.Signal("RESTResponded"), embedResult.Signal, embedResult.Output)

	// Word 2: an intervening transform word runs next.
	transformResult := threadingCommand(
		resolveThreadingOp(t, def, "worker", "transform_event"),
		embedResult,
	).Execute()
	require.Equal(t, core.Signal("RESTResponded"), transformResult.Signal, transformResult.Output)
	require.True(t, transformHit, "the intervening transform word executed")

	// The loop's execution log after two steps.
	execution := core.Execution{
		{Iteration: 1, CommandName: "embed_query", Result: core.ResultDigest{Output: embedResult.Output}},
		{Iteration: 2, CommandName: "transform_event", Result: core.ResultDigest{Output: transformResult.Output}},
	}

	// Suspend and resume in a fresh process: persist the log, drop live state,
	// reload, and rebuild the command-state view from the reloaded log.
	cp := &core.InMemoryCheckpoint{}
	require.NoError(t, cp.Save(core.Position{}, execution))
	_, restored, err := cp.Load()
	require.NoError(t, err)
	view := core.NewCommandStateView(restored)

	// Word 3: after the restart, query resolves embed_query non-adjacently by
	// label and threads its vector into the request body as a nested array.
	queryCmd := threadingCommand(resolveThreadingOp(t, def, "chroma", "query_collection"), core.Result{})
	queryCmd.(core.CommandStateAware).SetCommandState(view)
	queryResult := queryCmd.Execute()
	require.Equal(t, core.Signal("RESTResponded"), queryResult.Signal, queryResult.Output)

	require.Equal(t, "/query/corpus-a", queryPath)
	require.Equal(t, []interface{}{embedding}, queryBody["query_embeddings"])
	require.EqualValues(t, 5, queryBody["n_results"])
}

func tracerDefinition(t *testing.T, embedURL, transformURL, queryURL string) Definition {
	t.Helper()
	def := Definition{
		Version: "v1",
		Auth:    map[string]AuthProfile{"none": {Type: authNone}},
		Limits:  map[string]LimitProfile{"test": {}},
		Clients: map[string]Client{
			"ollama": {
				BaseURL: embedURL, AuthRef: "none", LimitsRef: "test",
				Operations: map[string]Operation{"embed_query": embedOperation()},
			},
			"worker": {
				BaseURL: transformURL, AuthRef: "none", LimitsRef: "test",
				Operations: map[string]Operation{"transform_event": transformOperation()},
			},
			"chroma": {
				BaseURL: queryURL, AuthRef: "none", LimitsRef: "test",
				Operations: map[string]Operation{"query_collection": commandStateQueryOperation()},
			},
		},
	}
	require.NoError(t, ValidateDefinition(def))
	return def
}

// transformOperation is a self-contained intervening word: it takes no runtime
// params (body_source none) so the prior Result's output does not leak into its
// request, and it does not feed the query word.
func transformOperation() Operation {
	return Operation{
		Method:        http.MethodPost,
		Path:          "/event",
		Params:        RequestBinding{BodySource: bodySourceNone},
		Success:       StatusMapping{Status: []int{200}, Signal: "RESTResponded"},
		Response:      ResponseMapping{Output: map[string]string{"status": "$.status"}},
		SideEffects:   []SideEffect{{Kind: "external_api", State: "read_only"}},
		Reversibility: Reversibility{Classification: "reversible", Undo: "noop"},
	}
}
