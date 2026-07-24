// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"math"
	"testing"
	"time"
	"unicode/utf8"

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
			Result:      checkpointDigest(ToolDone, "ok", Cost{}),
			Receipt:     `{"path":"a.txt","previous":null}`,
		},
		{
			Iteration:   2,
			Timestamp:   time.Unix(2000, 0).UTC(),
			CommandName: "read",
			FromState:   State("Reading"),
			ToState:     State("Reading"),
			Signal:      ToolDone,
			Result:      checkpointDigest(ToolDone, "", Cost{}),
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

func TestCheckpointJSONRoundTripPreservesBoundaryValues(t *testing.T) {
	t.Parallel()
	timestamp := time.Date(2262, time.April, 11, 23, 47, 16, 854775807, time.UTC)
	original := struct {
		Position  Position  `json:"position"`
		Execution Execution `json:"execution"`
	}{
		Position: Position{
			CurrentState: "Σ-working",
			LastSignal:   "Signal\u0001",
			Snapshot: AgentSnapshot{
				State: "Σ-working", Signal: "Signal\u0001",
				Iteration: math.MaxInt, TokensIn: math.MaxInt, TokensOut: math.MaxInt,
				TotalCost: math.MaxFloat64,
				Conversation: json.RawMessage(`[
					{"role":"user","content":"hello \u2603"},
					{"role":"assistant","content":"line 1\nline 2"}
				]`),
			},
		},
		Execution: Execution{{
			Iteration: math.MaxInt, Timestamp: timestamp, CommandName: "unicode-Δ",
			FromState: "Σ-working", ToState: "Done", Signal: "Signal\u0001",
			Result: ResultDigest{
				Signal: "Signal\u0001", Output: "output\u0000snowman ☃", Error: "error\nline",
				Cost: Cost{Duration: time.Duration(math.MaxInt64), TokensIn: math.MaxInt, TokensOut: math.MaxInt, Dollars: math.MaxFloat64},
			},
			Receipt: "opaque\u0000receipt\n☃",
		}},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)
	var restored struct {
		Position  Position  `json:"position"`
		Execution Execution `json:"execution"`
	}
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, original.Position.CurrentState, restored.Position.CurrentState)
	require.Equal(t, original.Position.Snapshot.Iteration, restored.Position.Snapshot.Iteration)
	require.Equal(t, original.Position.Snapshot.TotalCost, restored.Position.Snapshot.TotalCost)
	require.JSONEq(t, string(original.Position.Snapshot.Conversation), string(restored.Position.Snapshot.Conversation))
	require.Equal(t, original.Execution, restored.Execution)
	require.Equal(t, original.Execution[0].Receipt, restored.Execution[0].Receipt)
	require.True(t, original.Execution[0].Timestamp.Equal(restored.Execution[0].Timestamp))
}

func TestInMemoryCheckpointPreservesNilAndEmptyExecution(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		execution Execution
		wantNil   bool
	}{
		{name: "nil", execution: nil, wantNil: true},
		{name: "empty", execution: Execution{}, wantNil: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cp := &InMemoryCheckpoint{}
			require.NoError(t, cp.Save(Position{}, tt.execution))
			_, restored, err := cp.Load()
			require.NoError(t, err)
			require.Equal(t, tt.wantNil, restored == nil)
			encoded, err := json.Marshal(restored)
			require.NoError(t, err)
			if tt.wantNil {
				require.Equal(t, "null", string(encoded))
			} else {
				require.Equal(t, "[]", string(encoded))
			}
		})
	}
}

func TestInMemoryCheckpointRejectsInvalidConversationJSON(t *testing.T) {
	t.Parallel()
	cp := &InMemoryCheckpoint{}
	err := cp.Save(Position{Snapshot: AgentSnapshot{Conversation: json.RawMessage(`{"broken":`)}}, nil)
	require.ErrorContains(t, err, "conversation is not valid JSON")
	_, _, loadErr := cp.Load()
	require.ErrorIs(t, loadErr, ErrNoCheckpoint)
}

