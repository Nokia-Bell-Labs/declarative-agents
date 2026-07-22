// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestLoop_DeclarativeInit(t *testing.T) {
	t.Parallel()

	machineYAML := `
name: test-machine
initial_state: Start
states: [Start, Working, Finished, Failed]
terminal_states: [Finished, Failed]
signals: [Seed, Done, TaskCompleted, BudgetExhausted, CommandError]
transitions:
  - state: Start
    signal: Seed
    next: Working
    action: step_a
  - state: Working
    signal: Done
    next: Working
    action: step_b
  - state: Working
    signal: TaskCompleted
    next: Finished
  - state: Working
    signal: BudgetExhausted
    next: Failed
  - state: Working
    signal: CommandError
    next: Failed
`
	dir := t.TempDir()
	machineFile := dir + "/machine.yaml"
	if err := os.WriteFile(machineFile, []byte(machineYAML), 0644); err != nil {
		t.Fatal(err)
	}

	tr := &loopRecorder{}

	params := LoopParams{
		Prompt:      "test",
		MachineFile: machineFile,
		Trace:       tr,
		Budget:      Budget{MaxIterations: 100},
		InitFunc: func(reg *Registry) error {
			reg.Register(ToolSpec{Name: "step_a", Visibility: Internal}, &fakeBuilder{name: "step_a", signal: Signal("Done")})
			reg.Register(ToolSpec{Name: "step_b", Visibility: Internal}, &fakeBuilder{name: "step_b", signal: Signal("TaskCompleted")})
			return nil
		},
		Hooks: LoopHooks{
			TaskCompletedSignal: Signal("TaskCompleted"),
			TerminalStatus: func(s State) RunStatus {
				if s == "Finished" {
					return StatusSucceeded
				}
				return StatusFailed
			},
		},
	}

	rr, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, rr.Status)
	require.Equal(t, State("Finished"), rr.FinalState)
	require.Equal(t, 2, rr.Iterations)
}

func TestLoop_DeclarativeSuspendAction(t *testing.T) {
	t.Parallel()

	machineYAML := `
name: suspend-machine
initial_state: Start
states: [Start, AwaitingApproval, Failed]
terminal_states: [Failed]
signals: [Seed, AwaitApproval]
transitions:
  - state: Start
    signal: Seed
    next: AwaitingApproval
    action: suspend
`
	dir := t.TempDir()
	machineFile := dir + "/machine.yaml"
	require.NoError(t, os.WriteFile(machineFile, []byte(machineYAML), 0o644))

	params := LoopParams{
		Prompt:      "test",
		MachineFile: machineFile,
		Trace:       &loopRecorder{},
		Budget:      Budget{MaxIterations: 10},
		InitFunc: func(reg *Registry) error {
			reg.Register(ToolSpec{Name: "suspend", Visibility: Internal}, &fakeBuilder{name: "suspend", signal: AwaitApproval})
			return nil
		},
	}

	rr, err := Loop(params, context.Background())

	require.NoError(t, err)
	require.Equal(t, StatusSuspended, rr.Status)
	require.Equal(t, State("AwaitingApproval"), rr.FinalState)
}

func TestLoop_DeclarativeInit_MissingTool(t *testing.T) {
	t.Parallel()

	machineYAML := `
name: test
initial_state: S
states: [S, F]
terminal_states: [F]
signals: [Seed]
transitions:
  - state: S
    signal: Seed
    next: F
    action: nonexistent
`
	dir := t.TempDir()
	machineFile := dir + "/machine.yaml"
	if err := os.WriteFile(machineFile, []byte(machineYAML), 0644); err != nil {
		t.Fatal(err)
	}

	params := LoopParams{
		Prompt:      "test",
		MachineFile: machineFile,
		Trace:       &loopRecorder{},
		Budget:      Budget{MaxIterations: 10},
	}

	_, err := Loop(params, context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "nonexistent")
}

