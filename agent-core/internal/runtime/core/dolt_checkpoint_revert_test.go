// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestDoltCheckpointSaveReplacesShortenedHistoryAndReceipt(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)
	original := threeStepExecution()
	for length := 1; length <= len(original); length++ {
		require.NoError(t, cp.Save(samplePosition(), original[:length]))
	}

	replacement := append(Execution(nil), original[:2]...)
	replacement[1].Result.Output = "replacement"
	replacement[1].Receipt = ""
	require.NoError(t, cp.Save(samplePosition(), replacement))
	require.NoError(t, cp.Save(samplePosition(), replacement), "repeated replacement is idempotent")

	_, got, err := cp.Load()
	require.NoError(t, err)
	require.Equal(t, replacement, got)
	require.NotContains(t, db.store.steps, rowKey("run-1", 2))
	require.NotContains(t, db.store.results, rowKey("run-1", 2))
	require.NotContains(t, db.store.receipts, rowKey("run-1", 1))
	require.NotContains(t, db.store.receipts, rowKey("run-1", 2))
}

func TestDoltCheckpointSplitsToolOutputsAndReceipts(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)

	// Two steps: step 0 carries a receipt, step 1 has an empty receipt.
	require.NoError(t, cp.Save(samplePosition(), sampleExecution()[:1]))
	require.NoError(t, cp.Save(samplePosition(), sampleExecution()))

	var toolOutputsCreate string
	haveReceiptsCreate := false
	for _, q := range db.calls {
		if strings.Contains(q, "CREATE TABLE IF NOT EXISTS tool_outputs") {
			toolOutputsCreate = q
		}
		if strings.Contains(q, "CREATE TABLE IF NOT EXISTS receipts") {
			haveReceiptsCreate = true
		}
	}
	require.NotEmpty(t, toolOutputsCreate, "tool_outputs table is created")
	require.NotContains(t, toolOutputsCreate, "receipt", "tool_outputs must not carry a receipt column; the split cannot silently regress")
	require.True(t, haveReceiptsCreate, "receipts table is created")

	// The forward plane gets a row per step; the reverse plane gets a row only
	// for the step that carried a receipt, so an empty receipt is row absence.
	require.Equal(t, 2, countCalls(db.calls, "REPLACE INTO tool_outputs"), "one forward-plane row per step")
	require.Equal(t, 1, countCalls(db.calls, "REPLACE INTO receipts"), "only the step with a receipt writes a reverse-plane row")
}

func TestDoltCheckpointMergeOnTerminalState(t *testing.T) {
	t.Parallel()
	terminal := func(s State) bool { return s == "Done" }

	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", terminal)
	require.NoError(t, cp.Save(samplePosition(), sampleExecution()[:1]))
	require.Equal(t, 0, countCalls(db.calls, "DOLT_MERGE"), "non-terminal save does not merge")

	terminalPos := samplePosition()
	terminalPos.CurrentState = "Done"
	require.NoError(t, cp.Save(terminalPos, sampleExecution()))
	require.Equal(t, 1, countCalls(db.calls, "DOLT_MERGE"), "terminal save merges to main")
	require.Equal(t, 1, countCalls(db.calls, "DOLT_BRANCH('-d'"), "run branch deleted after merge")
	require.False(t, db.branches["run-1"], "run branch removed")
}

func TestDoltCheckpointRevertResetsToStepCommit(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)
	exec := sampleExecution()
	require.NoError(t, cp.Save(samplePosition(), exec[:1]))
	require.NoError(t, cp.Save(samplePosition(), exec))

	require.NoError(t, cp.Revert("run-1", 0))
	require.Equal(t, 1, countCalls(db.calls, "DOLT_RESET"), "revert resets the branch")

	// After reverting to step 0's commit, Load returns only step 0.
	_, gotExec, err := cp.Load()
	require.NoError(t, err)
	require.Len(t, gotExec, 1)
	require.Equal(t, exec[0], gotExec[0])
}

// TestDoltCheckpointRevertReapsBothPlanes proves invariant (1): reverting to an
// earlier step reaps both the tool_outputs forward plane and the receipts reverse
// plane for every later step, by branch construction and without a separate
// invalidation walk (srd036-dolt-state-persistence R4, srd038-command-state-store
// R4). Implements the reap half of rel07.0-uc002-two-plane-revert.
func TestDoltCheckpointRevertReapsBothPlanes(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)
	exec := threeStepExecution()

	require.NoError(t, cp.Save(samplePosition(), exec[:1]))
	require.NoError(t, cp.Save(samplePosition(), exec[:2]))
	require.NoError(t, cp.Save(samplePosition(), exec))

	require.NoError(t, cp.Revert("run-1", 1))

	// Steps 0 and 1 survive on both planes; step 2's rows are gone from both.
	require.Contains(t, db.store.results, rowKey("run-1", 0))
	require.Contains(t, db.store.results, rowKey("run-1", 1))
	require.NotContains(t, db.store.results, rowKey("run-1", 2), "forward-plane row past the target step is reaped")
	require.Contains(t, db.store.receipts, rowKey("run-1", 1))
	require.NotContains(t, db.store.receipts, rowKey("run-1", 2), "reverse-plane row past the target step is reaped")

	// Load returns only steps 0-1 and step 1's receipt still round-trips.
	_, gotExec, err := cp.Load()
	require.NoError(t, err)
	require.Len(t, gotExec, 2)
	require.Equal(t, exec[1].Receipt, gotExec[1].Receipt)
}

