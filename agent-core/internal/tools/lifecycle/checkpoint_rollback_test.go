// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package lifecycle

import (
	"encoding/json"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

type rollbackReferenceReverter struct {
	*fakeReverter
	priorReference    string
	rollbackReference string
}

func (r *rollbackReferenceReverter) ConversationReference() (string, bool) {
	if len(r.reverted) == 0 {
		return r.priorReference, r.priorReference != ""
	}
	return r.rollbackReference, r.rollbackReference != ""
}

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
	require.Empty(t, res.Receipt)
}

func TestCheckpointRollbackRequiresTargetIteration(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointRollbackBuilder{Checkpoint: &fakeReverter{}}).Build(core.Result{})

	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "requires to_iteration")
	require.Empty(t, res.Receipt)
}

func TestCheckpointRollbackReportsMissingUndoAfterDBRevert(t *testing.T) {
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

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Empty(t, res.Receipt)
	require.Equal(t, []int{0}, rev.reverted)
	require.Equal(t, "run-1", rev.runID)
	require.Contains(t, res.Output, "rolled back run run-1 to iteration 1 (step 0)")
	require.Contains(t, res.Output, "step=1 write: undo failed: rollback cannot reverse write: no registry")

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
	require.Empty(t, res.Receipt)
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
	require.Empty(t, res.Receipt)
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
		Run                 string                `json:"run"`
		TargetStep          int                   `json:"target_step"`
		RevertedEntries     int                   `json:"reverted_entries"`
		SkippedIrreversible []string              `json:"skipped_irreversible"`
		PendingCompensation []PendingCompensation `json:"pending_compensation"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.Output), &out))
	require.Equal(t, "run-x", out.Run)
	require.Equal(t, 0, out.TargetStep)
	require.Equal(t, 1, out.RevertedEntries)
	require.NotNil(t, out.SkippedIrreversible)
	require.NotNil(t, out.PendingCompensation)
}

func TestCheckpointRollbackAliasEmitsStrictRecoveryReceipt(t *testing.T) {
	t.Parallel()
	const (
		alias              = "recover_release_run"
		priorCheckpoint    = "checkpoint:v1:dolt:cnVuLXg:1:cHJpb3ItcmV2aXNpb24"
		rollbackCheckpoint = "checkpoint:v1:dolt:cnVuLXg:0:cm9sbGJhY2stcmV2aXNpb24"
	)
	reverter := &rollbackReferenceReverter{
		fakeReverter:      &fakeReverter{},
		priorReference:    priorCheckpoint,
		rollbackReference: rollbackCheckpoint,
	}
	require.NoError(t, reverter.Save(core.Position{CurrentState: "Working"}, core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "write", Signal: core.ToolDone, Receipt: "r-write"},
	}))
	registry := core.NewRegistry()
	registry.Register(
		core.ToolSpec{Name: "write", Visibility: core.Internal},
		reverserStub{name: "write"},
	)
	target := 1
	builder := &CheckpointRollbackBuilder{
		ToolName: alias, Config: catalog.CheckpointRollbackConfig{ToIteration: &target},
		Checkpoint: reverter, Registry: registry, RunID: "run-x",
	}
	command := builder.Build(core.Result{})

	executed := command.Execute()

	require.Equal(t, core.ToolDone, executed.Signal, executed.Output)
	require.Equal(t, alias, executed.CommandName)
	require.NotEmpty(t, executed.Receipt)
	receipt, err := decodeCheckpointRollbackReceipt(executed.Receipt)
	require.NoError(t, err)
	require.Equal(t, alias, receipt.Declaration)
	require.Equal(t, "run-x", receipt.Run)
	require.Equal(t, 1, receipt.TargetIteration)
	require.Equal(t, 0, receipt.TargetStep)
	require.Equal(t, "run-x", receipt.PriorBranch)
	require.Equal(t, priorCheckpoint, receipt.PriorCheckpoint)
	require.Equal(t, rollbackCheckpoint, receipt.RollbackCheckpoint)
	require.Equal(t, checkpointRollbackReceiptRequirements, receipt.Requires)

	reverser, ok := interface{}(builder).(core.Reverser)
	require.True(t, ok)
	fresh := reverser.BuildReverser()
	require.Equal(t, alias, fresh.Name())
	compensation := fresh.Undo(executed)
	require.Equal(t, core.CompensationRequired, compensation.Signal)
	require.Equal(t, alias, compensation.CommandName)
	require.Len(t, reverter.reverted, 1, "Undo must not replay forward execution")

	pending, ok, err := undo.DecodeBoundaryReceipt(compensation.Output)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, checkpointRollbackReceiptStrategy, pending.Strategy)
	require.Equal(t, checkpointRollbackReceiptRequirements, pending.Requires)
	require.Equal(t, "run-x", pending.Data["run"])
	require.Equal(t, float64(1), pending.Data["target_iteration"])
	require.Equal(t, float64(0), pending.Data["target_step"])
	require.Equal(t, "run-x", pending.Data["prior_branch"])
	require.Equal(t, priorCheckpoint, pending.Data["prior_checkpoint"])
	require.Equal(t, rollbackCheckpoint, pending.Data["rollback_checkpoint"])
}

func TestCheckpointRollbackFreshReverserRejectsInvalidRecoveryReceipts(t *testing.T) {
	t.Parallel()
	builder := &CheckpointRollbackBuilder{ToolName: "rollback_alias"}
	fresh := builder.BuildReverser()
	valid := checkpointRollbackReceipt{
		Version: checkpointRollbackReceiptVersion, Strategy: checkpointRollbackReceiptStrategy,
		Declaration: "rollback_alias", Run: "run-1", TargetIteration: 2, TargetStep: 3,
		PriorBranch: "run-1", Requires: append([]string(nil), checkpointRollbackReceiptRequirements...),
	}
	unknown := receiptJSON(t, map[string]interface{}{
		"version": valid.Version, "strategy": valid.Strategy,
		"declaration": valid.Declaration, "run": valid.Run,
		"target_iteration": valid.TargetIteration, "target_step": valid.TargetStep,
		"prior_branch": valid.PriorBranch, "requires": valid.Requires,
		"unexpected": true,
	})
	wrongDeclaration := valid
	wrongDeclaration.Declaration = "other_alias"
	missingDecision := valid
	missingDecision.Requires = nil
	malformedContext := valid
	malformedContext.Run = "run-\x00"

	for _, test := range []struct {
		name    string
		prior   core.Result
		wantErr string
	}{
		{
			name: "missing", prior: core.Result{CommandName: "rollback_alias"},
			wantErr: "receipt is required",
		},
		{
			name:    "unknown field",
			prior:   core.Result{CommandName: "rollback_alias", Receipt: unknown},
			wantErr: "unknown field",
		},
		{
			name:    "wrong persisted command",
			prior:   core.Result{CommandName: "other_alias", Receipt: receiptJSON(t, valid)},
			wantErr: "does not match declaration",
		},
		{
			name:    "wrong receipt declaration",
			prior:   core.Result{CommandName: "rollback_alias", Receipt: receiptJSON(t, wrongDeclaration)},
			wantErr: "does not match",
		},
		{
			name:    "missing operator decision",
			prior:   core.Result{CommandName: "rollback_alias", Receipt: receiptJSON(t, missingDecision)},
			wantErr: "operator recovery decision",
		},
		{
			name:    "malformed run context",
			prior:   core.Result{CommandName: "rollback_alias", Receipt: receiptJSON(t, malformedContext)},
			wantErr: "run is required and must be canonical",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := fresh.Undo(test.prior)
			require.Equal(t, core.CommandError, result.Signal)
			require.ErrorContains(t, result.Err, test.wantErr)
			require.Equal(t, "rollback_alias", result.CommandName)
			require.Empty(t, result.Receipt)
		})
	}
}

func TestCheckpointRollbackDeclarationMatchesRecoveryReceipt(t *testing.T) {
	t.Parallel()
	root := repoRootFromLifecycleRuntime()
	defs, err := catalog.LoadToolDeclarations([]string{
		filepath.Join(root, "tools", "builtin", "checkpoint-rollback.yaml"),
	})
	require.NoError(t, err)
	require.NoError(t, catalog.ValidateReceiptContracts(defs))
	def := lifecycleDef(t, defs, "checkpoint_rollback")
	require.Equal(t, "compensatable", def.Reversibility.Classification)
	require.Equal(t, "compensating_action", def.Undo.Strategy)
	for _, field := range []string{
		"declaration", "run", "target_iteration", "target_step",
		"prior_branch", "prior_checkpoint", "rollback_checkpoint", "requires",
	} {
		require.Contains(t, def.Undo.Captures, field)
	}
	require.Contains(t, def.Undo.Requires, "operator_decision")
	require.Contains(t, def.Undo.Requires, "resume_checkpoint_or_rollback_target")
}
