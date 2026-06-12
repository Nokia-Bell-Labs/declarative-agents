// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// ValidateToolEmits checks declared tool output signals against a machine.
//
// The contract is intentionally incremental: tools may omit emits while the
// declarations are being migrated. When an action tool declares emits, every
// emitted signal must be declared by the machine, and named transition actions
// must have a follow-up transition from the action's target state for every
// emitted signal.
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
		transitionSet[core.TransitionInput{
			State:  core.State(tr.State),
			Signal: core.Signal(tr.Signal),
		}] = true
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

	for action, transitions := range actions {
		def, ok := defsByName[action]
		if !ok || len(def.Emits) == 0 {
			continue
		}
		for _, tr := range transitions {
			if terminalSet[tr.Next] {
				continue
			}
			for _, emit := range def.Emits {
				key := core.TransitionInput{
					State:  core.State(tr.Next),
					Signal: core.Signal(emit),
				}
				if !transitionSet[key] {
					errs = append(errs, fmt.Sprintf("tool %q emits %q after %s/%s -> %s, but machine %q has no transition for %s/%s",
						def.Name, emit, tr.State, tr.Signal, tr.Next, spec.Name, tr.Next, emit))
				}
			}
		}
	}
	for _, tr := range dynamicTransitions {
		if terminalSet[tr.Next] {
			continue
		}
		for _, def := range defs {
			if def.Visibility == "internal" || len(def.Emits) == 0 {
				continue
			}
			for _, emit := range def.Emits {
				key := core.TransitionInput{
					State:  core.State(tr.Next),
					Signal: core.Signal(emit),
				}
				if !transitionSet[key] {
					errs = append(errs, fmt.Sprintf("dynamic $tool may dispatch tool %q which emits %q after %s/%s -> %s, but machine %q has no transition for %s/%s",
						def.Name, emit, tr.State, tr.Signal, tr.Next, spec.Name, tr.Next, emit))
				}
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("tool emits validation: %s", strings.Join(errs, "; "))
	}
	return nil
}
