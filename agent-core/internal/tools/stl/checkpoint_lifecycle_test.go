// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// fakeReverter is an in-memory CheckpointReverter for lifecycle tests. Revert
// truncates the stored Execution to the target step, mirroring the Dolt adapter
// resetting DB state, and records the calls it received.
type fakeReverter struct {
	pos        core.Position
	execution  core.Execution
	reverted   []int
	runID      string
	failRevert bool
}

func (f *fakeReverter) Save(p core.Position, e core.Execution) error {
	f.pos = p
	f.execution = append(core.Execution(nil), e...)
	return nil
}

func (f *fakeReverter) Load() (core.Position, core.Execution, error) {
	if f.execution == nil {
		return core.Position{}, nil, core.ErrNoCheckpoint
	}
	return f.pos, append(core.Execution(nil), f.execution...), nil
}

func (f *fakeReverter) Revert(runID string, step int) error {
	if f.failRevert {
		return errors.New("revert boom")
	}
	f.runID = runID
	f.reverted = append(f.reverted, step)
	if step+1 <= len(f.execution) {
		f.execution = f.execution[:step+1]
	}
	return nil
}

var _ core.CheckpointReverter = (*fakeReverter)(nil)

func TestCheckpointHistoryExecuteFormatsExecutionLog(t *testing.T) {
	t.Parallel()
	cp := &core.InMemoryCheckpoint{}
	require.NoError(t, cp.Save(
		core.Position{
			CurrentState: "Working",
			LastSignal:   core.ToolDone,
			Snapshot:     core.AgentSnapshot{State: "Working", Signal: core.ToolDone, Iteration: 2},
		},
		core.Execution{
			{Iteration: 1, CommandName: "read", FromState: "Idle", ToState: "Reading", Signal: core.ToolDone},
			{Iteration: 2, CommandName: "write", FromState: "Reading", ToState: "Working", Signal: core.ToolDone, Receipt: `{"path":"a.txt"}`},
		},
	))

	cmd := (&CheckpointHistoryBuilder{Checkpoint: cp}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal)
	require.Equal(t, "checkpoint_history", res.CommandName)
	require.Contains(t, res.Output, "state: Working")
	require.Contains(t, res.Output, "step=0  iteration=1  read  Idle -> Reading  signal=ToolDone")
	require.Contains(t, res.Output, "step=1  iteration=2  write  Reading -> Working  signal=ToolDone  reversible")
}

func TestCheckpointHistoryExecuteRequiresCheckpoint(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointHistoryBuilder{}).Build(core.Result{})

	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "requires a Checkpoint")
}

func TestCheckpointHistoryExecuteReportsNoCheckpoint(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointHistoryBuilder{Checkpoint: &core.InMemoryCheckpoint{}}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "no checkpoint persisted")
}

func TestCheckpointHistoryUndoMementoIsNoop(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointHistoryBuilder{}).Build(core.Result{})
	provider, ok := cmd.(core.UndoMementoProvider)
	require.True(t, ok)

	memento, err := provider.UndoMemento()

	require.NoError(t, err)
	require.Equal(t, core.UndoMementoNoop, memento.Kind)
	require.Equal(t, "checkpoint_history", memento.CommandName)
	require.Equal(t, core.ToolDone, cmd.Undo(core.Result{}).Signal)
}

func TestCheckpointRollbackRequiresRevertibleCheckpoint(t *testing.T) {
	t.Parallel()
	target := 1
	cmd := (&CheckpointRollbackBuilder{
		Config: CheckpointRollbackConfig{ToIteration: &target},
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
		Config:     CheckpointRollbackConfig{ToIteration: &target},
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

func TestCheckpointRollbackReportsMissingTargetIteration(t *testing.T) {
	t.Parallel()
	target := 99
	rev := &fakeReverter{}
	require.NoError(t, rev.Save(core.Position{}, core.Execution{
		{Iteration: 1, CommandName: "read"},
	}))

	cmd := (&CheckpointRollbackBuilder{
		Config:     CheckpointRollbackConfig{ToIteration: &target},
		Checkpoint: rev,
		RunID:      "run-1",
	}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Contains(t, res.Output, "target iteration 99 not found")
	require.Empty(t, rev.reverted)
}

func TestCheckpointRollbackRestoresFileViaPersistedReceipt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(target, []byte("v1"), 0o644))

	// Execute a real write that overwrites the file; capture its opaque receipt.
	writeBuilder := &WriteBuilder{Root: dir}
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
	reg.Register(WriteToolSpec(), &WriteBuilder{Root: dir})

	toIteration := 1
	cmd := (&CheckpointRollbackBuilder{
		Config:     CheckpointRollbackConfig{ToIteration: &toIteration},
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

func TestCheckpointRollbackUndoMementoIsCompensatable(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointRollbackBuilder{}).Build(core.Result{})
	provider, ok := cmd.(core.UndoMementoProvider)
	require.True(t, ok)

	memento, err := provider.UndoMemento()

	require.NoError(t, err)
	require.Equal(t, core.UndoMementoCompensatable, memento.Kind)
	require.Equal(t, "checkpoint_rollback", memento.CommandName)
	require.Equal(t, core.CommandError, cmd.Undo(core.Result{}).Signal)
	var payload BoundaryCompensationPayload
	require.NoError(t, json.Unmarshal(memento.Payload, &payload))
	require.Equal(t, "operator_checkpoint_selection", payload.BoundaryCompensation.Strategy)
	require.Contains(t, payload.BoundaryCompensation.Requires, "checkpoint_id")
}
