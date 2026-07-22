// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
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
	require.NoError(t, cp.Save(core.Position{}, core.Execution{{CommandName: write.Name(), Receipt: res.Receipt}}))
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
