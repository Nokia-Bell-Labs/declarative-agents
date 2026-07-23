// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type secretOutputCommand struct{}

func (secretOutputCommand) Name() string { return "fetch_secret" }
func (secretOutputCommand) Execute() Result {
	return Result{
		CommandName: "fetch_secret",
		Signal:      ToolDone,
		Output:      `{"credentials":{"token":"secret","owner":"alice"},"public":"ok"}`,
		Redaction: OutputRedaction{
			Version: OutputRedactionVersion1,
			Paths:   []OutputRedactionPath{{"credentials", "token"}},
		},
	}
}
func (secretOutputCommand) Undo(Result) Result { return NoopUndo("fetch_secret") }

func TestOutputRedactionRemovesFieldBeforeLiveEntry(t *testing.T) {
	t.Parallel()

	result := (secretOutputCommand{}).Execute()
	entry := dispatchEntry(1, "start", "done", Seed, "fetch", result)

	require.Contains(t, result.Output, "secret", "adjacent Result behavior remains unchanged")
	require.JSONEq(t, `{"credentials":{"owner":"alice"},"public":"ok"}`, entry.Result.Output)
	require.NotContains(t, entry.Result.Output, "secret")
	require.Equal(t, OutputRedactionVersion1, entry.Result.RedactionVersion)
	require.Equal(t, OutputRedactionApplied, entry.Result.RedactionStatus)
	require.Equal(t, []OutputRedactionPath{{"credentials", "token"}}, entry.Result.RedactedPaths)

	view := NewCommandStateView(Execution{entry})
	value, err := ResolveFromSelector(view, "$from(fetch).credentials.owner")
	require.NoError(t, err)
	require.Equal(t, "alice", value)

	_, err = ResolveFromSelector(view, "$from(fetch).credentials.token")
	var missing *UnresolvedPathError
	require.ErrorAs(t, err, &missing)
}

func TestOutputRedactionIsIdempotent(t *testing.T) {
	t.Parallel()

	paths := []OutputRedactionPath{{"credentials", "token"}}
	first, firstPaths, firstStatus := applyOutputRedaction(
		`{"credentials":{"token":"secret","owner":"alice"}}`,
		OutputRedactionVersion1,
		paths,
	)
	second, secondPaths, secondStatus := applyOutputRedaction(
		first,
		OutputRedactionVersion1,
		firstPaths,
	)

	require.JSONEq(t, first, second)
	require.Equal(t, OutputRedactionApplied, firstStatus)
	require.Equal(t, firstStatus, secondStatus)
	require.Equal(t, firstPaths, secondPaths)
}

func TestCommandStateViewReappliesMarkedPaths(t *testing.T) {
	t.Parallel()

	view := NewCommandStateView(Execution{{
		CommandName: "synthetic",
		Result: ResultDigest{
			Output:           `{"secret":"raw","public":"ok"}`,
			RedactionVersion: OutputRedactionVersion1,
			RedactedPaths:    []OutputRedactionPath{{"secret"}},
			RedactionStatus:  OutputRedactionApplied,
		},
	}})

	value, err := ResolveFromSelector(view, "$from(synthetic).public")
	require.NoError(t, err)
	require.Equal(t, "ok", value)
	_, err = ResolveFromSelector(view, "$from(synthetic).secret")
	var missing *UnresolvedPathError
	require.ErrorAs(t, err, &missing)
}

func TestOutputRedactionFailsClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		output    string
		redaction OutputRedaction
	}{
		{
			name:   "unknown version",
			output: `{"secret":"value"}`,
			redaction: OutputRedaction{
				Version: 99,
				Paths:   []OutputRedactionPath{{"secret"}},
			},
		},
		{
			name:   "version zero with paths",
			output: `{"secret":"value"}`,
			redaction: OutputRedaction{
				Paths: []OutputRedactionPath{{"secret"}},
			},
		},
		{
			name:   "empty path",
			output: `{"secret":"value"}`,
			redaction: OutputRedaction{
				Version: OutputRedactionVersion1,
				Paths:   []OutputRedactionPath{{}},
			},
		},
		{
			name:   "whitespace segment",
			output: `{"secret":"value"}`,
			redaction: OutputRedaction{
				Version: OutputRedactionVersion1,
				Paths:   []OutputRedactionPath{{" secret"}},
			},
		},
		{
			name:   "dotted segment",
			output: `{"nested":{"secret":"value"}}`,
			redaction: OutputRedaction{
				Version: OutputRedactionVersion1,
				Paths:   []OutputRedactionPath{{"nested.secret"}},
			},
		},
		{
			name:   "non object output",
			output: `"secret"`,
			redaction: OutputRedaction{
				Version: OutputRedactionVersion1,
				Paths:   []OutputRedactionPath{{"secret"}},
			},
		},
		{
			name:   "scalar intermediate",
			output: `{"nested":"secret"}`,
			redaction: OutputRedaction{
				Version: OutputRedactionVersion1,
				Paths:   []OutputRedactionPath{{"nested", "secret"}},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			entry := dispatchEntry(1, "start", "done", Seed, "unsafe", Result{
				CommandName: "unsafe",
				Output:      tc.output,
				Redaction:   tc.redaction,
			})

			require.Empty(t, entry.Result.Output)
			require.Empty(t, entry.Result.RedactedPaths)
			require.Equal(t, OutputRedactionOmitted, entry.Result.RedactionStatus)

			view := NewCommandStateView(Execution{entry})
			_, ok := view.Lookup("unsafe")
			require.False(t, ok)
			_, err := ResolveFromSelector(view, "$from(unsafe).secret")
			var unavailable *CommandStateOutputUnavailableError
			require.True(t, errors.As(err, &unavailable))
		})
	}
}

func TestCommandStateRejectsUnversionedLegacyOutput(t *testing.T) {
	t.Parallel()

	view := NewCommandStateView(Execution{{
		CommandName: "legacy",
		Label:       "old",
		Result:      ResultDigest{Output: `{"secret":"persisted"}`},
		Receipt:     `{"rollback":"still-available"}`,
	}})

	output, ok := view.Lookup("old")
	require.False(t, ok)
	require.Empty(t, output)

	_, err := ResolveFromSelector(view, "$from(old).secret")
	var unavailable *CommandStateOutputUnavailableError
	require.ErrorAs(t, err, &unavailable)
	require.Equal(t, "old", unavailable.Label)
	require.Zero(t, unavailable.Version)
	require.Empty(t, unavailable.Status)
}
