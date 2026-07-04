// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// ValidateToolEmits checks declared tool output signals against a machine.
func ValidateToolEmits(spec core.MachineSpec, defs []ToolDef) error {
	signalSet := make(map[string]bool, len(spec.Signals))
	for _, sig := range spec.Signals.Names() {
		signalSet[sig] = true
	}
	terminalSet := make(map[string]bool, len(spec.TerminalStates))
	for _, state := range spec.TerminalStates {
		terminalSet[state] = true
	}

	transitionSet := make(map[core.TransitionInput]bool, len(spec.Transitions))
	actions := make(map[string][]core.TransitionSpec)
	actionNames := make(map[string]bool)
	var dynamicTransitions []core.TransitionSpec
	for _, tr := range spec.Transitions {
		transitionSet[core.TransitionInput{State: core.State(tr.State), Signal: core.Signal(tr.Signal)}] = true
		switch {
		case tr.Action == "$tool":
			dynamicTransitions = append(dynamicTransitions, tr)
		case tr.Action != "":
			actions[tr.Action] = append(actions[tr.Action], tr)
			actionNames[tr.Action] = true
		}
	}

	defsByName := make(map[string]ToolDef, len(defs))
	for _, def := range defs {
		defsByName[def.Name] = def
	}

	var errs []string
	for _, def := range defs {
		if !actionNames[def.Name] {
			continue
		}
		for _, emit := range def.Emits {
			if !signalSet[emit] {
				errs = append(errs, fmt.Sprintf("tool %q emits signal %q not declared by machine %q", def.Name, emit, spec.Name))
			}
		}
	}
	errs = append(errs, validateNamedActionEmits(spec, actions, defsByName, transitionSet, terminalSet)...)
	errs = append(errs, validateDynamicEmits(spec, dynamicTransitions, defs, transitionSet, terminalSet)...)
	if len(errs) > 0 {
		return fmt.Errorf("tool emits validation: %s", strings.Join(errs, "; "))
	}
	return nil
}

// ApplyDynamicToolPhases annotates external ToolDefs with manifest phases
// derived from MachineSpec $tool transitions.
func ApplyDynamicToolPhases(spec core.MachineSpec, defs []ToolDef) []ToolDef {
	phases, scoped := DeriveDynamicToolPhases(spec, defs)
	if !scoped {
		return defs
	}
	out := make([]ToolDef, len(defs))
	copy(out, defs)
	for i := range out {
		if !dynamicDispatchVisible(out[i]) {
			continue
		}
		out[i].Phases = stateNames(phases[out[i].Name])
		out[i].phaseScoped = true
	}
	return out
}

// DeriveDynamicToolPhases returns the manifest states where each external tool
// may be shown for LLM-selected dynamic dispatch.
func DeriveDynamicToolPhases(spec core.MachineSpec, defs []ToolDef) (map[string][]core.State, bool) {
	terminalSet := make(map[string]bool, len(spec.TerminalStates))
	for _, state := range spec.TerminalStates {
		terminalSet[state] = true
	}
	transitionSet := make(map[core.TransitionInput]bool, len(spec.Transitions))
	var dynamicTransitions []core.TransitionSpec
	for _, tr := range spec.Transitions {
		transitionSet[core.TransitionInput{State: core.State(tr.State), Signal: core.Signal(tr.Signal)}] = true
		if tr.Action == "$tool" {
			dynamicTransitions = append(dynamicTransitions, tr)
		}
	}
	if len(dynamicTransitions) == 0 {
		return nil, false
	}
	phases := make(map[string][]core.State, len(defs))
	for _, def := range defs {
		if !dynamicDispatchVisible(def) {
			continue
		}
		for _, tr := range dynamicTransitions {
			phase := core.State(tr.Next)
			if !explicitPhaseAllows(def, phase) {
				continue
			}
			if !dynamicToolRoutesFromTarget(def, tr.Next, transitionSet, terminalSet) {
				continue
			}
			phases[def.Name] = appendStateOnce(phases[def.Name], phase)
		}
	}
	for name := range phases {
		sort.Slice(phases[name], func(i, j int) bool { return phases[name][i] < phases[name][j] })
	}
	return phases, true
}

func validateNamedActionEmits(spec core.MachineSpec, actions map[string][]core.TransitionSpec, defsByName map[string]ToolDef, transitionSet map[core.TransitionInput]bool, terminalSet map[string]bool) []string {
	var errs []string
	for action, transitions := range actions {
		def, ok := defsByName[action]
		if !ok || len(def.Emits) == 0 {
			continue
		}
		for _, tr := range transitions {
			errs = append(errs, validateActionTransitionEmits(spec, def, tr, transitionSet, terminalSet)...)
		}
	}
	return errs
}

func validateActionTransitionEmits(spec core.MachineSpec, def ToolDef, tr core.TransitionSpec, transitionSet map[core.TransitionInput]bool, terminalSet map[string]bool) []string {
	if terminalSet[tr.Next] {
		return nil
	}
	var errs []string
	for _, emit := range def.Emits {
		key := core.TransitionInput{State: core.State(tr.Next), Signal: core.Signal(emit)}
		if !transitionSet[key] {
			errs = append(errs, fmt.Sprintf("tool %q emits %q after %s/%s -> %s, but machine %q has no transition for %s/%s",
				def.Name, emit, tr.State, tr.Signal, tr.Next, spec.Name, tr.Next, emit))
		}
	}
	return errs
}

func validateDynamicEmits(spec core.MachineSpec, transitions []core.TransitionSpec, defs []ToolDef, transitionSet map[core.TransitionInput]bool, terminalSet map[string]bool) []string {
	var errs []string
	for _, tr := range transitions {
		if terminalSet[tr.Next] {
			continue
		}
		for _, def := range defs {
			if !dynamicDispatchVisible(def) || len(def.Emits) == 0 {
				continue
			}
			for _, emit := range def.Emits {
				key := core.TransitionInput{State: core.State(tr.Next), Signal: core.Signal(emit)}
				if !transitionSet[key] {
					errs = append(errs, fmt.Sprintf("dynamic $tool may dispatch tool %q which emits %q after %s/%s -> %s, but machine %q has no transition for %s/%s",
						def.Name, emit, tr.State, tr.Signal, tr.Next, spec.Name, tr.Next, emit))
				}
			}
		}
	}
	return errs
}

func dynamicDispatchVisible(def ToolDef) bool {
	return def.ToToolSpec().Visibility == core.External
}

func dynamicToolRoutesFromTarget(def ToolDef, target string, transitionSet map[core.TransitionInput]bool, terminalSet map[string]bool) bool {
	if terminalSet[target] {
		return true
	}
	if len(def.Emits) == 0 {
		return false
	}
	for _, emit := range def.Emits {
		key := core.TransitionInput{State: core.State(target), Signal: core.Signal(emit)}
		if !transitionSet[key] {
			return false
		}
	}
	return true
}

func explicitPhaseAllows(def ToolDef, phase core.State) bool {
	if len(def.Phases) == 0 {
		return true
	}
	for _, allowed := range def.Phases {
		if core.State(allowed) == phase {
			return true
		}
	}
	return false
}

func appendStateOnce(states []core.State, state core.State) []core.State {
	for _, existing := range states {
		if existing == state {
			return states
		}
	}
	return append(states, state)
}

func stateNames(states []core.State) []string {
	if len(states) == 0 {
		return nil
	}
	names := make([]string, 0, len(states))
	for _, state := range states {
		names = append(names, string(state))
	}
	return names
}
