// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUndoMementoRoundTripJSON(t *testing.T) {
	t.Parallel()
	memento, err := NewUndoMemento("write", UndoMementoReversible, map[string]string{
		"path":        "notes.txt",
		"before_hash": "abc123",
	})
	require.NoError(t, err)

	data, err := json.Marshal(memento)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"version": 1,
		"kind": "reversible",
		"command_name": "write",
		"payload": {
			"path": "notes.txt",
			"before_hash": "abc123"
		}
	}`, string(data))

	var got UndoMemento
	require.NoError(t, json.Unmarshal(data, &got))
	require.NoError(t, ValidateUndoMemento(got))
	require.Equal(t, UndoMementoReversible, got.Kind)
}

func TestUndoMementoKindsValidate(t *testing.T) {
	t.Parallel()
	cases := []UndoMemento{
		NoopUndoMemento("read"),
		IrreversibleUndoMemento("publish", "external service accepted the request"),
		{
			Version:     UndoMementoVersion,
			Kind:        UndoMementoCompensatable,
			CommandName: "create_issue",
			Payload:     json.RawMessage(`{"issue_id":"agent-core-1"}`),
		},
	}

	for _, tc := range cases {
		require.NoError(t, ValidateUndoMemento(tc), tc.Kind)
	}
}

func TestUndoMementoValidationErrorsAreClassified(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, ValidateUndoMemento(UndoMemento{}), ErrUndoMementoMissing)
	require.ErrorIs(t, ValidateUndoMemento(UndoMemento{
		Version:     2,
		Kind:        UndoMementoNoop,
		CommandName: "read",
	}), ErrUndoMementoIncompatible)
	require.ErrorIs(t, ValidateUndoMemento(UndoMemento{
		Version:     UndoMementoVersion,
		Kind:        UndoMementoReversible,
		CommandName: "write",
	}), ErrUndoMementoMissing)
	require.ErrorIs(t, ValidateUndoMemento(UndoMemento{
		Version:     UndoMementoVersion,
		Kind:        UndoMementoReversible,
		CommandName: "write",
		Payload:     json.RawMessage(`{`),
	}), ErrUndoMementoIncompatible)
	require.ErrorIs(t, ValidateUndoMemento(UndoMemento{
		Version:     UndoMementoVersion,
		Kind:        UndoMementoKind("unknown"),
		CommandName: "write",
	}), ErrUndoMementoIncompatible)

	_, err := NewUndoMemento("bad", UndoMementoReversible, func() {})
	require.True(t, errors.Is(err, ErrUndoMementoIncompatible))
}

func TestHistoryDigestPreservesUndoMementoCopy(t *testing.T) {
	t.Parallel()
	payload := json.RawMessage(`{"path":"notes.txt"}`)
	history := History{{
		Iteration:   1,
		CommandName: "write",
		FromState:   "Editing",
		ToState:     "Parsing",
		Result:      ResultDigest{Signal: ToolDone},
		Undo: &UndoMemento{
			Version:     UndoMementoVersion,
			Kind:        UndoMementoReversible,
			CommandName: "write",
			Payload:     payload,
		},
	}}

	digest := historyDigest(history)
	require.Len(t, digest, 1)
	require.NoError(t, ValidateUndoMemento(*digest[0].Undo))

	history[0].Undo.Payload[0] = '['
	require.JSONEq(t, `{"path":"notes.txt"}`, string(digest[0].Undo.Payload))

	restored := historyFromDigest(digest)
	require.Len(t, restored, 1)
	require.NoError(t, ValidateUndoMemento(*restored[0].Undo))

	digest[0].Undo.Payload[0] = '['
	require.JSONEq(t, `{"path":"notes.txt"}`, string(restored[0].Undo.Payload))
}
