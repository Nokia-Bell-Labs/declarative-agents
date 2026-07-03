// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type alwaysCheckpointPolicy struct{}

func (alwaysCheckpointPolicy) ShouldCheckpoint(CheckpointEvent) bool { return true }

var _ CheckpointPolicy = alwaysCheckpointPolicy{}

func TestCheckpointContractsCompileAndRoundTrip(t *testing.T) {
	t.Parallel()

	cp := CheckpointRecord{
		ID:        "cp-1",
		Iteration: 2,
		Timestamp: time.Unix(100, 0).UTC(),
		AgentState: AgentSnapshot{
			State:     State("Working"),
			Signal:    Signal("ToolDone"),
			Iteration: 2,
			TokensIn:  10,
			TokensOut: 5,
			TotalCost: 0.25,
		},
		ConversationLog: json.RawMessage(`[{"role":"user","content":"hello"}]`),
		DomainState:     json.RawMessage(`{"conversation_len":3}`),
		WorkspaceRef:    "abc123",
		History: []HistoryDigest{{
			Iteration:    2,
			CommandName:  "write",
			FromState:    State("Composing"),
			ToState:      State("Parsing"),
			Signal:       Signal("ToolDone"),
			WorkspaceRef: "abc123",
		}},
	}

	data, err := json.Marshal(cp)
	require.NoError(t, err)

	var got CheckpointRecord
	require.NoError(t, json.Unmarshal(data, &got))
	require.Equal(t, cp.ID, got.ID)
	require.Equal(t, cp.AgentState.State, got.AgentState.State)
	require.Equal(t, cp.WorkspaceRef, got.WorkspaceRef)
	require.JSONEq(t, string(cp.ConversationLog), string(got.ConversationLog))
	require.JSONEq(t, string(cp.DomainState), string(got.DomainState))
	require.Len(t, got.History, 1)
	require.Equal(t, "write", got.History[0].CommandName)
}

func TestNoopUndoReturnsSuccessfulResult(t *testing.T) {
	t.Parallel()

	res := NoopUndo("read")

	require.Equal(t, ToolDone, res.Signal)
	require.Equal(t, "read", res.CommandName)
	require.Contains(t, res.Output, "no-op")
}
