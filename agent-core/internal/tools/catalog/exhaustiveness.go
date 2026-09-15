// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"fmt"
	"sort"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// Transition liveness over tool signatures (srd006): a transition waiting on a
// signal nothing entering its state can produce never fires.
//
// The converse direction, that every signal an action emits is handled where it
// lands, is already enforced by ValidateToolEmits, which covers named actions,
// dynamic dispatch, terminal states, and a for_each word's continue_on and
// abort_on lists. This file does not repeat it.
//
// Liveness is decidable because a signature makes a tool's emitted set
// authoritative. A tool with no declared emits contributes nothing, so an
// unconverted machine stays quiet.

// ExhaustivenessInputs carries the signals that reach a machine from outside
// its own transitions. The caller assembles them because their sources differ:
// the runtime seeds one, the machine declares two, and request signal sources
// live in the REST collection, which this package cannot see.
type ExhaustivenessInputs struct {
	// External names signals injected from outside the transition table, such
	// as a request signal source's mapped values.
	External []string
}

// ValidateMachineExhaustiveness reports transitions waiting on signals nothing
// entering their state can emit.
func ValidateMachineExhaustiveness(
	spec core.MachineSpec, defs []ToolDef, inputs ExhaustivenessInputs,
) []core.MachineDiagnostic {
	byName := make(map[string]ToolDef, len(defs))
	for _, def := range defs {
		byName[def.Name] = def
	}
	return deadTransitionDiagnostics(spec, byName, inputs)
}

// deadTransitionDiagnostics reports a transition waiting on a signal nothing
// entering its state can produce, so the row can never fire.
func deadTransitionDiagnostics(
	spec core.MachineSpec, byName map[string]ToolDef, inputs ExhaustivenessInputs,
) []core.MachineDiagnostic {
	emittable := emittableSignalsByState(spec, byName, inputs)
	resumePoints := resumePointStates(spec, byName)
	var diagnostics []core.MachineDiagnostic
	for i, transition := range spec.Transitions {
		if emittable[transition.State][transition.Signal] || resumePoints[transition.State] {
			continue
		}
		diagnostics = append(diagnostics, core.MachineDiagnostic{
			Severity: core.MachineDiagnosticWarning,
			Code:     core.DiagnosticDeadTransition,
			Message: fmt.Sprintf(
				"transition %s --%s--> %s waits on a signal nothing entering %q emits",
				transition.State, transition.Signal, transition.Next, transition.State),
			State: transition.State, Signal: transition.Signal, TransitionIndex: i,
		})
	}
	return diagnostics
}

// resumePointStates finds the states a run can be resumed into. An action
// declaring a human_boundary side effect returns control outside the run, and
// whoever resumes supplies the signal: an approval gate routes both Approved
// and Rejected while the machine can declare only one resume_signal. Liveness
// is not decidable at such a state, so it is exempt.
//
// The marker is the declared side effect rather than the word's name, so a new
// word that returns control is covered without being listed here.
func resumePointStates(spec core.MachineSpec, byName map[string]ToolDef) map[string]bool {
	resumePoints := map[string]bool{}
	for _, transition := range spec.Transitions {
		action, ok := byName[transition.Action]
		if !ok || !returnsControlOutsideTheRun(action) {
			continue
		}
		resumePoints[transition.Next] = true
	}
	return resumePoints
}

func returnsControlOutsideTheRun(def ToolDef) bool {
	for _, effect := range def.SideEffects.Items {
		if effect.Kind == "human_boundary" {
			return true
		}
	}
	return false
}

// emittableSignalsByState collects, per state, every signal that can arrive
// there: what the actions entering it emit, what a for_each join produces, and
// what reaches the machine from outside.
func emittableSignalsByState(
	spec core.MachineSpec, byName map[string]ToolDef, inputs ExhaustivenessInputs,
) map[string]map[string]bool {
	emittable := map[string]map[string]bool{}
	mark := func(state, signal string) {
		if state == "" || signal == "" {
			return
		}
		if emittable[state] == nil {
			emittable[state] = map[string]bool{}
		}
		emittable[state][signal] = true
	}
	// The loop engine produces Seed, BudgetExhausted, and CommandError itself,
	// whatever a word declares, so all three can arrive at any state. A machine
	// may also declare its own resume and summary signals, and a request signal
	// source injects the rest.
	for _, signal := range append([]string{
		string(core.Seed), string(core.BudgetExhausted), string(core.CommandError),
		spec.ResumeSignal, spec.SummarySignal,
	}, inputs.External...) {
		for _, state := range spec.States.Names() {
			mark(state, signal)
		}
	}
	for _, transition := range spec.Transitions {
		for _, signal := range emittedSignals(transition, byName) {
			mark(transition.Next, signal)
		}
		markJoinSignals(mark, transition)
	}
	return emittable
}

func markJoinSignals(mark func(state, signal string), transition core.TransitionSpec) {
	if transition.ForEach == nil {
		return
	}
	join := transition.ForEach.Join
	for _, signal := range []string{
		join.Signals.AllSuccess, join.Signals.Partial,
		join.Signals.Failed, join.Signals.Empty,
	} {
		mark(join.Next, signal)
	}
}

// emittedSignals returns what one transition's action can emit. A $tool
// transition dispatches dynamically, so its emitted set is the union over every
// tool the state's manifest can select.
func emittedSignals(transition core.TransitionSpec, byName map[string]ToolDef) []string {
	if transition.Action == DynamicActionSentinel {
		return dynamicEmittedSignals(transition, byName)
	}
	action, ok := byName[transition.Action]
	if !ok {
		return nil
	}
	return action.Emits
}

func dynamicEmittedSignals(
	transition core.TransitionSpec, byName map[string]ToolDef,
) []string {
	union := map[string]bool{}
	for _, def := range byName {
		if !dynamicDispatchVisible(def) || !toolPhaseAllows(def, transition.Next) {
			continue
		}
		for _, signal := range def.Emits {
			union[signal] = true
		}
	}
	signals := make([]string, 0, len(union))
	for signal := range union {
		signals = append(signals, signal)
	}
	sort.Strings(signals)
	return signals
}

// toolPhaseAllows reports whether a tool is in the manifest for a state. A tool
// declaring no phases is available everywhere.
func toolPhaseAllows(def ToolDef, state string) bool {
	if len(def.Phases) == 0 {
		return true
	}
	for _, phase := range def.Phases {
		if phase == state {
			return true
		}
	}
	return false
}

// ValidateMachineExhaustivenessStrict is the load-time gate, reported as one
// error naming every violation. Promoted in GH-2005, once the two conformance
// machines routing a ToolFailed that checkpoint_history and
// checkpoint_rollback cannot emit had those dead routes removed.
func ValidateMachineExhaustivenessStrict(
	spec core.MachineSpec, defs []ToolDef, inputs ExhaustivenessInputs,
) error {
	diagnostics := ValidateMachineExhaustiveness(spec, defs, inputs)
	if len(diagnostics) == 0 {
		return nil
	}
	messages := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		messages = append(messages, diagnostic.Message)
	}
	return fmt.Errorf("machine is not exhaustive: %s", joinMessages(messages))
}
