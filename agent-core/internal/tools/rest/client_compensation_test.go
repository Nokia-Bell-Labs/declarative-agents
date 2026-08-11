// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
	"github.com/stretchr/testify/require"
)

func TestRESTClient_CompensationUndoAndReceipt(t *testing.T) {
	t.Parallel()
	requireRESTClientCompensationUndoReceipt(t)
}

func TestRESTClient_CompensationUndoMemento(t *testing.T) {
	t.Parallel()
	requireRESTClientCompensationUndoReceipt(t)
}

func TestRESTClient_CompensationExecutorRunsFromReceipt(t *testing.T) {
	t.Parallel()
	var restored bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/repos/acme/agent-core/issues/ISS-1" {
			restored = true
			require.Equal(t, http.MethodPatch, req.Method)
			writeJSON(w, http.StatusOK, map[string]interface{}{"title": "restored", "id": "ISS-1"})
			return
		}
		issueHandler(w, req)
	}))
	defer upstream.Close()
	def := clientDefinition(t, upstream.URL, issueClient())
	write := clientCommand(t, def, InitClientSet, "set", params("1", "new"))
	res := write.Execute()
	require.Equal(t, core.Signal("RESTResourceWritten"), res.Signal)
	require.NotEmpty(t, res.Receipt)

	cp := &core.InMemoryCheckpoint{}
	require.NoError(t, cp.Save(core.Position{}, core.Execution{{
		CommandName: write.Name(),
		Result:      commandStateDigest(""),
		Receipt:     res.Receipt,
	}}))
	_, exec, err := cp.Load()
	require.NoError(t, err)
	require.Len(t, exec, 1)

	result := restCompensationExecutor(t, def).CompensateFromReceipt(context.Background(), exec[0].CommandName, exec[0].Receipt)

	require.Equal(t, core.Signal("RESTResourceWritten"), result.Signal, result.Output)
	require.True(t, restored)
}

func TestRESTClient_CompensationExecutorReportsMissingOperation(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(issueHandler))
	defer upstream.Close()
	def := clientDefinition(t, upstream.URL, issueClient())
	write := clientCommand(t, def, InitClientSet, "set", params("1", "new"))
	res := write.Execute()
	require.Equal(t, core.Signal("RESTResourceWritten"), res.Signal)
	receipt := replaceRESTCompensationOperation(t, res.Receipt, "missing")

	result := restCompensationExecutor(t, def).CompensateFromReceipt(context.Background(), write.Name(), receipt)

	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "compensation_lookup")
}

func TestRESTClient_ChromaAddFreshUndoUsesConfiguredDelete(t *testing.T) {
	t.Parallel()

	var paths []string
	var deletedIDs []string
	failDelete := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		paths = append(paths, req.URL.Path)
		switch req.URL.Path {
		case "/collections/collection-1/add":
			writeJSON(w, http.StatusCreated, map[string]interface{}{"status": "added"})
		case "/collections/collection-1/delete":
			require.Equal(t, http.MethodPost, req.Method)
			var body struct {
				IDs []string `json:"ids"`
			}
			require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
			deletedIDs = body.IDs
			if failDelete {
				http.Error(w, "delete failed", http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"status": "deleted"})
		default:
			http.NotFound(w, req)
		}
	}))
	defer upstream.Close()

	collection := NewCollection()
	require.NoError(t, collection.Add(chromaCompensationDefinition(upstream.URL)))
	add, err := collection.ResolveClientOperation(ClientToolConfig{RestRef: "chroma", Operation: "add_records"})
	require.NoError(t, err)
	builder := ClientBuilder{
		ToolName: "chroma_add", Init: InitClientInvoke,
		Operation: add, Definitions: collection,
	}
	execute := func(id string) core.Result {
		return builder.Build(core.Result{Output: jsonOutput(map[string]interface{}{
			"parameters": map[string]interface{}{"collection": "collection-1", "ids": id},
		})}).Execute()
	}

	result := execute("record-1")
	require.Equal(t, core.Signal("DocumentAdded"), result.Signal, result.Output)
	compensation, ok, err := undo.DecodeBoundaryReceipt(result.Receipt)
	require.NoError(t, err)
	require.True(t, ok)
	parameters := compensation.Data["parameters"].(map[string]interface{})
	require.Equal(t, "collection-1", parameters["collection"])
	require.Equal(t, "record-1", parameters["ids"])
	configured := compensation.Data["compensation"].(map[string]interface{})
	require.Equal(t, "delete_records", configured["operation"])

	undoResult := builder.BuildReverser().Undo(core.Result{CommandName: "chroma_add", Receipt: result.Receipt})
	require.Equal(t, core.Signal("DocumentDeleted"), undoResult.Signal, undoResult.Output)
	require.Equal(t, []string{"record-1"}, deletedIDs)
	require.Equal(t, []string{"/collections/collection-1/add", "/collections/collection-1/delete"}, paths)

	failDelete = true
	failedResult := execute("record-2")
	failedUndo := builder.BuildReverser().Undo(core.Result{CommandName: "chroma_add", Receipt: failedResult.Receipt})
	require.Equal(t, core.CommandError, failedUndo.Signal)
	require.Contains(t, failedUndo.Output, "status_mapping")
	require.Contains(t, failedUndo.Output, "status 500 is not mapped")
}

func chromaCompensationDefinition(baseURL string) Definition {
	path := map[string]interface{}{"collection": map[string]interface{}{"type": "string"}}
	body := map[string]interface{}{"ids": []interface{}{"{{ params.ids }}"}}
	params := RequestBinding{Path: path, BodySchema: bodySchemaWithRequired("ids")}
	return Definition{
		Version: "v1",
		Clients: map[string]Client{"chroma": {
			BaseURL: baseURL,
			Operations: map[string]Operation{
				"add_records": {
					Method: http.MethodPost, Path: "/collections/{collection}/add", Params: params, Body: body,
					Success:       StatusMapping{Status: []int{http.StatusCreated}, Signal: "DocumentAdded"},
					Reversibility: Reversibility{Classification: "compensatable", Undo: "delete_records"},
					Compensation:  map[string]interface{}{"operation": "delete_records"},
				},
				"delete_records": {
					Method: http.MethodPost, Path: "/collections/{collection}/delete", Params: params, Body: body,
					Success:       StatusMapping{Status: []int{http.StatusOK}, Signal: "DocumentDeleted"},
					Reversibility: Reversibility{Classification: "irreversible", Undo: "irreversible"},
				},
			},
		}},
	}
}
