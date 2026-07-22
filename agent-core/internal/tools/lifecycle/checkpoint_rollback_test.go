// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"encoding/json"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCheckpointRollbackRequiresRevertibleCheckpoint(t *testing.T) {
	t.Parallel()
	target := 1
	cmd := (&CheckpointRollbackBuilder{
		Config: catalog.CheckpointRollbackConfig{ToIteration: &target},
	}).Build(core.Result{})

	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "requires a revertible Checkpoint backend")
}

func TestCheckpointRollbackRequiresTargetIteration(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointRollbackBuilder{Checkpoint: &fakeReverter{}}).Build(core.Result{})

	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "requires to_iteration")
}

func TestCheckpointRollbackRevertsDBStateToTargetStep(t *testing.T) {
	t.Parallel()
	target := 1
	rev := &fakeReverter{}
	require.NoError(t, rev.Save(core.Position{CurrentState: "Working"}, core.Execution{
		{Iteration: 1, CommandName: "read", FromState: "Idle", ToState: "Reading", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "write", FromState: "Reading", ToState: "Working", Signal: core.ToolDone},
	}))

	cmd := (&CheckpointRollbackBuilder{
		Config:     catalog.CheckpointRollbackConfig{ToIteration: &target},
		Checkpoint: rev,
		RunID:      "run-1",
	}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal)
	require.Equal(t, []int{0}, rev.reverted)
	require.Equal(t, "run-1", rev.runID)
	require.Contains(t, res.Output, "rolled back run run-1 to iteration 1 (step 0)")
	require.Contains(t, res.Output, "step=1 write: skipped (no registry)")

	_, reloaded, err := rev.Load()
	require.NoError(t, err)
	require.Len(t, reloaded, 1)
	require.Equal(t, "read", reloaded[0].CommandName)
}

func TestCheckpointRollbackRevertFailurePreservesStateAndSkipsUndo(t *testing.T) {
	t.Parallel()
	target := 1
	original := core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "write", Signal: core.ToolDone, Receipt: `{"path":"a.txt"}`},
	}
	rev := &fakeReverter{failRevert: true}
	require.NoError(t, rev.Save(core.Position{CurrentState: "Working"}, original))
	undoCalls := 0
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "write"}, &undoTrackingBuilder{calls: &undoCalls})

	res := (&CheckpointRollbackBuilder{
		Config:     catalog.CheckpointRollbackConfig{ToIteration: &target},
		Checkpoint: rev,
		Registry:   reg,
		RunID:      "run-1",
	}).Build(core.Result{}).Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.ErrorContains(t, res.Err, `revert run "run-1" to step 0`)
	require.ErrorContains(t, res.Err, "revert boom")
	require.Empty(t, rev.reverted)
	require.Empty(t, rev.runID)
	require.Equal(t, original, rev.execution)
	require.Zero(t, undoCalls)
}

func TestCheckpointRollbackReportsMissingTargetIteration(t *testing.T) {
	t.Parallel()
	target := 99
	rev := &fakeReverter{}
	require.NoError(t, rev.Save(core.Position{}, core.Execution{
		{Iteration: 1, CommandName: "read"},
	}))

	cmd := (&CheckpointRollbackBuilder{
		Config:     catalog.CheckpointRollbackConfig{ToIteration: &target},
		Checkpoint: rev,
		RunID:      "run-1",
	}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Contains(t, res.Output, "target iteration 99 not found")
	require.Empty(t, rev.reverted)
}

// TestCheckpointRollbackStructuredOutputMatchesSchema decodes the rollback
// Result.Output against the declared checkpoint-rollback schema and asserts the
// required fields — run, target_step, reverted_entries — and skipped list are
// present and correct (srd026 R3.8; GH-493).
func TestCheckpointRollbackStructuredOutputMatchesSchema(t *testing.T) {
	t.Parallel()
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "ok", Visibility: core.Internal}, reverserStub{name: "ok"})
	rev := &fakeReverter{}
	require.NoError(t, rev.Save(core.Position{CurrentState: "Working"}, core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "ok", Signal: core.ToolDone, Receipt: "r-ok"},
	}))

	toIteration := 1
	cmd := (&CheckpointRollbackBuilder{
		Config:     catalog.CheckpointRollbackConfig{ToIteration: &toIteration},
		Checkpoint: rev,
		Registry:   reg,
		RunID:      "run-x",
	}).Build(core.Result{})
	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal, res.Output)

	var out struct {
		Run                 string   `json:"run"`
		TargetStep          int      `json:"target_step"`
		RevertedEntries     int      `json:"reverted_entries"`
		SkippedIrreversible []string `json:"skipped_irreversible"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.Output), &out))
	require.Equal(t, "run-x", out.Run)
	require.Equal(t, 0, out.TargetStep)
	require.Equal(t, 1, out.RevertedEntries)
	require.NotNil(t, out.SkippedIrreversible)
}
