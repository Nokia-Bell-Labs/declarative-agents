// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"strings"
	"testing"
)

const validYAML = `
name: test-machine
initial_state: Idle
states: [Idle, Running, Done, Error]
terminal_states: [Done, Error]
signals: [Start, Finished, Failed]
transitions:
  - state: Idle
    signal: Start
    next: Running
    action: do_work
  - state: Running
    signal: Finished
    next: Done
  - state: Running
    signal: Failed
    next: Error
`

func TestParseMachineSpec_Valid(t *testing.T) {
	spec, err := ParseMachineSpec([]byte(validYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Name != "test-machine" {
		t.Errorf("name = %q, want %q", spec.Name, "test-machine")
	}
	if spec.InitialState != "Idle" {
		t.Errorf("initial_state = %q, want %q", spec.InitialState, "Idle")
	}
	if len(spec.States) != 4 {
		t.Errorf("states count = %d, want 4", len(spec.States))
	}
	if len(spec.TerminalStates) != 2 {
		t.Errorf("terminal_states count = %d, want 2", len(spec.TerminalStates))
	}
	if len(spec.Transitions) != 3 {
		t.Errorf("transitions count = %d, want 3", len(spec.Transitions))
	}
}

func TestParseMachineSpec_MissingInitialState(t *testing.T) {
	yaml := `
name: bad
states: [A]
terminal_states: [A]
signals: [S]
transitions:
  - state: A
    signal: S
    next: A
`
	_, err := ParseMachineSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing initial_state")
	}
	if !strings.Contains(err.Error(), "initial_state is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseMachineSpec_UnknownStateInTransition(t *testing.T) {
	yaml := `
name: bad
initial_state: A
states: [A, B]
terminal_states: [B]
signals: [Go]
transitions:
  - state: C
    signal: Go
    next: B
`
	_, err := ParseMachineSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for unknown state in transition")
	}
	if !strings.Contains(err.Error(), `state "C" not in states list`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseMachineSpec_UnknownSignalInTransition(t *testing.T) {
	yaml := `
name: bad
initial_state: A
states: [A, B]
terminal_states: [B]
signals: [Go]
transitions:
  - state: A
    signal: Unknown
    next: B
`
	_, err := ParseMachineSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for unknown signal in transition")
	}
	if !strings.Contains(err.Error(), `signal "Unknown" not in signals list`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseMachineSpec_TerminalNotInStates(t *testing.T) {
	yaml := `
name: bad
initial_state: A
states: [A]
terminal_states: [Z]
signals: [Go]
transitions:
  - state: A
    signal: Go
    next: A
`
	_, err := ParseMachineSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for terminal state not in states list")
	}
	if !strings.Contains(err.Error(), `terminal_state "Z" not in states list`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDiagnoseMachineSpecCleanMachine(t *testing.T) {
	spec, err := ParseMachineSpec([]byte(validYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	diagnostics := DiagnoseMachineSpec(spec)

	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}
}

func TestDiagnoseMachineSpecReportsReachabilityAndDeadGrammar(t *testing.T) {
	yaml := `
name: diagnostics
initial_state: Start
states: [Start, Working, Done, Orphan, TerminalWithTransition]
terminal_states: [Done, TerminalWithTransition]
signals: [Seed, ToolDone, Unused]
transitions:
  - state: Start
    signal: Seed
    next: Working
    action: step
  - state: Working
    signal: ToolDone
    next: Done
  - state: Orphan
    signal: ToolDone
    next: Done
  - state: TerminalWithTransition
    signal: ToolDone
    next: Done
`
	spec, err := ParseMachineSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	diagnostics := DiagnoseMachineSpec(spec)

	assertMachineDiagnostic(t, diagnostics, "unreachable_state", "Orphan", "", 0)
	assertMachineDiagnostic(t, diagnostics, "unreachable_transition", "Orphan", "ToolDone", 2)
	assertMachineDiagnostic(t, diagnostics, "terminal_transition", "TerminalWithTransition", "ToolDone", 3)
	assertMachineDiagnostic(t, diagnostics, "unused_signal", "", "Unused", 0)
}

type dummyCmd struct{ name string }

func (d *dummyCmd) Name() string    { return d.name }
func (d *dummyCmd) Execute() Result { return Result{} }
func (d *dummyCmd) Undo() Result    { return NoopUndo(d.name) }

type dummyBuilder struct{ name string }

func (d *dummyBuilder) Build(_ Result) Command { return &dummyCmd{name: d.name} }

func TestBuildTransitionTable(t *testing.T) {
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

func TestBuildTransitionTable_ToolActionNil(t *testing.T) {
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

func TestLoadMachineSpec_FileNotFound(t *testing.T) {
	_, err := LoadMachineSpec("/nonexistent/path/machine.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// TestToolComposition_ViaStateMachine demonstrates the pattern of composing
// atomic tools via machine.yaml transitions. Three tools (stage, commit,
// tag) are chained through state transitions, each emitting ToolDone to
// advance to the next step. The state machine is the composition layer.
func TestToolComposition_ViaStateMachine(t *testing.T) {
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

type orderTracker struct {
	toolName string
	order    *[]string
}

func (o *orderTracker) Build(_ Result) Command {
	return &orderCmd{toolName: o.toolName, order: o.order}
}

type orderCmd struct {
	toolName string
	order    *[]string
}

func (o *orderCmd) Name() string { return o.toolName }
func (o *orderCmd) Execute() Result {
	*o.order = append(*o.order, o.toolName)
	return Result{Signal: ToolDone, CommandName: o.toolName, Output: o.toolName + " done"}
}
func (o *orderCmd) Undo() Result { return NoopUndo(o.toolName) }

func assertMachineDiagnostic(t *testing.T, diagnostics []MachineDiagnostic, code, state, signal string, transitionIndex int) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code != code {
			continue
		}
		if state != "" && diagnostic.State != state {
			continue
		}
		if signal != "" && diagnostic.Signal != signal {
			continue
		}
		if transitionIndex != 0 && diagnostic.TransitionIndex != transitionIndex {
			continue
		}
		if diagnostic.Severity != MachineDiagnosticWarning {
			t.Fatalf("diagnostic severity = %q, want %q", diagnostic.Severity, MachineDiagnosticWarning)
		}
		if diagnostic.Message == "" {
			t.Fatal("diagnostic message should not be empty")
		}
		return
	}
	t.Fatalf("missing diagnostic code=%s state=%s signal=%s transition=%d in %#v", code, state, signal, transitionIndex, diagnostics)
}
