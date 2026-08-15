// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package lifecycle

import (
	"context"
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
	"time"
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

func TestCheckpointRollbackCancelsInFlightRESTCompensation(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		close(started)
		select {
		case <-req.Context().Done():
		case <-release:
			writeLifecycleJSON(t, w, http.StatusOK, map[string]interface{}{"id": "1", "title": "late"})
		}
	}))
	defer func() {
		close(release)
		upstream.Close()
	}()

	collection, operation := lifecycleRESTCollection(t, upstream.URL)
	writeBuilder := toolrest.ClientBuilder{
		ToolName: "rest_set_issue", Init: toolrest.InitClientSet,
		Operation: operation, Definitions: collection,
	}
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "rest_set_issue", Visibility: core.Internal}, writeBuilder)

	target := 1
	cmd := (&CheckpointRollbackBuilder{
		Config:     catalog.CheckpointRollbackConfig{ToIteration: &target},
		Checkpoint: lifecycleReverterWithRESTReceipt(t),
		Registry:   reg,
		RunID:      "run-cancel",
	}).Build(core.Result{})
	contextual, ok := cmd.(core.ContextCommand)
	require.True(t, ok)

	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan core.Result, 1)
	go func() {
		results <- contextual.ExecuteContext(ctx)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("REST compensation request did not start")
	}
	cancel()

	select {
	case result := <-results:
		require.Equal(t, core.CommandError, result.Signal, result.Output)
		require.Empty(t, result.Receipt)
		var partial *PartialRollbackError
		require.ErrorAs(t, result.Err, &partial)
		require.Len(t, partial.Failures, 1)
		require.Equal(t, "rest_set_issue", partial.Failures[0].CommandName)
		require.Contains(t, partial.Failures[0].Detail, context.Canceled.Error())
		require.Contains(t, result.Output, `"command":"rest_set_issue"`)
	case <-time.After(time.Second):
		t.Fatal("checkpoint rollback did not return after context cancellation")
	}
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
	require.Empty(t, res.Receipt)
	require.Contains(t, res.Output, "step=1 rest_set_issue: undo failed")
	require.Contains(t, res.Output, "compensation_lookup")
	require.Contains(t, res.Output, "receipt-walk Undo failure")

	var partial *PartialRollbackError
	require.ErrorAs(t, res.Err, &partial)
	require.Equal(t, 0, partial.Reverted)
	require.Len(t, partial.Failures, 1)
	require.Equal(t, "rest_set_issue", partial.Failures[0].CommandName)
}

func TestPersistedCheckpointRollbackReceiptReportsNamedPendingCompensation(t *testing.T) {
	t.Parallel()
	const alias = "recover_release_run"
	receipt, err := encodeCheckpointRollbackReceipt(checkpointRollbackReceipt{
		Version: checkpointRollbackReceiptVersion, Strategy: checkpointRollbackReceiptStrategy,
		Declaration: alias, Run: "run-release", TargetIteration: 4, TargetStep: 6,
		PriorBranch:        "run-release",
		RollbackCheckpoint: "checkpoint:v1:dolt:cnVuLXJlbGVhc2U:6:cm9sbGJhY2stcmV2aXNpb24",
		Requires:           append([]string(nil), checkpointRollbackReceiptRequirements...),
	})
	require.NoError(t, err)
	registry := core.NewRegistry()
	registry.Register(
		core.ToolSpec{Name: alias, Visibility: core.Internal},
		&CheckpointRollbackBuilder{ToolName: alias},
	)

	report, err := rollbackViaReceipts(rollbackViaReceiptsOptions{
		Reverter: &recordingReverter{}, Registry: registry, RunID: "operator-run",
		Execution: core.Execution{
			{Iteration: 1, CommandName: "seed"},
			{Iteration: 2, CommandName: alias, Receipt: receipt},
		},
		TargetIteration: 1,
	})

	require.NoError(t, err, report.Detail)
	require.Zero(t, report.Reverted)
	require.Len(t, report.PendingCompensation, 1)
	pending := report.PendingCompensation[0]
	require.Equal(t, alias, pending.CommandName)
	require.Contains(t, pending.Description, `rollback of run "run-release"`)
	require.Equal(t, checkpointRollbackReceiptRequirements, pending.Requires)
	require.Equal(t, "run-release", pending.Data["run"])
	require.Equal(t, "run-release", pending.Data["prior_branch"])
	require.Contains(t, report.Detail, alias+": compensation required")
}

func TestCheckpointRollbackUndoRequestsCompensation(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointRollbackBuilder{ToolName: "rollback_alias"}).Build(core.Result{})
	receipt, err := encodeCheckpointRollbackReceipt(checkpointRollbackReceipt{
		Version: checkpointRollbackReceiptVersion, Strategy: checkpointRollbackReceiptStrategy,
		Declaration: "rollback_alias", Run: "run-1", TargetIteration: 2, TargetStep: 3,
		PriorBranch: "run-1", Requires: append([]string(nil), checkpointRollbackReceiptRequirements...),
	})
	require.NoError(t, err)

	res := cmd.Undo(core.Result{CommandName: "rollback_alias", Receipt: receipt})

	require.Equal(t, core.CompensationRequired, res.Signal)
	require.NoError(t, res.Err)
	require.Equal(t, "rollback_alias", res.CommandName)
	require.Contains(t, res.Output, checkpointRollbackReceiptStrategy)
	require.Contains(t, res.Output, "operator_decision")
}
