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

// TestRESTClient_CommandStateThreadsNonAdjacentStep proves the rest-tool-format
// command_state_threading example validates and executes: a query word reaches
// back past an intervening step to a labeled embed word through body_source
// command_state and $from(embed_query) selectors, and the flat vector renders as
// a nested array through the single-token raw passthrough (srd028 R13; srd038
// R1-R3; rel07.0-uc001).
func TestRESTClient_CommandStateThreadsNonAdjacentStep(t *testing.T) {
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

	def := commandStateDefinition(t, embed.URL, query.URL)

	// Word 1: embed_query produces the vector and carries the input text forward.
	embedResult := threadingCommand(
		resolveThreadingOp(t, def, "ollama", "embed_query"),
		seedParameters(t, map[string]interface{}{"model": "all-minilm", "input": "corpus-a"}),
	).Execute()
	require.Equal(t, core.Signal("RESTResponded"), embedResult.Signal, embedResult.Output)

	// The store view holds embed_query and a later intervening step, so word 3
	// resolves embed_query non-adjacently by label.
	view := core.NewCommandStateView(core.Execution{
		{CommandName: "embed_query", Result: core.ResultDigest{Output: embedResult.Output}},
		{CommandName: "record_event", Result: core.ResultDigest{Output: `{"status":"logged"}`}},
	})

	queryCmd := threadingCommand(resolveThreadingOp(t, def, "chroma", "query_collection"), core.Result{})
	queryCmd.(core.CommandStateAware).SetCommandState(view)
	queryResult := queryCmd.Execute()
	require.Equal(t, core.Signal("RESTResponded"), queryResult.Signal, queryResult.Output)

	require.Equal(t, "/query/corpus-a", queryPath)
	require.Equal(t, []interface{}{embedding}, queryBody["query_embeddings"])
	require.EqualValues(t, 5, queryBody["n_results"])
}

func TestRESTClient_CommandStateDuplicateLabelMostRecentWins(t *testing.T) {
	t.Parallel()
	view := core.NewCommandStateView(core.Execution{
		{CommandName: "embed_query", Result: core.ResultDigest{Output: `{"embedding":[1]}`}},
		{CommandName: "embed_query", Result: core.ResultDigest{Output: `{"embedding":[2]}`}},
	})
	binding := RequestBinding{
		BodySchema:   objectSchema([]string{"query_embeddings"}, map[string]string{"query_embeddings": "array"}),
		BodySource:   bodySourceCommandState,
		InputMapping: map[string]string{"query_embeddings": "$from(embed_query).embedding"},
	}
	selected, err := selectCommandStateParams(view, binding)
	require.NoError(t, err)
	require.Equal(t, []interface{}{float64(2)}, selected["query_embeddings"])
}

func TestRESTClient_CommandStateMissNamesLabel(t *testing.T) {
	t.Parallel()
	view := core.NewCommandStateView(core.Execution{
		{CommandName: "other_step", Result: core.ResultDigest{Output: `{"x":1}`}},
	})
	binding := RequestBinding{
		BodySchema:   objectSchema([]string{"query_embeddings"}, map[string]string{"query_embeddings": "array"}),
		BodySource:   bodySourceCommandState,
		InputMapping: map[string]string{"query_embeddings": "$from(embed_query).embedding"},
	}
	_, err := selectCommandStateParams(view, binding)
	require.ErrorContains(t, err, `no prior step labeled "embed_query"`)
}

func TestRESTClient_CommandStateNoViewConfiguredRejected(t *testing.T) {
	t.Parallel()
	binding := RequestBinding{
		BodySchema:   objectSchema([]string{"query_embeddings"}, map[string]string{"query_embeddings": "array"}),
		BodySource:   bodySourceCommandState,
		InputMapping: map[string]string{"query_embeddings": "$from(embed_query).embedding"},
	}
	_, err := selectCommandStateParams(nil, binding)
	require.ErrorContains(t, err, "not supported until a shared command-state store exists")
}

// TestRESTClient_CommandStateRejectionTable covers the R13.3 / V32 rejections
// enforced at definition validation time: undeclared and authority targets, a
// malformed $from, a $.-style selector under command_state, and a $from selector
// under previous_result.
func TestRESTClient_CommandStateRejectionTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(op *Operation)
		message string
	}{
		{
			name:    "undeclared input_mapping target",
			mutate:  func(op *Operation) { op.Params.InputMapping["unknown"] = "$from(embed_query).embedding" },
			message: "input_mapping target \"unknown\" is not declared",
		},
		{
			name:    "transport authority target",
			mutate:  func(op *Operation) { op.Params.InputMapping["url"] = "$from(embed_query).embedding" },
			message: "input_mapping target \"url\" cannot set REST authority",
		},
		{
			name:    "malformed $from selector",
			mutate:  func(op *Operation) { op.Params.InputMapping["query_embeddings"] = "$from(embed_query" },
			message: "must be a $from(label).path selector under body_source command_state",
		},
		{
			name:    "$. selector under command_state",
			mutate:  func(op *Operation) { op.Params.InputMapping["query_embeddings"] = "$.embedding" },
			message: "must be a $from(label).path selector under body_source command_state",
		},
		{
			name: "$from selector under previous_result",
			mutate: func(op *Operation) {
				op.Params.BodySource = bodySourcePreviousResult
				op.Params.InputMapping["query_embeddings"] = "$from(embed_query).embedding"
			},
			message: "must start with $.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			def := commandStateDefinition(t, "http://embed.invalid", "http://query.invalid")
			client := def.Clients["chroma"]
			op := client.Operations["query_collection"]
			op.Params.InputMapping = cloneStringMap(op.Params.InputMapping)
			tc.mutate(&op)
			client.Operations["query_collection"] = op
			def.Clients["chroma"] = client

			require.ErrorContains(t, ValidateDefinition(def), tc.message)
		})
	}
}

func commandStateDefinition(t *testing.T, embedURL, queryURL string) Definition {
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
			"chroma": {
				BaseURL: queryURL, AuthRef: "none", LimitsRef: "test",
				Operations: map[string]Operation{"query_collection": commandStateQueryOperation()},
			},
		},
	}
	require.NoError(t, ValidateDefinition(def))
	return def
}

func commandStateQueryOperation() Operation {
	return Operation{
		Method: http.MethodPost,
		Path:   "/query/{collection}",
		Params: RequestBinding{
			Path:       map[string]interface{}{"collection": map[string]interface{}{"type": "string"}},
			BodySchema: objectSchema([]string{"query_embeddings"}, map[string]string{"query_embeddings": "array"}),
			BodySource: bodySourceCommandState,
			InputMapping: map[string]string{
				"query_embeddings": "$from(embed_query).mapped.embedding",
				"collection":       "$from(embed_query).carried.input",
			},
		},
		Body:          map[string]interface{}{"query_embeddings": []interface{}{"{{ params.query_embeddings }}"}, "n_results": 5},
		Success:       StatusMapping{Status: []int{200}, Signal: "RESTResponded"},
		Response:      ResponseMapping{Output: map[string]string{"ids": "$.ids"}},
		SideEffects:   []SideEffect{{Kind: "external_api", State: "read_only"}},
		Reversibility: Reversibility{Classification: "reversible", Undo: "noop"},
	}
}
