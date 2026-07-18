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

// TestRESTClient_ThreadsPreviousResultParams proves body_source previous_result
// selects declared params from a prior Result output through input_mapping, so
// the prior Result's fixed output shape no longer trips the declared-only
// contract (srd028 R12.1, R12.2; AC11).
func TestRESTClient_ThreadsPreviousResultParams(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotBody map[string]interface{}
	query := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotPath = req.URL.Path
		_ = json.NewDecoder(req.Body).Decode(&gotBody)
		writeJSON(w, http.StatusOK, map[string]interface{}{"ids": []interface{}{"doc-1"}})
	}))
	defer query.Close()

	def := threadingDefinition(t, "http://embed.invalid", query.URL)
	op := resolveThreadingOp(t, def, "chroma", "query_collection")

	prior := priorResultOutput(t, map[string]interface{}{
		"mapped":  map[string]interface{}{"embedding": []interface{}{0.1, 0.2, 0.3}},
		"carried": map[string]interface{}{"input": "hello"},
	})
	result := threadingCommand(op, prior).Execute()

	require.Equal(t, core.Signal("RESTResponded"), result.Signal, result.Output)
	require.NotContains(t, result.Output, "not declared")
	require.Equal(t, "/query/hello", gotPath)
	require.Equal(t, []interface{}{[]interface{}{0.1, 0.2, 0.3}}, gotBody["query_embeddings"])
}

// TestRESTClient_CarryForwardEchoesDeclaredInputs proves an operation copies
// selected declared inputs into its Result output under a carried key so a
// downstream word can select them (srd028 R12.3; AC11/AC3).
func TestRESTClient_CarryForwardEchoesDeclaredInputs(t *testing.T) {
	t.Parallel()

	embed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"embedding": []interface{}{0.4, 0.5}})
	}))
	defer embed.Close()

	def := threadingDefinition(t, embed.URL, "http://query.invalid")
	op := resolveThreadingOp(t, def, "ollama", "embed_query")

	result := threadingCommand(op, seedParameters(t, map[string]interface{}{
		"model": "all-minilm", "input": "hello",
	})).Execute()

	require.Equal(t, core.Signal("RESTResponded"), result.Signal, result.Output)
	var output map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	carried, ok := output["carried"].(map[string]interface{})
	require.True(t, ok, result.Output)
	require.Equal(t, "hello", carried["input"])
}

// TestRESTClient_CarryForwardOmittedWithoutConfig keeps the fixed output shape
// unchanged for operations that do not declare carry_forward.
func TestRESTClient_CarryForwardOmittedWithoutConfig(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(issueHandler))
	defer upstream.Close()
	def := clientDefinition(t, upstream.URL, issueClient())

	var output map[string]interface{}
	result := clientCommand(def, InitClientGet, "get", params("1")).Execute()
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.NotContains(t, output, "carried")
}

// TestRESTClient_RejectsPreviousResultThreadingMisuse covers the R12.4 / V28-V30
// rejection cases enforced at definition validation time.
func TestRESTClient_RejectsPreviousResultThreadingMisuse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*Operation)
		message string
	}{
		{
			name:    "undeclared input_mapping target",
			mutate:  func(op *Operation) { op.Params.InputMapping["unknown"] = "$.mapped.value" },
			message: "input_mapping target \"unknown\" is not declared",
		},
		{
			// Activated in GH-278: command_state is structurally valid, but a
			// $.-style selector under it now trips the V32 selector-form rule.
			name:    "command_state body_source with $. selector",
			mutate:  func(op *Operation) { op.Params.BodySource = bodySourceCommandState },
			message: "must be a $from(label).path selector under body_source command_state",
		},
		{
			name:    "input_mapping without previous_result or command_state",
			mutate:  func(op *Operation) { op.Params.BodySource = bodySourceParams },
			message: "input_mapping requires body_source previous_result or command_state",
		},
		{
			name:    "transport authority input_mapping target",
			mutate:  func(op *Operation) { op.Params.InputMapping["url"] = "$.mapped.url" },
			message: "input_mapping target \"url\" cannot set REST authority",
		},
		{
			name:    "transport authority carry_forward entry",
			mutate:  func(op *Operation) { op.Params.CarryForward = []string{"method"} },
			message: "carry_forward entry \"method\" cannot set REST authority",
		},
		{
			name:    "undeclared carry_forward entry",
			mutate:  func(op *Operation) { op.Params.CarryForward = []string{"missing"} },
			message: "carry_forward entry \"missing\" is not declared",
		},
		{
			name:    "unsupported body_source",
			mutate:  func(op *Operation) { op.Params.BodySource = "elsewhere" },
			message: "unsupported body_source \"elsewhere\"",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			def := threadingDefinition(t, "http://embed.invalid", "http://query.invalid")
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

func threadingDefinition(t *testing.T, embedURL, queryURL string) Definition {
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
				Operations: map[string]Operation{"query_collection": queryOperation()},
			},
		},
	}
	require.NoError(t, ValidateDefinition(def))
	return def
}

