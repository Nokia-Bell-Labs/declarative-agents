// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func appliedEntry(label, command, output, toState string, signal Signal, iteration int) Entry {
	return Entry{
		Iteration:   iteration,
		CommandName: command,
		Label:       label,
		ToState:     State(toState),
		Signal:      signal,
		Result: ResultDigest{
			Signal:           signal,
			Output:           output,
			RedactionVersion: OutputRedactionVersion1,
			RedactionStatus:  OutputRedactionApplied,
		},
	}
}

func TestLiveCommandStateSource_ExposesLatestAppliedOutput(t *testing.T) {
	t.Parallel()

	source := NewLiveCommandStateSource()
	source.ObserveCommandState(Execution{
		appliedEntry("polled_step", "rest_client_get", `{"agents":[{"name":"a"}]}`, "Polling", ToolDone, 3),
	})

	entry, found := source.LookupCommandState("polled_step")
	require.True(t, found)
	require.True(t, entry.Available)
	require.Equal(t, `{"agents":[{"name":"a"}]}`, entry.Output)
	require.Equal(t, "Polling", entry.State)
	require.Equal(t, string(ToolDone), entry.Signal)
	require.Equal(t, 3, entry.Iteration)

	_, found = source.LookupCommandState("never_ran")
	require.False(t, found)
}

func TestLiveCommandStateSource_CommandNameFallbackAndNewestWins(t *testing.T) {
	t.Parallel()

	source := NewLiveCommandStateSource()
	source.ObserveCommandState(Execution{
		appliedEntry("", "list_pods", `{"pods":["old"]}`, "Discovering", ToolDone, 1),
		appliedEntry("", "list_pods", `{"pods":["new"]}`, "Discovering", ToolDone, 4),
	})

	entry, found := source.LookupCommandState("list_pods")
	require.True(t, found)
	require.Equal(t, `{"pods":["new"]}`, entry.Output)
	require.Equal(t, 4, entry.Iteration)
}

func TestLiveCommandStateSource_OmittedAndLegacyOutputUnavailable(t *testing.T) {
	t.Parallel()

	source := NewLiveCommandStateSource()
	source.ObserveCommandState(Execution{
		{
			CommandName: "omitted_step", ToState: "S", Signal: ToolDone,
			Result: ResultDigest{
				RedactionVersion: OutputRedactionVersion1,
				RedactionStatus:  OutputRedactionOmitted,
			},
		},
		{
			CommandName: "legacy_step", ToState: "S", Signal: ToolDone,
			Result: ResultDigest{Output: `{"secret":"x"}`},
		},
	})

	omitted, found := source.LookupCommandState("omitted_step")
	require.True(t, found)
	require.False(t, omitted.Available)
	require.Empty(t, omitted.Output)

	legacy, found := source.LookupCommandState("legacy_step")
	require.True(t, found)
	require.False(t, legacy.Available)
	require.Empty(t, legacy.Output)
}
