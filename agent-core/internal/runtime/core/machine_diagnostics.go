// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import "fmt"

type MachineDiagnosticSeverity string

const MachineDiagnosticWarning MachineDiagnosticSeverity = "warning"

const (
	DiagnosticUnreachableState       = "unreachable_state"
	DiagnosticUnreachableTransition  = "unreachable_transition"
	DiagnosticTerminalTransition     = "terminal_transition"
	DiagnosticUnusedSignal           = "unused_signal"
	DiagnosticImplicitSummarySignal  = "implicit_summary_signal"
	DiagnosticImplicitResumeSignal   = "implicit_resume_signal"
	DiagnosticImplicitCommandTimeout = "implicit_command_timeout"
	DiagnosticImplicitMaxIterations  = "implicit_max_iterations"
	DiagnosticMissingTerminalStatus  = "undeclared_terminal_status"
)

func MachineDiagnosticCodes() []string {
	return []string{
		DiagnosticUnreachableState, DiagnosticUnreachableTransition,
		DiagnosticTerminalTransition, DiagnosticUnusedSignal,
		DiagnosticImplicitSummarySignal, DiagnosticImplicitResumeSignal,
		DiagnosticImplicitCommandTimeout, DiagnosticImplicitMaxIterations,
		DiagnosticMissingTerminalStatus,
	}
}

type MachineDiagnostic struct {
	Severity        MachineDiagnosticSeverity
	Code            string
	Message         string
	State           string
	Signal          string
	TransitionIndex int
}

// DiagnoseMachineSpec reports non-fatal policy and dead-grammar diagnostics.
func DiagnoseMachineSpec(spec MachineSpec) []MachineDiagnostic {
	reachable := reachableStates(spec)
	usedSignals := make(map[string]bool)
	terminalSet := make(map[string]bool, len(spec.TerminalStates))
	for _, state := range spec.TerminalStates {
		terminalSet[state] = true
	}
	diagnostics := machinePolicyDiagnostics(spec)
	for _, state := range spec.States.Names() {
		if state != spec.InitialState && !reachable[state] {
			diagnostics = append(diagnostics, MachineDiagnostic{
				Severity: MachineDiagnosticWarning, Code: DiagnosticUnreachableState,
				Message: fmt.Sprintf("state %q is not reachable from initial_state %q", state, spec.InitialState),
				State:   state,
			})
		}
	}
	for i, transition := range spec.Transitions {
		usedSignals[transition.Signal] = true
		markForEachSignals(usedSignals, transition.ForEach)
		diagnostics = append(diagnostics,
			transitionDiagnostics(i, transition, reachable, terminalSet)...)
	}
	for _, signal := range spec.Signals.Names() {
		if !usedSignals[signal] {
			diagnostics = append(diagnostics, MachineDiagnostic{
				Severity: MachineDiagnosticWarning, Code: DiagnosticUnusedSignal,
				Message: fmt.Sprintf("signal %q is declared but no transition uses it", signal),
				Signal:  signal,
			})
		}
	}
	return diagnostics
}

func transitionDiagnostics(
	index int, transition TransitionSpec, reachable, terminals map[string]bool,
) []MachineDiagnostic {
	var diagnostics []MachineDiagnostic
	if !reachable[transition.State] {
		diagnostics = append(diagnostics, MachineDiagnostic{
			Severity: MachineDiagnosticWarning, Code: DiagnosticUnreachableTransition,
			Message: fmt.Sprintf("transition[%d] from %s/%s is unreachable",
				index, transition.State, transition.Signal),
			State: transition.State, Signal: transition.Signal, TransitionIndex: index,
		})
	}
	if terminals[transition.State] {
		diagnostics = append(diagnostics, MachineDiagnostic{
			Severity: MachineDiagnosticWarning, Code: DiagnosticTerminalTransition,
			Message: fmt.Sprintf("transition[%d] starts from terminal state %q", index, transition.State),
			State:   transition.State, Signal: transition.Signal, TransitionIndex: index,
		})
	}
	return diagnostics
}