func embedOperation() Operation {
	return Operation{
		Method: http.MethodPost,
		Path:   "/api/embed",
		Params: RequestBinding{
			BodySchema:   objectSchema([]string{"model", "input"}, map[string]string{"model": "string", "input": "string"}),
			CarryForward: []string{"input"},
		},
		Body:          map[string]interface{}{"model": "{{ params.model }}", "input": "{{ params.input }}"},
		Success:       StatusMapping{Status: []int{200}, Signal: "RESTResponded"},
		Response:      ResponseMapping{Output: map[string]string{"embedding": "$.embedding"}},
		SideEffects:   []SideEffect{{Kind: "external_api", State: "read_only"}},
		Reversibility: Reversibility{Classification: "reversible", Undo: "noop"},
	}
}

func queryOperation() Operation {
	return Operation{
		Method: http.MethodPost,
		Path:   "/query/{collection}",
		Params: RequestBinding{
			Path:         map[string]interface{}{"collection": map[string]interface{}{"type": "string"}},
			BodySchema:   objectSchema([]string{"query_embeddings"}, map[string]string{"query_embeddings": "array"}),
			BodySource:   bodySourcePreviousResult,
			InputMapping: map[string]string{"query_embeddings": "$.mapped.embedding", "collection": "$.carried.input"},
		},
		Body:          map[string]interface{}{"query_embeddings": []interface{}{"{{ params.query_embeddings }}"}, "n_results": 5},
		Success:       StatusMapping{Status: []int{200}, Signal: "RESTResponded"},
		Response:      ResponseMapping{Output: map[string]string{"ids": "$.ids"}},
		SideEffects:   []SideEffect{{Kind: "external_api", State: "read_only"}},
		Reversibility: Reversibility{Classification: "reversible", Undo: "noop"},
	}
}

func objectSchema(required []string, properties map[string]string) map[string]interface{} {
	props := map[string]interface{}{}
	for name, kind := range properties {
		props[name] = map[string]interface{}{"type": kind}
	}
	req := make([]interface{}, 0, len(required))
	for _, name := range required {
		req = append(req, name)
	}
	return map[string]interface{}{"type": "object", "required": req, "properties": props}
}

func resolveThreadingOp(t *testing.T, def Definition, restRef, operation string) ClientOperationDefinition {
	t.Helper()
	collection := NewCollection()
	require.NoError(t, collection.Add(def))
	resolved, err := collection.ResolveClientOperation(ClientToolConfig{RestRef: restRef, Operation: operation})
	require.NoError(t, err)
	return resolved
}

func threadingCommand(op ClientOperationDefinition, prior core.Result) core.Command {
	return ClientBuilder{ToolName: op.OperationName, Init: InitClientInvoke, Operation: op}.Build(prior)
}

// priorResultOutput builds a REST client Result whose output mirrors the fixed
// producer output shape (unwrapped, no parameters key), the shape a downstream
// word receives from the machine.
func priorResultOutput(t *testing.T, output map[string]interface{}) core.Result {
	t.Helper()
	data, err := json.Marshal(output)
	require.NoError(t, err)
	return core.Result{Output: string(data)}
}

func seedParameters(t *testing.T, input map[string]interface{}) core.Result {
	t.Helper()
	data, err := json.Marshal(map[string]interface{}{"parameters": input})
	require.NoError(t, err)
	return core.Result{Output: string(data)}
}

func cloneStringMap(source map[string]string) map[string]string {
	clone := map[string]string{}
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
