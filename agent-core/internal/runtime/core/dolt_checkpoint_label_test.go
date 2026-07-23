// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDoltCheckpointSchemaUpgradeAddsNullableLabel(t *testing.T) {
	t.Parallel()

	db := newFakeDB()
	db.executionStepsExists = true
	db.executionStepsHasLabel = false
	require.NoError(t, createSchema(db))
	require.NoError(t, createSchema(db), "schema upgrade is idempotent")

	var executionStepsCreate, labelUpgrade string
	for _, query := range db.calls {
		switch {
		case strings.Contains(query, "CREATE TABLE IF NOT EXISTS execution_steps"):
			executionStepsCreate = query
		case strings.Contains(query, "ALTER TABLE execution_steps"):
			labelUpgrade = query
		}
	}
	require.Contains(t, executionStepsCreate, "label VARCHAR(255)")
	require.Contains(t, labelUpgrade, "ADD COLUMN label VARCHAR(255)")
	require.NotContains(t, labelUpgrade, "NOT NULL")
	require.Equal(t, 1, countCalls(db.calls, "ADD COLUMN label"), "the upgrade runs once")

	legacy := sampleExecution()[:1]
	legacy[0].Label = ""
	checkpoint := NewDoltCheckpoint(db, "legacy-run", nil)
	require.NoError(t, checkpoint.Save(samplePosition(), legacy))

	fresh := NewDoltCheckpoint(db, "legacy-run", nil)
	_, restored, err := fresh.Load()
	require.NoError(t, err)
	require.Empty(t, restored[0].Label)
	output, ok := NewCommandStateView(restored).Lookup("invoke")
	require.True(t, ok, "a null label falls back to command_name")
	require.Equal(t, "hi", output)
}

func TestCommandStateAuthoredLabelPersistsAcrossDoltRestart(t *testing.T) {
	t.Parallel()

	spec, err := ParseMachineSpec([]byte(`
name: persisted-labels
initial_state: Start
states: [Start, First, Second, Finished]
terminal_states: [Finished]
signals: [Seed, FirstDone, SecondDone]
transitions:
  - state: Start
    signal: Seed
    next: First
    action: first_command
    label: repeated
  - state: First
    signal: FirstDone
    next: Second
    action: second_command
    label: repeated
  - state: Second
    signal: SecondDone
    next: Finished
`))
	require.NoError(t, err)

	db := newFakeDB()
	checkpoint := NewDoltCheckpoint(db, "labeled-run", nil)
	result, err := Loop(LoopParams{
		MachineSpec: &spec,
		Trace:       &loopRecorder{},
		Budget:      Budget{MaxIterations: 10},
		Checkpoint:  checkpoint,
		InitFunc: func(reg *Registry) error {
			reg.Register(
				ToolSpec{Name: "first_command", Visibility: Internal},
				labelOutputBuilder{name: "first_command", signal: "FirstDone"},
			)
			reg.Register(
				ToolSpec{Name: "second_command", Visibility: Internal},
				labelOutputBuilder{name: "second_command", signal: "SecondDone"},
			)
			return nil
		},
		Hooks: LoopHooks{
			TerminalStatus: func(State) RunStatus { return StatusSucceeded },
		},
	}, context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, result.Status)

	fresh := NewDoltCheckpoint(db, "labeled-run", nil)
	_, restored, err := fresh.Load()
	require.NoError(t, err)
	require.Len(t, restored, 2)
	require.Equal(t, "repeated", restored[0].Label)
	require.Equal(t, "first_command", restored[0].CommandName)
	require.Equal(t, "repeated", restored[1].Label)
	require.Equal(t, "second_command", restored[1].CommandName)
	require.Equal(t, "receipt:first_command", restored[0].Receipt)
	require.Equal(t, "receipt:second_command", restored[1].Receipt)

	view := NewCommandStateView(restored)
	value, err := ResolveFromSelector(view, "$from(repeated).value")
	require.NoError(t, err)
	require.Equal(t, "second_command", value, "duplicate restored labels resolve to the highest step")

	value, err = ResolveFromSelector(view, "$from(first_command).value")
	require.NoError(t, err, "restored entries retain command-name addressing")
	require.Equal(t, "first_command", value)

	_, err = ResolveFromSelector(view, "$from(repeated).receipt")
	var pathErr *UnresolvedPathError
	require.ErrorAs(t, err, &pathErr, "the forward view cannot resolve the separately restored receipt")
}

type labelOutputBuilder struct {
	name   string
	signal Signal
}

func (b labelOutputBuilder) Build(Result) Command {
	return labelOutputCommand(b)
}

type labelOutputCommand labelOutputBuilder

func (c labelOutputCommand) Name() string { return c.name }

func (c labelOutputCommand) Execute() Result {
	return Result{
		Output:      `{"value":"` + c.name + `"}`,
		Signal:      c.signal,
		CommandName: c.name,
		Receipt:     "receipt:" + c.name,
	}
}

func (c labelOutputCommand) Undo(Result) Result { return NoopUndo(c.name) }
