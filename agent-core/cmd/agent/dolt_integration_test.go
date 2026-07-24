// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// TestDoltCheckpointSuspendResumeRoundTrip proves same-process adapter reopen:
// a run persisted through DoltCheckpoint is reloaded by a new adapter with an
// equivalent Position, folded conversation, and opaque receipts. Cross-process
// persistence is covered separately below. Dolt speaks the MySQL
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
		Result:      core.DigestResult(core.Result{Signal: core.AwaitApproval}),
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

func TestDoltCheckpointSuspendResumeAcrossProcesses(t *testing.T) {
	mode := os.Getenv("DOLT_PROCESS_PROOF_MODE")
	if mode != "" {
		runDoltProcessProofChild(t, mode)
		return
	}

	requireDoltServer(t)
	runID := fmt.Sprintf("run-process-it-%d", time.Now().UnixNano())
	artifact := filepath.Join(t.TempDir(), "loaded.json")
	runDoltProcessProof(t, "save", runID, artifact)
	runDoltProcessProof(t, "load", runID, artifact)

	data, err := os.ReadFile(artifact)
	require.NoError(t, err)
	var loaded struct {
		Position  core.Position
		Execution core.Execution
	}
	require.NoError(t, json.Unmarshal(data, &loaded))
	require.Equal(t, core.State("AwaitingApproval"), loaded.Position.CurrentState)
	require.Equal(t, core.AwaitApproval, loaded.Position.LastSignal)
	require.Equal(t, 1, loaded.Position.Snapshot.Iteration)
	require.JSONEq(t, `[{"role":"user","content":"before-process-exit"}]`, string(loaded.Position.Snapshot.Conversation))
	require.Len(t, loaded.Execution, 1)
	require.Equal(t, "suspend", loaded.Execution[0].CommandName)
	require.Equal(t, core.AwaitApproval, loaded.Execution[0].Signal)
	require.Equal(t, `{"kind":"process-boundary"}`, loaded.Execution[0].Receipt)
}

func runDoltProcessProof(t *testing.T, mode, runID, artifact string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestDoltCheckpointSuspendResumeAcrossProcesses$")
	cmd.Env = append(os.Environ(),
		"DOLT_PROCESS_PROOF_MODE="+mode,
		"DOLT_PROCESS_PROOF_RUN_ID="+runID,
		"DOLT_PROCESS_PROOF_ARTIFACT="+artifact,
	)
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "%s child failed:\n%s", mode, output)
}

func runDoltProcessProofChild(t *testing.T, mode string) {
	t.Helper()
	dsn := doltServerDSN + doltTestDB
	runID := os.Getenv("DOLT_PROCESS_PROOF_RUN_ID")
	checkpoint, err := core.OpenDoltCheckpoint(dsn, runID, func(core.State) bool { return false })
	require.NoError(t, err)
	defer func() { require.NoError(t, checkpoint.Close()) }()

	switch mode {
	case "save":
		position := core.Position{
			CurrentState: "AwaitingApproval",
			LastSignal:   core.AwaitApproval,
			Snapshot: core.AgentSnapshot{
				State: "AwaitingApproval", Signal: core.AwaitApproval, Iteration: 1,
				Conversation: json.RawMessage(`[{"role":"user","content":"before-process-exit"}]`),
			},
		}
		execution := core.Execution{{
			Iteration: 1, CommandName: "suspend", FromState: "Start",
			ToState: "AwaitingApproval", Signal: core.AwaitApproval,
			Result:  core.DigestResult(core.Result{Signal: core.AwaitApproval}),
			Receipt: `{"kind":"process-boundary"}`,
		}}
		require.NoError(t, checkpoint.Save(position, execution))
	case "load":
		position, execution, err := checkpoint.Load()
		require.NoError(t, err)
		data, err := json.Marshal(struct {
			Position  core.Position
			Execution core.Execution
		}{position, execution})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(os.Getenv("DOLT_PROCESS_PROOF_ARTIFACT"), data, 0o600))
	default:
		t.Fatalf("unsupported process proof mode %q", mode)
	}
}

// TestDoltCommandStateRehydratesThroughRealAdapter covers the rel07.0 restart
// through the real Dolt code path: an execution persisted and reloaded by a fresh
// DoltCheckpoint rebuilds a command-state view whose label lookup resolves the
// step's output (srd038 R1.4; srd036 R5). It runs against a reachable local dolt
// sql-server and skips otherwise.
func TestDoltCommandStateRehydratesThroughRealAdapter(t *testing.T) {
	requireDoltServer(t)
	dsn := doltServerDSN + doltTestDB

	runID := fmt.Sprintf("run-cs-%d", time.Now().UnixNano())
	noMerge := func(core.State) bool { return false }

	saver, err := core.OpenDoltCheckpoint(dsn, runID, noMerge)
	require.NoError(t, err)
	exec := core.Execution{{
		Iteration: 1, CommandName: "embed_query", FromState: "Start", ToState: "Working",
		Signal: core.LLMResponded,
		Result: core.DigestResult(core.Result{
			Signal: core.LLMResponded,
			Output: `{"mapped":{"embedding":[0.1,0.2]}}`,
		}),
	}}
	require.NoError(t, saver.Save(core.Position{CurrentState: "Working", LastSignal: core.LLMResponded}, exec))
	require.NoError(t, saver.Close())

	loader, err := core.OpenDoltCheckpoint(dsn, runID, noMerge)
	require.NoError(t, err)
	defer func() { require.NoError(t, loader.Close()) }()

	_, gotExec, err := loader.Load()
	require.NoError(t, err)
	view := core.NewCommandStateView(gotExec)
	out, ok := view.Lookup("embed_query")
	require.True(t, ok)
	require.JSONEq(t, `{"mapped":{"embedding":[0.1,0.2]}}`, out)
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
		t.Skipf("no reachable dolt sql-server at %s (%v); run `mage dolt:up` to start a persistent one, then re-run; skipping Dolt integration test", doltServerDSN, err)
	}
	if _, err := db.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS "+doltTestDB); err != nil {
		t.Skipf("create database %s on dolt sql-server: %v; skipping", doltTestDB, err)
	}
}
