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

// TestMachineRequestSeedFeedsRESTClientFirstWord proves a REST-client word can be
// the first word of a machine_request request machine: the seed exposes the
// mapped request input under "parameters", so runtimeParams reads it and the
// runtime-input authority guard does not see a transport-metadata "method"
// (srd030 seed-parameters contract). The seeded query vector threads into the
// outbound request body.
func TestMachineRequestSeedFeedsRESTClientFirstWord(t *testing.T) {
	t.Parallel()

	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_ = json.NewDecoder(req.Body).Decode(&gotBody)
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	}))
	defer srv.Close()

	def := Definition{
		Version: "v1",
		Auth:    map[string]AuthProfile{"none": {Type: authNone}},
		Limits:  map[string]LimitProfile{"test": {}},
		Clients: map[string]Client{
			"chroma": {
				BaseURL: srv.URL, AuthRef: "none", LimitsRef: "test",
				Operations: map[string]Operation{
					"query": {
						Method: http.MethodPost,
						Path:   "/query",
						Params: RequestBinding{
							BodySchema:   objectSchema([]string{"query_embeddings"}, map[string]string{"query_embeddings": "array"}),
							BodySource:   bodySourcePreviousResult,
							InputMapping: map[string]string{"query_embeddings": "$.query_embeddings"},
						},
						Body:          map[string]interface{}{"query_embeddings": []interface{}{"{{ params.query_embeddings }}"}},
						Success:       StatusMapping{Status: []int{200}, Signal: "QueryResponded"},
						Response:      ResponseMapping{Output: map[string]string{"ok": "$.ok"}},
						SideEffects:   []SideEffect{{Kind: "external_api", State: "read_only"}},
						Reversibility: Reversibility{Classification: "reversible", Undo: "noop"},
					},
				},
			},
		},
	}
	require.NoError(t, ValidateDefinition(def))
	op := resolveThreadingOp(t, def, "chroma", "query")

	// The seed a machine_request produces for the first word: the HTTP method is
	// POST (a forbidden runtime-authority field name), but the seed exposes only
	// the mapped parameters, so a REST-client word consumes it without error.
	seed := requestSeed(MachineRequestRun{
		Server:  "chroma_rag_requests",
		Route:   "query",
		Method:  http.MethodPost,
		Path:    "/api/v1/rag/query",
		Payload: map[string]interface{}{"query_embeddings": []interface{}{0.1, 0.2, 0.3}},
	}, "Seed")

	result := threadingCommand(op, seed).Execute()

	require.Equal(t, core.Signal("QueryResponded"), result.Signal, result.Output)
	require.NotContains(t, result.Output, "cannot set REST authority")
	require.Equal(t, []interface{}{[]interface{}{0.1, 0.2, 0.3}}, gotBody["query_embeddings"])
}

// TestRequestSeedExposesParametersNotTransportMetadata pins the seed shape: the
// mapped request input is under "parameters" and the transport metadata is not
// present, so a request-machine word never sees transport authority.
func TestRequestSeedExposesParametersNotTransportMetadata(t *testing.T) {
	t.Parallel()

	seed := requestSeed(MachineRequestRun{
		Server:  "s",
		Route:   "r",
		Method:  http.MethodPost,
		Path:    "/p",
		Payload: map[string]interface{}{"name": "alice", "path": "docs/VISION.yaml"},
	}, "Seed")

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(seed.Output), &out))

	params, ok := out["parameters"].(map[string]interface{})
	require.True(t, ok, "seed exposes a parameters object")
	require.Equal(t, "alice", params["name"])
	require.Equal(t, "docs/VISION.yaml", params["path"])

	// The forbidden transport authority is absent; the non-authority URL path is
	// kept for adapters. The seed passes the runtime-input authority guard.
	require.NotContains(t, out, "method")
	require.NotContains(t, out, "server")
	require.NotContains(t, out, "route")
	require.NoError(t, ValidateRuntimeInput(out), "the seed passes the runtime-input authority guard")
}
