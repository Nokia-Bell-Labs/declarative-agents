// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func samplePositionExecution() (Position, Execution) {
	pos := Position{
		CurrentState: State("Reading"),
		LastSignal:   ToolDone,
		Snapshot: AgentSnapshot{
			State:        State("Reading"),
			Signal:       ToolDone,
			Iteration:    2,
			TokensIn:     10,
			TokensOut:    20,
			TotalCost:    1.5,
			Conversation: json.RawMessage(`[{"role":"user","content":"hi"}]`),
		},
	}
	exec := Execution{
		{
			Iteration:   1,
			Timestamp:   time.Unix(1000, 0).UTC(),
			CommandName: "write",
			FromState:   State("Start"),
			ToState:     State("Reading"),
			Signal:      ToolDone,
			Result:      ResultDigest{Signal: ToolDone, Output: "ok"},
			Receipt:     `{"path":"a.txt","previous":null}`,
		},
		{
			Iteration:   2,
			Timestamp:   time.Unix(2000, 0).UTC(),
			CommandName: "read",
			FromState:   State("Reading"),
			ToState:     State("Reading"),
			Signal:      ToolDone,
			Result:      ResultDigest{Signal: ToolDone},
		},
	}
	return pos, exec
}

func TestNoopCheckpointSaveIsNoopAndLoadReportsNotFound(t *testing.T) {
	var cp Checkpoint = NoopCheckpoint{}
	pos, exec := samplePositionExecution()
	require.NoError(t, cp.Save(pos, exec))

	_, _, err := cp.Load()
	require.ErrorIs(t, err, ErrNoCheckpoint)
}

func TestInMemoryCheckpointRoundTripsConversationAndReceipts(t *testing.T) {
	var cp Checkpoint = &InMemoryCheckpoint{}
	pos, exec := samplePositionExecution()
	require.NoError(t, cp.Save(pos, exec))

	gotPos, gotExec, err := cp.Load()
	require.NoError(t, err)
	require.Equal(t, pos.CurrentState, gotPos.CurrentState)
	require.Equal(t, pos.LastSignal, gotPos.LastSignal)
	require.Equal(t, pos.Snapshot.Iteration, gotPos.Snapshot.Iteration)
	require.JSONEq(t, string(pos.Snapshot.Conversation), string(gotPos.Snapshot.Conversation))
	require.Equal(t, exec, gotExec)
	require.Equal(t, `{"path":"a.txt","previous":null}`, gotExec[0].Receipt)
	require.Empty(t, gotExec[1].Receipt)
}

func TestInMemoryCheckpointLoadNotFoundBeforeSave(t *testing.T) {
	cp := &InMemoryCheckpoint{}
	_, _, err := cp.Load()
	require.ErrorIs(t, err, ErrNoCheckpoint)
}

func TestInMemoryCheckpointIsolatesCallerMutation(t *testing.T) {
	cp := &InMemoryCheckpoint{}
	pos, exec := samplePositionExecution()
	require.NoError(t, cp.Save(pos, exec))

	// Mutate the caller's copies and the values returned from Load.
	exec[0].Receipt = "tampered"
	gotPos, gotExec, err := cp.Load()
	require.NoError(t, err)
	gotExec[1].CommandName = "tampered"
	gotPos.Snapshot.Conversation[0] = 'X'

	reloadPos, reloadExec, err := cp.Load()
	require.NoError(t, err)
	require.Equal(t, `{"path":"a.txt","previous":null}`, reloadExec[0].Receipt)
	require.Equal(t, "read", reloadExec[1].CommandName)
	require.JSONEq(t, `[{"role":"user","content":"hi"}]`, string(reloadPos.Snapshot.Conversation))
}

func TestFileCheckpointRoundTripsAcrossProcesses(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	pos, exec := samplePositionExecution()

	// Distinct instances over the same root simulate separate processes.
	writer := NewFileCheckpoint(root)
	require.NoError(t, writer.Save(pos, exec))

	reader := NewFileCheckpoint(root)
	gotPos, gotExec, err := reader.Load()
	require.NoError(t, err)
	require.Equal(t, pos.CurrentState, gotPos.CurrentState)
	require.JSONEq(t, string(pos.Snapshot.Conversation), string(gotPos.Snapshot.Conversation))
	require.Equal(t, exec, gotExec)
}

func TestFileCheckpointLoadReportsNotFound(t *testing.T) {
	cp := NewFileCheckpoint(filepath.Join(t.TempDir(), "empty"))
	_, _, err := cp.Load()
	require.ErrorIs(t, err, ErrNoCheckpoint)
}

func TestCheckpointWireFormat(t *testing.T) {
	// Position serializes with current_state, last_signal, and a nested snapshot.
	pos := Position{CurrentState: State("Reading"), LastSignal: ToolDone}
	data, err := json.Marshal(pos)
	require.NoError(t, err)
	var obj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &obj))
	require.Contains(t, obj, "current_state")
	require.Contains(t, obj, "last_signal")
	require.Contains(t, obj, "snapshot")

	// Empty Execution serializes as JSON null.
	empty, err := json.Marshal(Execution(nil))
	require.NoError(t, err)
	require.Equal(t, "null", string(empty))

	// Receipt is omitted when empty and present as a JSON string when set.
	noReceipt, err := json.Marshal(Entry{CommandName: "read"})
	require.NoError(t, err)
	require.NotContains(t, string(noReceipt), "receipt")

	withReceipt, err := json.Marshal(Entry{CommandName: "write", Receipt: `{"path":"a"}`})
	require.NoError(t, err)
	require.Contains(t, string(withReceipt), `"receipt":"{\"path\":\"a\"}"`)
}
