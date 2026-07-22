// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"strings"
	"testing"
)

func TestBuildTransitionTable(t *testing.T) {
	t.Parallel()

	spec, err := ParseMachineSpec([]byte(validYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	reg := NewRegistry()
	reg.Register(ToolSpec{Name: "do_work", Phases: []State{"Running"}}, &dummyBuilder{name: "do_work"})

	table, isTerminal, err := BuildTransitionTable(spec, reg, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if len(table) != 3 {
		t.Errorf("table size = %d, want 3", len(table))
	}

	if !isTerminal("Done") {
		t.Error("Done should be terminal")
	}
	if !isTerminal("Error") {
		t.Error("Error should be terminal")
	}
	if isTerminal("Idle") {
		t.Error("Idle should not be terminal")
	}

	key := TransitionInput{State: "Idle", Signal: "Start"}
	tv, ok := table[key]
	if !ok {
		t.Fatal("missing transition for Idle+Start")
	}
	if tv.NextState != "Running" {
		t.Errorf("next = %q, want Running", tv.NextState)
	}
	if tv.Action == nil {
		t.Error("action should not be nil for do_work transition")
	}

	key2 := TransitionInput{State: "Running", Signal: "Finished"}
	tv2, ok := table[key2]
	if !ok {
		t.Fatal("missing transition for Running+Finished")
	}
	if tv2.Action != nil {
		t.Error("terminal transition action should be nil")
	}
}

func TestBuildTransitionTable_ToolActionSentinel(t *testing.T) {
	t.Parallel()

	yaml := `
name: tool-test
initial_state: A
states: [A, B]
terminal_states: [B]
signals: [Go]
transitions:
  - state: A
    signal: Go
    next: B
    action: $tool
`
	spec, err := ParseMachineSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	called := false
	toolAction := func(r Result) Command {
		called = true
		return &dummyCmd{}
	}

	reg := NewRegistry()
	table, _, err := BuildTransitionTable(spec, reg, toolAction)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	key := TransitionInput{State: "A", Signal: "Go"}
	tv := table[key]
	tv.Action(Result{})
	if !called {
		t.Error("$tool action should have invoked the toolAction function")
	}
}

func TestBuildTransitionTablePassesTargetStateToAction(t *testing.T) {
	t.Parallel()

	yaml := `
name: state-context
initial_state: A
states: [A, B]
terminal_states: [B]
signals: [Go]
transitions:
  - state: A
    signal: Go
    next: B
    action: $tool
`
	spec, err := ParseMachineSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var got State
	toolAction := func(r Result) Command {
		got = r.State
		return &dummyCmd{}
	}

	reg := NewRegistry()
	table, _, err := BuildTransitionTable(spec, reg, toolAction)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	table[TransitionInput{State: "A", Signal: "Go"}].Action(Result{})
	if got != "B" {
		t.Fatalf("action state = %q, want B", got)
	}
}

func TestBuildTransitionTable_ToolActionNil(t *testing.T) {
	t.Parallel()

	yaml := `
name: tool-test
initial_state: A
states: [A, B]
terminal_states: [B]
signals: [Go]
transitions:
  - state: A
    signal: Go
    next: B
    action: $tool
`
	spec, err := ParseMachineSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	reg := NewRegistry()
	_, _, err = BuildTransitionTable(spec, reg, nil)
	if err == nil {
		t.Fatal("expected error when $tool action used without toolAction function")
	}
}

func TestBuildTransitionTable_UnknownAction(t *testing.T) {
	t.Parallel()

	yaml := `
name: bad-action
initial_state: A
states: [A, B]
terminal_states: [B]
signals: [Go]
transitions:
  - state: A
    signal: Go
    next: B
    action: nonexistent
`
	spec, err := ParseMachineSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	reg := NewRegistry()
	_, _, err = BuildTransitionTable(spec, reg, nil)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
	if !strings.Contains(err.Error(), `"nonexistent" not found in registry`) {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestToolComposition_ViaStateMachine demonstrates the pattern of composing
// atomic tools via machine.yaml transitions. Three tools (stage, commit,
// tag) are chained through state transitions, each emitting ToolDone to
// advance to the next step. The state machine is the composition layer.
func TestToolComposition_ViaStateMachine(t *testing.T) {
	t.Parallel()

	machineYAML := `
name: commit-workflow
initial_state: Idle
states: [Idle, Staging, Committing, Tagging, Done, Failed]
terminal_states: [Done, Failed]
signals: [Seed, ToolDone, ToolFailed, CommandError]
transitions:
  - state: Idle
    signal: Seed
    next: Staging
    action: stage_all
  - state: Staging
    signal: ToolDone
    next: Committing
    action: commit
  - state: Staging
    signal: ToolFailed
    next: Failed
  - state: Committing
    signal: ToolDone
    next: Tagging
    action: tag
  - state: Committing
    signal: ToolFailed
    next: Failed
  - state: Tagging
    signal: ToolDone
    next: Done
  - state: Tagging
    signal: ToolFailed
    next: Failed
`
	spec, err := ParseMachineSpec([]byte(machineYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var executionOrder []string
	makeBuilder := func(name string) Builder {
		return &orderTracker{toolName: name, order: &executionOrder}
	}

	reg := NewRegistry()
	reg.Register(ToolSpec{Name: "stage_all"}, makeBuilder("stage_all"))
	reg.Register(ToolSpec{Name: "commit"}, makeBuilder("commit"))
	reg.Register(ToolSpec{Name: "tag"}, makeBuilder("tag"))

	table, isTerminal, err := BuildTransitionTable(spec, reg, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	sm := NewStateMachine(table, isTerminal)
	state := State("Idle")
	result := Result{Signal: Seed}

	for !sm.IsTerminal(state) {
		nextState, cmd, err := sm.Step(state, result.Signal, result)
		if err != nil {
			t.Fatalf("step from %s with %s: %v", state, result.Signal, err)
		}
		state = nextState
		if cmd != nil {
			result = cmd.Execute()
		}
	}

	if state != "Done" {
		t.Errorf("final state = %q, want Done", state)
	}

	expected := []string{"stage_all", "commit", "tag"}
	if len(executionOrder) != len(expected) {
		t.Fatalf("execution order length = %d, want %d: %v", len(executionOrder), len(expected), executionOrder)
	}
	for i, name := range expected {
		if executionOrder[i] != name {
			t.Errorf("execution order[%d] = %q, want %q", i, executionOrder[i], name)
		}
	}
}