// TestDoltCheckpointTerminalReapsBothPlanesWithBranch proves invariant (2): the
// terminal-state merge-and-delete reaps the run branch, and with it both planes'
// per-run rows (srd036-dolt-state-persistence R4.3).
func TestDoltCheckpointTerminalReapsBothPlanesWithBranch(t *testing.T) {
	t.Parallel()
	terminal := func(s State) bool { return s == "Done" }
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", terminal)
	exec := threeStepExecution()

	require.NoError(t, cp.Save(samplePosition(), exec[:1]))
	require.NoError(t, cp.Save(samplePosition(), exec[:2]))

	// Both planes were written on the run branch before the terminal save.
	require.GreaterOrEqual(t, countCalls(db.calls, "REPLACE INTO tool_outputs"), 2)
	require.GreaterOrEqual(t, countCalls(db.calls, "REPLACE INTO receipts"), 2)

	terminalPos := samplePosition()
	terminalPos.CurrentState = "Done"
	require.NoError(t, cp.Save(terminalPos, exec))

	require.Equal(t, 1, countCalls(db.calls, "DOLT_MERGE"), "terminal save merges to main")
	require.Equal(t, 1, countCalls(db.calls, "DOLT_BRANCH('-d'"), "run branch deleted after merge")
	require.False(t, db.branches["run-1"], "run branch and both of its planes are reaped")
}

// TestDoltCheckpointReceiptWalkReversesSurvivingStep proves the reverse-plane
// half of rel07.0-uc002: after reverting to step 1, the rollback receipt walk
// drives reversal of the surviving steps newest-first from their receipts, while
// the reaped later step contributes no receipt (srd036 R3, R6.3; srd038 R4).
func TestDoltCheckpointReceiptWalkReversesSurvivingStep(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)
	exec := threeStepExecution()

	require.NoError(t, cp.Save(samplePosition(), exec[:1]))
	require.NoError(t, cp.Save(samplePosition(), exec[:2]))
	require.NoError(t, cp.Save(samplePosition(), exec))

	require.NoError(t, cp.Revert("run-1", 1))
	_, restored, err := cp.Load()
	require.NoError(t, err)
	require.Len(t, restored, 2)

	// Walk the surviving execution newest-first, reversing each step from the
	// receipt restored from the receipts plane.
	var reversed []string
	for i := len(restored) - 1; i >= 0; i-- {
		rev := &receiptReverser{}
		rev.Undo(Result{CommandName: restored[i].CommandName, Receipt: restored[i].Receipt})
		reversed = append(reversed, rev.seen)
	}

	// Step 2's receipt was reaped with the branch; only steps 1 then 0 reverse.
	require.Equal(t, []string{`{"step":1}`, `{"step":0}`}, reversed)
}

func TestDoltCheckpointRevertUnresolvedStep(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)
	require.NoError(t, cp.Save(samplePosition(), sampleExecution()[:1]))

	err := cp.Revert("run-1", 99)
	require.ErrorIs(t, err, ErrRevertUnresolved)
}

// TestCommandStateViewRehydratesFromDoltLoad proves the command-state view built
// from an execution restored through the Dolt Load path (tool_outputs forward
// plane) resolves identical labels to the live log, so a run resumed from Dolt
// reads the same command state (srd038-command-state-store R1.4, srd036 R5).
func TestCommandStateViewRehydratesFromDoltLoad(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)
	exec := threeStepExecution()

	require.NoError(t, cp.Save(samplePosition(), exec[:1]))
	require.NoError(t, cp.Save(samplePosition(), exec[:2]))
	require.NoError(t, cp.Save(samplePosition(), exec))

	_, restored, err := cp.Load()
	require.NoError(t, err)

	live := NewCommandStateView(exec)
	rehydrated := NewCommandStateView(restored)
	for _, label := range []string{"invoke", "read", "write", "missing"} {
		liveOut, liveOK := live.Lookup(label)
		rehOut, rehOK := rehydrated.Lookup(label)
		require.Equal(t, liveOK, rehOK, "label %q resolves the same after Dolt rehydration", label)
		require.Equal(t, liveOut, rehOut, "label %q output matches after Dolt rehydration", label)
	}
}
