// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const (
	// doltServerDSN is a conventional local dolt sql-server address, without a
	// database. The test connects here, creates its own database, and skips when
	// no server is reachable, so it needs no environment configuration.
	doltServerDSN = "root@tcp(127.0.0.1:3306)/"
	doltTestDB    = "agent_core_test"
)

// TestDoltCheckpointSuspendResumeRoundTrip covers rel02.0 AC1/AC5: a run
// suspended and persisted through DoltCheckpoint is reloaded by a fresh adapter
// (a fresh-process analog) with an equivalent Position, folded conversation, and
// opaque receipts (srd036-dolt-state-persistence R5.4). Dolt speaks the MySQL
// wire protocol, so the test drives a running `dolt sql-server` through the
// composition-root "dolt" driver. It runs against a reachable local dolt
// sql-server and skips otherwise.
func TestDoltCheckpointSuspendResumeRoundTrip(t *testing.T) {
	requireDoltServer(t)
	dsn := doltServerDSN + doltTestDB

	runID := fmt.Sprintf("run-it-%d", time.Now().UnixNano())
	noMerge := func(core.State) bool { return false }

	saver, err := core.OpenDoltCheckpoint(dsn, runID, noMerge)
	require.NoError(t, err)
	pos := core.Position{
		CurrentState: "AwaitingApproval",
		LastSignal:   core.AwaitApproval,
		Snapshot: core.AgentSnapshot{
			State:        "AwaitingApproval",
			Signal:       core.AwaitApproval,
			Iteration:    1,
			TokensIn:     10,
			TokensOut:    5,
			TotalCost:    0.25,
			Conversation: json.RawMessage(`[{"role":"user","content":"before"}]`),
		},
	}
	exec := core.Execution{{
		Iteration:   1,
		CommandName: "suspend",
		FromState:   "Start",
		ToState:     "AwaitingApproval",
		Signal:      core.AwaitApproval,
		Result:      core.ResultDigest{Signal: core.AwaitApproval},
		Receipt:     `{"kind":"boundary"}`,
	}}
	require.NoError(t, saver.Save(pos, exec))
	require.NoError(t, saver.Close())

	loader, err := core.OpenDoltCheckpoint(dsn, runID, noMerge)
	require.NoError(t, err)
	defer func() { require.NoError(t, loader.Close()) }()

	gotPos, gotExec, err := loader.Load()
	require.NoError(t, err)
	require.Equal(t, core.State("AwaitingApproval"), gotPos.CurrentState)
	require.Equal(t, 1, gotPos.Snapshot.Iteration)
	require.Equal(t, 10, gotPos.Snapshot.TokensIn)
	require.JSONEq(t, `[{"role":"user","content":"before"}]`, string(gotPos.Snapshot.Conversation))
	require.Len(t, gotExec, 1)
	require.Equal(t, "suspend", gotExec[0].CommandName)
	require.Equal(t, `{"kind":"boundary"}`, gotExec[0].Receipt)
}

// requireDoltServer skips the test unless a local dolt sql-server is reachable,
// creating the test database on it, so the test needs no environment
// configuration to select the server.
func requireDoltServer(t *testing.T) {
	t.Helper()
	db, err := sql.Open("dolt", doltServerDSN)
	if err != nil {
		t.Skipf("open dolt driver: %v; skipping Dolt integration test", err)
	}
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("no reachable dolt sql-server at %s (%v); skipping Dolt integration test", doltServerDSN, err)
	}
	if _, err := db.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS "+doltTestDB); err != nil {
		t.Skipf("create database %s on dolt sql-server: %v; skipping", doltTestDB, err)
	}
}
