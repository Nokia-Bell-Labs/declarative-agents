// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestDoltCheckpointImplementsPort(t *testing.T) {
	t.Parallel()
	var cp Checkpoint = NewDoltCheckpoint(newFakeDB(), "run-1", nil)
	require.NotNil(t, cp)
}

func TestDoltCheckpointSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)
	exec := sampleExecution()
	pos := samplePosition()

	require.NoError(t, cp.Save(pos, exec[:1]))
	require.NoError(t, cp.Save(pos, exec))

	gotPos, gotExec, err := cp.Load()
	require.NoError(t, err)
	require.Equal(t, pos.CurrentState, gotPos.CurrentState)
	require.Equal(t, pos.LastSignal, gotPos.LastSignal)
	require.Equal(t, pos.Snapshot, gotPos.Snapshot)
	require.Equal(t, exec, gotExec)
	// Receipt round-trips verbatim; the empty receipt restores empty from NULL.
	require.Equal(t, `{"file":"a.txt"}`, gotExec[0].Receipt)
	require.Equal(t, "", gotExec[1].Receipt)
}

func TestDoltCheckpointSaveEmptyExecutionReapsAllStepRows(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)
	require.NoError(t, cp.Save(samplePosition(), sampleExecution()))
	require.NoError(t, cp.Save(samplePosition(), nil))

	_, got, err := cp.Load()
	require.NoError(t, err)
	require.Empty(t, got)
	require.Empty(t, db.store.transitions)
	require.Empty(t, db.store.steps)
	require.Empty(t, db.store.results)
	require.Empty(t, db.store.receipts)
}

func TestDoltCheckpointSingleTransactionAtomicity(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	db.failOn = "REPLACE INTO receipts"
	cp := NewDoltCheckpoint(db, "run-1", nil)

	// step 0 carries a receipt, so the receipts write is reached and forced to
	// fail between the two per-step table writes.
	err := cp.Save(samplePosition(), sampleExecution()[:1])
	require.Error(t, err, "a fault on the receipts write fails the save")
	require.Equal(t, 0, countCalls(db.calls, "DOLT_COMMIT"), "no commit is issued when a step is only partially written")
}

func TestDoltCheckpointReappliesAndPersistsOutputRedaction(t *testing.T) {
	t.Parallel()

	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "redacted-run", nil)
	entry := redactionCheckpointEntry("dolt-secret")
	require.NoError(t, cp.Save(samplePosition(), Execution{entry}))

	require.Len(t, db.toolOutputArgs, 1)
	require.NotContains(t, fmt.Sprint(db.toolOutputArgs[0]), "dolt-secret")
	stored := db.store.results[rowKey("redacted-run", 0)]
	require.NotNil(t, stored.output)
	require.JSONEq(t, `{"public":"ok"}`, *stored.output)
	require.Equal(t, int64(OutputRedactionVersion1), *stored.redactionVersion)
	require.JSONEq(t, `[["secret"]]`, *stored.redactedPaths)
	require.Equal(t, string(OutputRedactionApplied), *stored.status)
	require.Equal(t, `{"opaque":"receipt"}`, *db.store.receipts[rowKey("redacted-run", 0)])

	fresh := NewDoltCheckpoint(db, "redacted-run", nil)
	_, restored, err := fresh.Load()
	require.NoError(t, err)
	value, err := ResolveFromSelector(NewCommandStateView(restored), "$from(fetch).public")
	require.NoError(t, err)
	require.Equal(t, "ok", value)
	_, err = ResolveFromSelector(NewCommandStateView(restored), "$from(fetch).secret")
	var missing *UnresolvedPathError
	require.ErrorAs(t, err, &missing)
}

func TestDoltCheckpointRedactionFailureDoesNotBeginOrCommit(t *testing.T) {
	t.Parallel()

	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "invalid-redaction", nil)
	entry := redactionCheckpointEntry("must-not-persist")
	entry.Result.RedactedPaths = []OutputRedactionPath{{" secret"}}

	err := cp.Save(samplePosition(), Execution{entry})
	require.ErrorContains(t, err, "output redaction")
	require.Empty(t, db.toolOutputArgs)
	require.Empty(t, db.commits)
	require.Empty(t, db.store.results)
}

func TestDoltCheckpointSchemaUpgradeAddsRedactionMetadata(t *testing.T) {
	t.Parallel()

	db := newFakeDB()
	db.toolOutputsExists = true
	require.NoError(t, createSchema(db))
	require.NoError(t, createSchema(db), "redaction schema upgrade is idempotent")

	for _, column := range []string{"redaction_version", "redacted_paths", "redaction_status"} {
		require.True(t, db.redactionColumns[column])
		require.Equal(t, 1, countCalls(db.calls, "ADD COLUMN "+column))
	}
}

func TestDoltCheckpointLegacyOutputLoadsButSelectorsDenyIt(t *testing.T) {
	t.Parallel()

	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "legacy-output", nil)
	require.NoError(t, cp.Save(samplePosition(), sampleExecution()[:1]))

	key := rowKey("legacy-output", 0)
	row := db.store.results[key]
	row.redactionVersion = nil
	row.redactedPaths = nil
	row.status = nil
	db.store.results[key] = row
	db.redactionColumns = map[string]bool{}

	_, restored, err := NewDoltCheckpoint(db, "legacy-output", nil).Load()
	require.NoError(t, err)
	for _, column := range []string{"redaction_version", "redacted_paths", "redaction_status"} {
		require.True(t, db.redactionColumns[column], "Load upgrades the legacy forward-plane schema")
	}
	require.Equal(t, `{"file":"a.txt"}`, restored[0].Receipt)
	_, err = ResolveFromSelector(NewCommandStateView(restored), "$from(draft).value")
	var unavailable *CommandStateOutputUnavailableError
	require.ErrorAs(t, err, &unavailable)
	require.Zero(t, unavailable.Version)
}

func TestDoltCheckpointQuotesReservedSignalColumn(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)

	require.NoError(t, cp.Save(samplePosition(), sampleExecution()[:1]))
	_, _, err := cp.Load()
	require.NoError(t, err)

	queries := strings.Join(db.calls, "\n")
	require.Equal(t, 2, strings.Count(queries, "`signal` VARCHAR(255) NOT NULL"))
	require.Contains(t, queries, "REPLACE INTO transitions (run_id, step_index, from_state, `signal`, to_state)")
	require.Contains(t, queries, "(run_id, step_index, `signal`, output, error")
	require.Contains(t, queries, "t.`signal`")
	require.Contains(t, queries, "o.`signal`")
	requireNoUnquotedSignalColumn(t, queries)
}

func TestDoltCheckpointCommitPerStepAndBranchPerRun(t *testing.T) {
	t.Parallel()
	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)
	exec := sampleExecution()

	require.NoError(t, cp.Save(samplePosition(), exec[:1]))
	require.NoError(t, cp.Save(samplePosition(), exec))

	require.Equal(t, 1, countCalls(db.calls, "DOLT_CHECKOUT('-b'"), "branch created exactly once per run")
	require.Equal(t, 2, len(db.commits), "one commit per step")
	require.Contains(t, db.commits[0].message, "step 0 signal")
	require.Contains(t, db.commits[1].message, "step 1 signal")
}

func TestDoltCheckpointLoadNotFound(t *testing.T) {
	t.Parallel()
	cp := NewDoltCheckpoint(newFakeDB(), "missing", nil)
	_, _, err := cp.Load()
	require.ErrorIs(t, err, ErrNoCheckpoint)
}
