// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/filesystem"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckpointRollbackRestoresFileViaPersistedReceipt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(target, []byte("v1"), 0o644))

	// Execute a real write that overwrites the file; capture its opaque receipt.
	writeBuilder := &filesystem.WriteBuilder{Root: dir}
	writeCmd := writeBuilder.Build(core.Result{Output: `{"parameters":{"path":"a.txt","content":"v2"}}`})
	writeRes := writeCmd.Execute()
	require.Equal(t, core.ToolDone, writeRes.Signal)
	require.NotEmpty(t, writeRes.Receipt)
	content, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "v2", string(content))

	// Persist the execution (including the receipt) in the reverter.
	rev := &fakeReverter{}
	require.NoError(t, rev.Save(core.Position{CurrentState: "Working"}, core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "write", Signal: core.ToolDone, Receipt: writeRes.Receipt},
	}))

	// A fresh registry resolves "write" to a builder that implements core.Reverser.
	reg := core.NewRegistry()
	reg.Register(filesystem.WriteToolSpec(), &filesystem.WriteBuilder{Root: dir})

	toIteration := 1
	cmd := (&CheckpointRollbackBuilder{
		Config:     catalog.CheckpointRollbackConfig{ToIteration: &toIteration},
		Checkpoint: rev,
		Registry:   reg,
		RunID:      "run-1",
	}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal)
	require.Contains(t, res.Output, "step=1 write:")
	restored, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "v1", string(restored))
}

func TestRollbackCheckpointExecutesBoundaryCompensation(t *testing.T) {
	t.Parallel()
	var restored bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/repos/acme/agent-core/issues/1":
			require.Equal(t, http.MethodPatch, req.Method)
			writeLifecycleJSON(t, w, http.StatusOK, map[string]interface{}{"id": "ISS-1", "title": "new"})
		case "/repos/acme/agent-core/issues/ISS-1":
			restored = true
			require.Equal(t, http.MethodPatch, req.Method)
			writeLifecycleJSON(t, w, http.StatusOK, map[string]interface{}{"id": "ISS-1", "title": "restored"})
		default:
			http.NotFound(w, req)
		}
	}))
	defer upstream.Close()

	collection, operation := lifecycleRESTCollection(t, upstream.URL)
	writeBuilder := toolrest.ClientBuilder{
		ToolName: "rest_set_issue", Init: toolrest.InitClientSet,
		Operation: operation, Definitions: collection,
	}
	writeRes := writeBuilder.Build(core.Result{Output: `{"parameters":{"owner":"acme","repo":"agent-core","number":"1","title":"new"}}`}).Execute()
	require.Equal(t, core.Signal("RESTResourceWritten"), writeRes.Signal, writeRes.Output)
	require.NotEmpty(t, writeRes.Receipt)

	rev := &fakeReverter{}
	require.NoError(t, rev.Save(core.Position{CurrentState: "Working"}, core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "rest_set_issue", Signal: core.Signal("RESTResourceWritten"), Receipt: writeRes.Receipt},
	}))
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "rest_set_issue", Visibility: core.Internal}, writeBuilder)

	toIteration := 1
	cmd := (&CheckpointRollbackBuilder{
		Config:     catalog.CheckpointRollbackConfig{ToIteration: &toIteration},
		Checkpoint: rev,
		Registry:   reg,
		RunID:      "run-rest",
	}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal, res.Output)
	require.Contains(t, res.Output, "step=1 rest_set_issue:")
	require.True(t, restored)
}

func TestCheckpointRollbackReportsMissingRESTCompensationExecutor(t *testing.T) {
	t.Parallel()
	_, operation := lifecycleRESTCollection(t, "http://127.0.0.1")
	writeBuilder := toolrest.ClientBuilder{
		ToolName: "rest_set_issue", Init: toolrest.InitClientSet, Operation: operation,
	}
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "rest_set_issue", Visibility: core.Internal}, writeBuilder)

	toIteration := 1
	cmd := (&CheckpointRollbackBuilder{
		Config:     catalog.CheckpointRollbackConfig{ToIteration: &toIteration},
		Checkpoint: lifecycleReverterWithRESTReceipt(t),
		Registry:   reg,
		RunID:      "run-rest",
	}).Build(core.Result{})
	res := cmd.Execute()

	// A receipt-walk Undo that fails is a partial rollback, not a clean one:
	// the tool must report CommandError and name the entry whose external
	// effect was not reversed (srd026 R3.7, R6.3, R6.4; GH-491).
	require.Equal(t, core.CommandError, res.Signal, res.Output)
	require.Contains(t, res.Output, "step=1 rest_set_issue: undo failed")
	require.Contains(t, res.Output, "compensation_lookup")
	require.Contains(t, res.Output, "receipt-walk Undo failure")

	var partial *PartialRollbackError
	require.ErrorAs(t, res.Err, &partial)
	require.Equal(t, 0, partial.Reverted)
	require.Len(t, partial.Failures, 1)
	require.Equal(t, "rest_set_issue", partial.Failures[0].CommandName)
}

func TestCheckpointRollbackUndoRequestsCompensation(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointRollbackBuilder{}).Build(core.Result{})

	res := cmd.Undo(core.Result{})

	require.Equal(t, core.CommandError, res.Signal)
	require.Contains(t, res.Output, "resume from the original checkpoint or choose another rollback checkpoint")
}
