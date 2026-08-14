// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const doltTestDB = "agent_core_test"

var doltBin = flag.String("dolt-bin", "", "path or command name for the Dolt integration-test executable")

// TestDoltCheckpointSuspendResumeRoundTrip proves same-process adapter reopen:
// a run persisted through DoltCheckpoint is reloaded by a new adapter with an
// equivalent Position, folded conversation, and opaque receipts. Cross-process
// persistence is covered separately below. Dolt speaks the MySQL wire protocol,
// so the test drives a `dolt sql-server` through the composition-root "dolt"
// driver. The server is launched from a prebuilt dolt binary for the duration of
// the test (no Docker, no manual setup); the test skips only when no dolt binary
// is available.
func TestDoltCheckpointSuspendResumeRoundTrip(t *testing.T) {
	base := startDoltServer(t)
	requireDoltTestDB(t, base)
	dsn := base + doltTestDB

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

	base := startDoltServer(t)
	requireDoltTestDB(t, base)
	runID := fmt.Sprintf("run-process-it-%d", time.Now().UnixNano())
	artifact := filepath.Join(t.TempDir(), "loaded.json")
	runDoltProcessProof(t, base, "save", runID, artifact)
	runDoltProcessProof(t, base, "load", runID, artifact)

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

func runDoltProcessProof(t *testing.T, base, mode, runID, artifact string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestDoltCheckpointSuspendResumeAcrossProcesses$")
	cmd.Env = append(os.Environ(),
		"DOLT_PROCESS_PROOF_MODE="+mode,
		"DOLT_PROCESS_PROOF_DSN="+base,
		"DOLT_PROCESS_PROOF_RUN_ID="+runID,
		"DOLT_PROCESS_PROOF_ARTIFACT="+artifact,
	)
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "%s child failed:\n%s", mode, output)
}

