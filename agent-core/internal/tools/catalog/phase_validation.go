// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// ValidateToolPhases rejects unknown explicit phases, external words that
// derive no dynamic phase, and selector states that disagree with $tool targets.
func ValidateToolPhases(machine core.MachineSpec, defs []ToolDef) error {
	failures := validateDeclaredPhases(machine, defs)
	failures = append(failures, validateDynamicPhaseDerivation(machine, defs)...)
	failures = append(failures, validateDynamicManifestStates(machine, defs)...)
	if len(failures) > 0 {
		return fmt.Errorf("tool phase validation: %s", strings.Join(failures, "; "))
	}
	return nil
}

func validateDeclaredPhases(machine core.MachineSpec, defs []ToolDef) []string {
	states := make(map[string]bool, len(machine.States))
	for _, state := range machine.States {
		states[state.Name] = true
	}
	var failures []string
	for _, def := range defs {
		for _, phase := range def.Phases {
			if !states[phase] {
				failures = append(failures, fmt.Sprintf(
					"tool %q declares unknown phase %q", def.Name, phase,
				))
			}
		}
	}
	return failures
}

func validateDynamicPhaseDerivation(machine core.MachineSpec, defs []ToolDef) []string {
	phases, scoped := DeriveDynamicToolPhases(machine, defs)
	if !scoped {
		return nil
	}
	context := newDynamicPhaseContext(machine)
	var failures []string
	for _, def := range defs {
		if !dynamicDispatchVisible(def) || len(phases[def.Name]) > 0 {
			continue
		}
		var causes []string
		for _, transition := range context.transitions {
			causes = append(causes, dynamicPhaseCause(def, transition, context))
		}
		failures = append(failures, fmt.Sprintf(
			"tool %q derives no dynamic phase: %s", def.Name, strings.Join(causes, ", ")))
	}
	return failures
}

type dynamicPhaseContext struct {
	transitions   []core.TransitionSpec
	transitionSet map[core.TransitionInput]bool
	terminalSet   map[string]bool
}

func newDynamicPhaseContext(machine core.MachineSpec) dynamicPhaseContext {
	context := dynamicPhaseContext{
		transitionSet: make(map[core.TransitionInput]bool, len(machine.Transitions)),
		terminalSet:   make(map[string]bool, len(machine.TerminalStates)),
	}
	for _, terminal := range machine.TerminalStates {
		context.terminalSet[terminal] = true
	}
	for _, transition := range machine.Transitions {
		context.transitionSet[core.TransitionInput{
			State: core.State(transition.State), Signal: core.Signal(transition.Signal),
		}] = true
		if transition.Action == "$tool" {
			context.transitions = append(context.transitions, transition)
		}
	}
	return context
}

func dynamicPhaseCause(
	def ToolDef, transition core.TransitionSpec, context dynamicPhaseContext,
) string {
	target := core.State(transition.Next)
	if !explicitPhaseAllows(def, target) {
		return fmt.Sprintf("target %q is excluded by explicit phases [%s]",
			transition.Next, strings.Join(def.Phases, ", "))
	}
	if len(def.Emits) == 0 {
		return fmt.Sprintf("target %q cannot route a tool with no declared emitted signals", transition.Next)
	}
	var missing []string
	for _, emit := range def.Emits {
		key := core.TransitionInput{State: target, Signal: core.Signal(emit)}
		if !context.transitionSet[key] && !context.terminalSet[transition.Next] {
			missing = append(missing, emit)
		}
	}
	return fmt.Sprintf("target %q has no transition for emitted signals [%s]",
		transition.Next, strings.Join(missing, ", "))
}

func validateDynamicManifestStates(machine core.MachineSpec, defs []ToolDef) []string {
	context := newDynamicPhaseContext(machine)
	if len(context.transitions) == 0 || !hasToolInit(defs, "parse_response") {
		return nil
	}
	var failures []string
	for _, def := range defs {
		if def.Init != "invoke_llm" {
			continue
		}
		manifestState, ok := def.Config["manifest_state"].(string)
		if !ok || manifestState == "" {
			continue
		}
		for _, transition := range context.transitions {
			if manifestState != transition.Next {
				failures = append(failures, fmt.Sprintf(
					"invoke_llm tool %q manifest_state %q disagrees with $tool target %q after %s/%s",
					def.Name, manifestState, transition.Next, transition.State, transition.Signal))
			}
		}
	}
	return failures
}

func hasToolInit(defs []ToolDef, init string) bool {
	for _, def := range defs {
		if def.Init == init {
			return true
		}
	}
	return false
}
