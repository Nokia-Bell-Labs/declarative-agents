// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// reverserStub is a core.Reverser whose receipt-driven Undo succeeds or fails on
// demand, so the receipt walk can be exercised without a real transport.
type reverserStub struct {
	name      string
	undoFails bool
}

func (b reverserStub) Build(core.Result) core.Command { return undoStub(b) }
func (b reverserStub) BuildReverser() core.Command    { return undoStub(b) }

type undoStub reverserStub

func (c undoStub) Name() string { return c.name }
func (c undoStub) Execute() core.Result {
	return core.Result{Signal: core.ToolDone, CommandName: c.name}
}
func (c undoStub) Undo(core.Result) core.Result {
	if c.undoFails {
		return core.Result{Signal: core.CommandError, CommandName: c.name, Output: "undo boom", Err: errors.New("undo boom")}
	}
	return core.Result{Signal: core.ToolDone, CommandName: c.name, Output: "undone"}
}

// recordingReverter is a CheckpointReverter that only records the Revert call;
// the receipt walk's inputs come from the Execution passed to rollbackViaReceipts.
type recordingReverter struct {
	reverted bool
	step     int
}

func (r *recordingReverter) Save(core.Position, core.Execution) error {
	return nil
}
func (r *recordingReverter) Load() (core.Position, core.Execution, error) {
	return core.Position{}, nil, nil
}
func (r *recordingReverter) Revert(_ string, step int) error {
	r.reverted = true
	r.step = step
	return nil
}

var _ core.CheckpointReverter = (*recordingReverter)(nil)

// TestRollbackViaReceiptsContinuesPastFailure proves that a failing receipt-walk
// Undo does not stop the walk, that a later entry is still reversed, and that
// the whole rollback is reported as a partial failure carrying the reversed
// count and the failed entry (srd026 R3.7, R6.3, R6.4; GH-491).
func TestRollbackViaReceiptsContinuesPastFailure(t *testing.T) {
	t.Parallel()

	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "fails", Visibility: core.Internal}, reverserStub{name: "fails", undoFails: true})
	reg.Register(core.ToolSpec{Name: "ok", Visibility: core.Internal}, reverserStub{name: "ok"})

	// Steps after the target (iteration 1) are walked in reverse: step 2 "fails"
	// then step 1 "ok". The failure must not prevent "ok" from being reversed.
	execution := core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "ok", Signal: core.ToolDone, Receipt: "r-ok"},
		{Iteration: 3, CommandName: "fails", Signal: core.ToolDone, Receipt: "r-fail"},
	}

	report, err := rollbackViaReceipts(rollbackViaReceiptsOptions{
		Reverter:        &recordingReverter{},
		Registry:        reg,
		RunID:           "run-mix",
		Execution:       execution,
		TargetIteration: 1,
	})

	var partial *PartialRollbackError
	require.ErrorAs(t, err, &partial)
	require.Equal(t, 1, partial.Reverted, report.Detail)
	require.Len(t, partial.Failures, 1)
	require.Equal(t, "fails", partial.Failures[0].CommandName)
	require.Equal(t, "undo boom", partial.Failures[0].Detail)

	require.Equal(t, 1, report.Reverted)
	require.Contains(t, report.Detail, "step=1 ok: undone")
	require.Contains(t, report.Detail, "step=2 fails: undo failed")
	require.Contains(t, report.Detail, "reversed 1, skipped 0, failed 1")
}

// TestRollbackViaReceiptsCleanWhenAllReverse proves a fully reversible run
// returns no error and a clean tally.
func TestRollbackViaReceiptsCleanWhenAllReverse(t *testing.T) {
	t.Parallel()

	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "ok", Visibility: core.Internal}, reverserStub{name: "ok"})

	execution := core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "ok", Signal: core.ToolDone, Receipt: "r-ok"},
	}

	report, err := rollbackViaReceipts(rollbackViaReceiptsOptions{
		Reverter:        &recordingReverter{},
		Registry:        reg,
		RunID:           "run-clean",
		Execution:       execution,
		TargetIteration: 1,
	})
	require.NoError(t, err)
	require.Equal(t, 1, report.Reverted)
	require.Contains(t, report.Detail, "reversed 1, skipped 0, failed 0")
}