func TestLoop_DeclarativeInit_UsesPreloadedMachineSpec(t *testing.T) {
	t.Parallel()

	spec := MachineSpec{
		Name:           "test",
		InitialState:   "S",
		States:         StateSpecsFromNames("S", "F"),
		TerminalStates: []string{"F"},
		Signals:        SignalSpecsFromNames("Seed", "Done"),
		Transitions: []TransitionSpec{
			{State: "S", Signal: "Seed", Next: "F", Action: "step"},
		},
	}
	params := LoopParams{
		Prompt:      "test",
		MachineFile: "/definitely/not/read/machine.yaml",
		MachineSpec: &spec,
		Trace:       &loopRecorder{},
		Budget:      Budget{MaxIterations: 10},
		InitFunc: func(reg *Registry) error {
			reg.Register(ToolSpec{Name: "step", Visibility: Internal}, &fakeBuilder{name: "step", signal: Signal("Done")})
			return nil
		},
		Hooks: LoopHooks{
			TerminalStatus: func(s State) RunStatus {
				if s == "F" {
					return StatusSucceeded
				}
				return StatusFailed
			},
		},
	}

	rr, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, State("F"), rr.FinalState)
	require.Equal(t, StatusSucceeded, rr.Status)
}

func TestLoop_DeclarativeInit_MachineNameDoesNotChangeEngineBehavior(t *testing.T) {
	t.Parallel()

	run := func(t *testing.T, machineName string) RunResult {
		t.Helper()
		spec := MachineSpec{
			Name:           machineName,
			InitialState:   "Start",
			States:         StateSpecsFromNames("Start", "Working", "Finished"),
			TerminalStates: []string{"Finished"},
			Signals:        SignalSpecsFromNames("Seed", "Done", "TaskCompleted"),
			Transitions: []TransitionSpec{
				{State: "Start", Signal: "Seed", Next: "Working", Action: "step_a"},
				{State: "Working", Signal: "Done", Next: "Working", Action: "step_b"},
				{State: "Working", Signal: "TaskCompleted", Next: "Finished"},
			},
		}
		params := LoopParams{
			MachineSpec: &spec,
			Trace:       &loopRecorder{},
			Budget:      Budget{MaxIterations: 10},
			InitFunc: func(reg *Registry) error {
				reg.Register(ToolSpec{Name: "step_a", Visibility: Internal}, &fakeBuilder{name: "step_a", signal: Signal("Done")})
				reg.Register(ToolSpec{Name: "step_b", Visibility: Internal}, &fakeBuilder{name: "step_b", signal: Signal("TaskCompleted")})
				return nil
			},
			Hooks: LoopHooks{
				TaskCompletedSignal: Signal("TaskCompleted"),
				TerminalStatus: func(s State) RunStatus {
					if s == "Finished" {
						return StatusSucceeded
					}
					return StatusFailed
				},
			},
		}
		rr, err := Loop(params, context.Background())
		require.NoError(t, err)
		return rr
	}

	generatorRun := run(t, "executor")
	evaluatorRun := run(t, "critic")

	require.Equal(t, generatorRun.Status, evaluatorRun.Status)
	require.Equal(t, generatorRun.FinalState, evaluatorRun.FinalState)
	require.Equal(t, generatorRun.Iterations, evaluatorRun.Iterations)
	require.Equal(t, len(generatorRun.Events), len(evaluatorRun.Events))
	for i := range generatorRun.Events {
		require.Equal(t, generatorRun.Events[i].CommandName, evaluatorRun.Events[i].CommandName)
		require.Equal(t, generatorRun.Events[i].FromState, evaluatorRun.Events[i].FromState)
		require.Equal(t, generatorRun.Events[i].ToState, evaluatorRun.Events[i].ToState)
		require.Equal(t, generatorRun.Events[i].Signal, evaluatorRun.Events[i].Signal)
	}
}

func TestLoop_DeclarativeInit_InitFuncError(t *testing.T) {
	t.Parallel()

	machineYAML := `
name: test
initial_state: S
states: [S, F]
terminal_states: [F]
signals: [Seed]
transitions:
  - state: S
    signal: Seed
    next: F
`
	dir := t.TempDir()
	machineFile := dir + "/machine.yaml"
	if err := os.WriteFile(machineFile, []byte(machineYAML), 0644); err != nil {
		t.Fatal(err)
	}

	params := LoopParams{
		Prompt:      "test",
		MachineFile: machineFile,
		Trace:       &loopRecorder{},
		Budget:      Budget{MaxIterations: 10},
		InitFunc: func(reg *Registry) error {
			return fmt.Errorf("init failed: bad config")
		},
	}

	_, err := Loop(params, context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "init failed: bad config")
}
