// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"testing"
)

func TestDiagnoseMachineSpecCleanMachine(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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

func TestDiagnoseMachineSpecUsesRichNames(t *testing.T) {
	t.Parallel()

	yaml := `
name: diagnostics-rich
initial_state: Start
states:
  - name: Start
    meaning: Entry point.
  - name: Done
    meaning: Terminal state.
  - name: Orphan
    meaning: Unreachable branch.
terminal_states: [Done]
signals:
  - name: Seed
    trigger: Engine bootstrap.
  - name: Unused
    trigger: Never emitted.
transitions:
  - state: Start
    signal: Seed
    next: Done
`
	spec, err := ParseMachineSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	diagnostics := DiagnoseMachineSpec(spec)

	assertMachineDiagnostic(t, diagnostics, "unreachable_state", "Orphan", "", 0)
	assertMachineDiagnostic(t, diagnostics, "unused_signal", "", "Unused", 0)
}
