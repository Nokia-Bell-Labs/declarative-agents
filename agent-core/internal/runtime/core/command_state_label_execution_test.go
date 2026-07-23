// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMachineSpecCommandStateLabels(t *testing.T) {
	t.Parallel()

	for _, label := range []string{"", "collect.input", "collect input", "collect(input)"} {
		label := label
		t.Run(fmt.Sprintf("invalid_%q", label), func(t *testing.T) {
			t.Parallel()
			_, err := ParseMachineSpec([]byte(commandStateLabelMachineYAML(label)))
			require.ErrorContains(t, err, "transition[0].label")
			require.ErrorContains(t, err, "not a valid command-state label")
		})
	}

	t.Run("duplicate labels remain valid", func(t *testing.T) {
		t.Parallel()
		_, err := ParseMachineSpec([]byte(`
name: duplicate-labels
initial_state: Start
states: [Start, Working, Finished]
terminal_states: [Finished]
signals: [Seed, Done]
transitions:
  - state: Start
    signal: Seed
    next: Working
    action: collect
    label: repeated
  - state: Working
    signal: Done
    next: Finished
    label: repeated
`))
		require.NoError(t, err)
	})
}

func TestCommandStateAuthoredLabelMachineExecution(t *testing.T) {
	t.Parallel()

	t.Run("named action", func(t *testing.T) {
		t.Parallel()
		entry := runCommandStateLabelMachine(t, "collect", "gather-input", nil)
		require.Equal(t, "gather-input", entry.Label)
		require.Equal(t, "collect", entry.CommandName)
	})

	t.Run("dynamic tool keeps both addresses", func(t *testing.T) {
		t.Parallel()
		entry := runCommandStateLabelMachine(
			t,
			"$tool",
			"choose-reviewer",
			func(Result) Command {
				return &fakeCmd{name: "review_security", signal: Signal("Done")}
			},
		)
		require.Equal(t, "choose-reviewer", entry.Label)
		require.Equal(t, "review_security", entry.CommandName)
	})

	t.Run("programmatic table has command-name fallback", func(t *testing.T) {
		t.Parallel()
		checkpoint := &InMemoryCheckpoint{}
		params := simpleLoopParams(&loopRecorder{})
		params.Checkpoint = checkpoint

		_, err := Loop(params, context.Background())
		require.NoError(t, err)
		_, execution, err := checkpoint.Load()
		require.NoError(t, err)
		require.NotEmpty(t, execution)
		require.Empty(t, execution[0].Label)

		_, ok := NewCommandStateView(execution).Lookup("step_a")
		require.True(t, ok)
	})
}

func runCommandStateLabelMachine(
	t *testing.T,
	action string,
	label string,
	toolAction ActionFunc,
) Entry {
	t.Helper()

	spec, err := ParseMachineSpec([]byte(commandStateLabelMachineYAML(label)))
	require.NoError(t, err)
	spec.Transitions[0].Action = action

	checkpoint := &InMemoryCheckpoint{}
	params := LoopParams{
		MachineSpec: &spec,
		Trace:       &loopRecorder{},
		Budget:      Budget{MaxIterations: 10},
		Checkpoint:  checkpoint,
		ToolAction:  toolAction,
		Hooks: LoopHooks{
			TerminalStatus: func(State) RunStatus { return StatusSucceeded },
		},
	}
	if action != "$tool" {
		params.InitFunc = func(reg *Registry) error {
			reg.Register(
				ToolSpec{Name: action, Visibility: Internal},
				&fakeBuilder{name: action, signal: Signal("Done")},
			)
			return nil
		}
	}

	result, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, result.Status)

	_, execution, err := checkpoint.Load()
	require.NoError(t, err)
	require.Len(t, execution, 1)
	return execution[0]
}

func commandStateLabelMachineYAML(label string) string {
	return fmt.Sprintf(`
name: command-state-label
initial_state: Start
states: [Start, Working, Finished]
terminal_states: [Finished]
signals: [Seed, Done]
transitions:
  - state: Start
    signal: Seed
    next: Working
    action: collect
    label: %q
  - state: Working
    signal: Done
    next: Finished
`, label)
}