func TestInMemoryCheckpointReappliesOutputRedaction(t *testing.T) {
	t.Parallel()

	cp := &InMemoryCheckpoint{}
	execution := Execution{redactionCheckpointEntry("memory-secret")}
	require.NoError(t, cp.Save(Position{}, execution))

	// Caller mutation after Save cannot alter either the output or path metadata.
	execution[0].Result.Output = `{"secret":"mutated"}`
	execution[0].Result.RedactedPaths[0][0] = "public"

	_, restored, err := cp.Load()
	require.NoError(t, err)
	require.JSONEq(t, `{"public":"ok"}`, restored[0].Result.Output)
	require.Equal(t, []OutputRedactionPath{{"secret"}}, restored[0].Result.RedactedPaths)
	require.Equal(t, `{"opaque":"receipt"}`, restored[0].Receipt)

	view := NewCommandStateView(restored)
	value, err := ResolveFromSelector(view, "$from(fetch).public")
	require.NoError(t, err)
	require.Equal(t, "ok", value)
	_, err = ResolveFromSelector(view, "$from(fetch).secret")
	var missing *UnresolvedPathError
	require.ErrorAs(t, err, &missing)
}

func TestInMemoryCheckpointPersistsFailClosedRedaction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*ResultDigest)
	}{
		{
			name: "unknown version",
			mutate: func(result *ResultDigest) {
				result.RedactionVersion = 99
			},
		},
		{
			name: "malformed path",
			mutate: func(result *ResultDigest) {
				result.RedactedPaths = []OutputRedactionPath{{}}
			},
		},
		{
			name: "marked non-object",
			mutate: func(result *ResultDigest) {
				result.Output = `"second-secret"`
			},
		},
		{
			name: "missing metadata",
			mutate: func(result *ResultDigest) {
				result.RedactionVersion = 0
				result.RedactionStatus = ""
			},
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cp := &InMemoryCheckpoint{}
			invalid := Execution{redactionCheckpointEntry("second-secret")}
			invalid[0].Result.Signal = ToolFailed
			invalid[0].Result.Error = "lifecycle-visible"
			invalid[0].Result.Cost = Cost{TokensIn: 7}
			tc.mutate(&invalid[0].Result)
			require.NoError(t, cp.Save(Position{CurrentState: "FailClosed"}, invalid))

			position, restored, loadErr := cp.Load()
			require.NoError(t, loadErr)
			require.Equal(t, State("FailClosed"), position.CurrentState)
			require.Empty(t, restored[0].Result.Output)
			require.Equal(t, OutputRedactionVersion1, restored[0].Result.RedactionVersion)
			require.Empty(t, restored[0].Result.RedactedPaths)
			require.Equal(t, OutputRedactionOmitted, restored[0].Result.RedactionStatus)
			require.Equal(t, ToolFailed, restored[0].Result.Signal)
			require.Equal(t, "lifecycle-visible", restored[0].Result.Error)
			require.Equal(t, 7, restored[0].Result.Cost.TokensIn)
			require.Equal(t, `{"opaque":"receipt"}`, restored[0].Receipt)

			_, err := ResolveFromSelector(NewCommandStateView(restored), "$from(fetch).secret")
			var unavailable *CommandStateOutputUnavailableError
			require.ErrorAs(t, err, &unavailable)
		})
	}
}

func redactionCheckpointEntry(secret string) Entry {
	return Entry{
		CommandName: "fetch",
		Label:       "fetch",
		Result: ResultDigest{
			Output:           `{"secret":"` + secret + `","public":"ok"}`,
			RedactionVersion: OutputRedactionVersion1,
			RedactedPaths:    []OutputRedactionPath{{"secret"}},
			RedactionStatus:  OutputRedactionApplied,
		},
		Receipt: `{"opaque":"receipt"}`,
	}
}

func FuzzCheckpointReceiptJSONRoundTrip(f *testing.F) {
	f.Add("opaque receipt")
	f.Add("unicode ☃")
	f.Add("control\u0000newline\n")
	f.Add("")
	f.Fuzz(func(t *testing.T, receipt string) {
		if !utf8.ValidString(receipt) {
			t.Skip()
		}
		original := Entry{CommandName: "word", Receipt: receipt}
		data, err := json.Marshal(original)
		require.NoError(t, err)
		var restored Entry
		require.NoError(t, json.Unmarshal(data, &restored))
		require.Equal(t, receipt, restored.Receipt)
	})
}