func runDoltProcessProofChild(t *testing.T, mode string) {
	t.Helper()
	dsn := os.Getenv("DOLT_PROCESS_PROOF_DSN") + doltTestDB
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
// step's output (srd038 R1.4; srd036 R5).
func TestDoltCommandStateRehydratesThroughRealAdapter(t *testing.T) {
	base := startDoltServer(t)
	requireDoltTestDB(t, base)
	dsn := base + doltTestDB

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

// TestDoltCheckpointTerminalMergesAndDeletes proves the terminal lifecycle
// against the real adapter: a terminal Save merges the run branch to main and
// deletes it (DOLT_MERGE and DOLT_BRANCH('-d')), after which a fresh adapter
// resolves the finalized marker from main and reports ErrCheckpointFinalized
// (srd036 R4.3, R5).
func TestDoltCheckpointTerminalMergesAndDeletes(t *testing.T) {
	base := startDoltServer(t)
	requireDoltTestDB(t, base)
	dsn := base + doltTestDB

	runID := fmt.Sprintf("run-term-%d", time.Now().UnixNano())
	terminal := func(s core.State) bool { return s == "Done" }

	saver, err := core.OpenDoltCheckpoint(dsn, runID, terminal)
	require.NoError(t, err)
	exec := core.Execution{{
		Iteration: 1, CommandName: "finish", FromState: "Working", ToState: "Done",
		Signal: core.TaskCompleted,
		Result: core.DigestResult(core.Result{Signal: core.TaskCompleted}),
	}}
	require.NoError(t, saver.Save(core.Position{CurrentState: "Done", LastSignal: core.TaskCompleted}, exec))
	require.NoError(t, saver.Close())

	loader, err := core.OpenDoltCheckpoint(dsn, runID, terminal)
	require.NoError(t, err)
	defer func() { require.NoError(t, loader.Close()) }()

	gotPos, _, err := loader.Load()
	require.ErrorIs(t, err, core.ErrCheckpointFinalized)
	require.Equal(t, core.State("Done"), gotPos.CurrentState)
}

// TestDoltCheckpointRevertResetsBranch proves git-style rollback of the
// DB-persisted state: after two steps, reverting to step 0 resets the run branch
// to that commit (DOLT_RESET('--hard') resolved through dolt_log), so a reload
// sees only the first step (srd036 R6).
func TestDoltCheckpointRevertResetsBranch(t *testing.T) {
	base := startDoltServer(t)
	requireDoltTestDB(t, base)
	dsn := base + doltTestDB

	runID := fmt.Sprintf("run-rev-%d", time.Now().UnixNano())
	noMerge := func(core.State) bool { return false }

	adapter, err := core.OpenDoltCheckpoint(dsn, runID, noMerge)
	require.NoError(t, err)
	step0 := core.Execution{{
		Iteration: 1, CommandName: "first", FromState: "Start", ToState: "Working",
		Signal: core.LLMResponded, Result: core.DigestResult(core.Result{Signal: core.LLMResponded, Output: "first-output"}),
	}}
	require.NoError(t, adapter.Save(core.Position{CurrentState: "Working", LastSignal: core.LLMResponded}, step0))

	step1 := core.Execution{step0[0], {
		Iteration: 2, CommandName: "second", FromState: "Working", ToState: "Working",
		Signal: core.LLMResponded, Result: core.DigestResult(core.Result{Signal: core.LLMResponded, Output: "second-output"}),
	}}
	require.NoError(t, adapter.Save(core.Position{CurrentState: "Working", LastSignal: core.LLMResponded}, step1))

	require.NoError(t, adapter.Revert(runID, 0))
	require.NoError(t, adapter.Close())

	loader, err := core.OpenDoltCheckpoint(dsn, runID, noMerge)
	require.NoError(t, err)
	defer func() { require.NoError(t, loader.Close()) }()

	_, gotExec, err := loader.Load()
	require.NoError(t, err)
	require.Len(t, gotExec, 1, "revert to step 0 must drop the second step")
	require.Equal(t, "first", gotExec[0].CommandName)
	require.Equal(t, "first-output", gotExec[0].Result.Output)
}

func TestDoltCheckpointConversationReferencesAgainstRealDolt(t *testing.T) {
	base := startDoltServer(t)
	requireDoltTestDB(t, base)
	dsn := base + doltTestDB
	runID := fmt.Sprintf("run-ref-%d", time.Now().UnixNano())
	noMerge := func(core.State) bool { return false }

	saver, err := core.OpenDoltCheckpoint(dsn, runID, noMerge)
	require.NoError(t, err)
	firstPosition := core.Position{
		CurrentState: "Working", LastSignal: core.LLMResponded,
		Snapshot: core.AgentSnapshot{
			State: "Working", Signal: core.LLMResponded, Iteration: 1,
			Conversation: json.RawMessage(`[{"role":"user","content":"first"}]`),
			Domain:       json.RawMessage(`{"corpus":"first"}`),
		},
	}
	firstExecution := core.Execution{{
		Iteration: 1, CommandName: "first", FromState: "Start", ToState: "Working",
		Signal: core.LLMResponded, Result: core.DigestResult(core.Result{Signal: core.LLMResponded}),
	}}
	require.NoError(t, saver.Save(firstPosition, firstExecution))
	firstRef, ok := saver.ConversationReference()
	require.True(t, ok)
	firstDomainRef, ok := saver.DomainReference()
	require.True(t, ok)
	require.Equal(t, firstRef, firstDomainRef)

	secondPosition := firstPosition
	secondPosition.Snapshot.Iteration = 2
	secondPosition.Snapshot.Conversation = json.RawMessage(
		`[{"role":"user","content":"first"},{"role":"assistant","content":"second"}]`,
	)
	secondPosition.Snapshot.Domain = json.RawMessage(`{"corpus":"second"}`)
	secondExecution := append(firstExecution, core.Entry{
		Iteration: 2, CommandName: "second", FromState: "Working", ToState: "Working",
		Signal: core.LLMResponded, Result: core.DigestResult(core.Result{Signal: core.LLMResponded}),
	})
	require.NoError(t, saver.Save(secondPosition, secondExecution))
	require.NoError(t, saver.Close())

	fresh, err := core.OpenDoltCheckpoint(dsn, runID, noMerge)
	require.NoError(t, err)
	defer func() { require.NoError(t, fresh.Close()) }()
	_, _, err = fresh.Load()
	require.NoError(t, err)
	latestRef, ok := fresh.ConversationReference()
	require.True(t, ok)
	latestDomainRef, ok := fresh.DomainReference()
	require.True(t, ok)
	require.Equal(t, latestRef, latestDomainRef)
	resolved, err := fresh.ResolveConversationSnapshot(firstRef)
	require.NoError(t, err)
	require.JSONEq(t, string(firstPosition.Snapshot.Conversation), string(resolved))
	domain, err := fresh.ResolveDomainSnapshot(firstDomainRef)
	require.NoError(t, err)
	require.Equal(t, []byte(firstPosition.Snapshot.Domain), domain)
	domain, err = fresh.ResolveDomainSnapshot(latestDomainRef)
	require.NoError(t, err)
	require.Equal(t, []byte(secondPosition.Snapshot.Domain), domain)

	firstParts := strings.Split(firstRef, ":")
	latestParts := strings.Split(latestRef, ":")
	require.Len(t, firstParts, 6)
	require.Len(t, latestParts, 6)
	wrongStep := append([]string(nil), latestParts...)
	wrongStep[4] = firstParts[4]
	_, err = fresh.ResolveConversationSnapshot(strings.Join(wrongStep, ":"))
	require.ErrorIs(t, err, core.ErrConversationReferenceInvalid)
	_, err = fresh.ResolveDomainSnapshot(strings.Join(wrongStep, ":"))
	require.ErrorIs(t, err, core.ErrDomainReferenceInvalid)
	wrongRevision := append([]string(nil), latestParts...)
	wrongRevision[5] = firstParts[5]
	_, err = fresh.ResolveConversationSnapshot(strings.Join(wrongRevision, ":"))
	require.ErrorIs(t, err, core.ErrConversationReferenceInvalid)
	_, err = fresh.ResolveDomainSnapshot(strings.Join(wrongRevision, ":"))
	require.ErrorIs(t, err, core.ErrDomainReferenceInvalid)

	require.NoError(t, fresh.Revert(runID, 0))
	revertedRef, ok := fresh.ConversationReference()
	require.True(t, ok)
	require.Equal(t, firstRef, revertedRef)
	resolved, err = fresh.ResolveConversationSnapshot(revertedRef)
	require.NoError(t, err)
	require.JSONEq(t, string(firstPosition.Snapshot.Conversation), string(resolved))
	revertedDomainRef, ok := fresh.DomainReference()
	require.True(t, ok)
	require.Equal(t, firstDomainRef, revertedDomainRef)
	domain, err = fresh.ResolveDomainSnapshot(revertedDomainRef)
	require.NoError(t, err)
	require.Equal(t, []byte(firstPosition.Snapshot.Domain), domain)
}

func TestDoltBinaryPrefersExplicitFlag(t *testing.T) {
	previous := *doltBin
	t.Cleanup(func() { *doltBin = previous })
	*doltBin = " /declared/bin/dolt "

	require.Equal(t, "/declared/bin/dolt", doltBinary())
}

// doltBinary resolves the dolt executable: an explicit test flag wins,
// otherwise the binary is looked up on PATH. An empty result means no dolt is
// available and the gated tests skip.
func doltBinary() string {
	if configured := strings.TrimSpace(*doltBin); configured != "" {
		return configured
	}
	if path, err := exec.LookPath("dolt"); err == nil {
		return path
	}
	return ""
}

// startDoltServer launches a `dolt sql-server` from a prebuilt dolt binary on an
// ephemeral port against a throwaway data directory, waits until it accepts
// connections, and returns a database-less DSN base ("root@tcp(127.0.0.1:PORT)/").
// The server is torn down when the test ends. It skips when no dolt binary is
// installed so `go test ./...` stays green on machines without dolt, while
// `mage integration:dolt` runs it for real.
func startDoltServer(t *testing.T) string {
	t.Helper()
	bin := doltBinary()
	if bin == "" {
		t.Skip("no dolt binary found (install dolt or pass -dolt-bin); skipping Dolt integration test")
	}

	port := freeTCPPort(t)
	dataDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "sql-server",
		"--host=127.0.0.1",
		fmt.Sprintf("--port=%d", port),
		"--data-dir="+dataDir,
	)
	// DOLT_ROOT_HOST=% lets root connect over TCP from 127.0.0.1; the server
	// otherwise only provisions root@localhost. The empty password matches the DSN.
	cmd.Env = append(os.Environ(), "DOLT_ROOT_HOST=%", "DOLT_ROOT_PASSWORD=")
	var log bytes.Buffer
	cmd.Stdout = &log
	cmd.Stderr = &log
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})

	base := fmt.Sprintf("root@tcp(127.0.0.1:%d)/", port)
	waitForDolt(t, base, &log)
	return base
}

// freeTCPPort reserves an ephemeral port and releases it for the server to claim.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	return port
}

// waitForDolt polls the freshly started server until it answers a ping or the
// deadline passes, surfacing the captured server log on timeout.
func waitForDolt(t *testing.T, base string, log *bytes.Buffer) {
	t.Helper()
	db, err := sql.Open("dolt", base)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	deadline := time.Now().Add(60 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := db.PingContext(ctx)
		cancel()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("dolt sql-server did not become ready within 60s: %v\nserver log:\n%s", err, log.String())
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// requireDoltTestDB creates the shared test database on a database-less DSN base
// so subsequent adapters can select it.
func requireDoltTestDB(t *testing.T, base string) {
	t.Helper()
	db, err := sql.Open("dolt", base)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = db.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS "+doltTestDB)
	require.NoError(t, err)
}
