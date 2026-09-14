// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/typesys"
)

// Load-time resolution of $from(label) selectors against the labels a machine
// declares (GH-1966). The selector grammar was validated at roughly fifty
// scattered sites and the label universe was derivable from the machine, but
// nothing joined them: a typo'd label surfaced as a runtime
// core.UnresolvedLabelError instead of a load failure.

// MachineLabels returns every command-state address a machine publishes:
// transition labels, for_each join and item labels, the labels the runtime
// seeds through external_labels, and the executed command name of every named
// action. Command names belong here because a step stays addressable by the
// command that ran it whether or not the transition also carries a label
// (srd006 R1.6, srd038 R2.2 and R2.7).
func MachineLabels(spec core.MachineSpec) map[string]struct{} {
	labels := make(map[string]struct{})
	add := func(label string) {
		if label != "" {
			labels[label] = struct{}{}
		}
	}
	for _, label := range spec.ExternalLabels {
		add(label.Name)
	}
	for _, transition := range spec.Transitions {
		add(transition.Label)
		if transition.Action != DynamicActionSentinel {
			add(transition.Action)
		}
		if transition.ForEach == nil {
			continue
		}
		add(transition.ForEach.As)
		add(transition.ForEach.Join.Label)
	}
	return labels
}

// DynamicActionSentinel is the transition action that defers tool choice to
// runtime dispatch.
const DynamicActionSentinel = "$tool"

// SelectorRefs returns every $from(label).path selector a tool definition
// carries: the exec stdin_source and parameter sources, plus every selector
// anywhere in the config block.
//
// The config walk is structural rather than type-directed. Selector-bearing
// config shapes live in two places — the typed structs in this package and the
// owning tool packages (lifecycle, otlp, service), which import catalog and so
// cannot be imported back. Walking the decoded config reaches both, and a
// selector field added anywhere is covered without editing this function.
// Resolving a label does not depend on which field holds the selector, so
// shape is enough here; checking a selector's path against a declared output
// type is not, and belongs with the tool signatures of GH-1968.
func (td ToolDef) SelectorRefs() []string {
	refs := map[string]struct{}{}
	collectSelector(refs, td.StdinSource)
	collectSelectorsFrom(refs, td.Config)
	out := make([]string, 0, len(refs))
	for ref := range refs {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

func collectSelector(refs map[string]struct{}, value string) {
	if _, _, ok := core.ParseFromSelector(value); ok {
		refs[value] = struct{}{}
	}
}

// collectSelectorsFrom walks decoded YAML of any shape: maps, sequences, and
// scalars. Only strings the selector grammar accepts as $from(label).path are
// collected, so ordinary config text cannot be mistaken for a reference.
func collectSelectorsFrom(refs map[string]struct{}, value interface{}) {
	switch typed := value.(type) {
	case string:
		collectSelector(refs, typed)
	case []interface{}:
		for _, item := range typed {
			collectSelectorsFrom(refs, item)
		}
	case map[string]interface{}:
		for _, item := range typed {
			collectSelectorsFrom(refs, item)
		}
	case map[interface{}]interface{}:
		for _, item := range typed {
			collectSelectorsFrom(refs, item)
		}
	}
}

// ValidateSelectorLabels reports every selector, in the machine and in the
// configs of tools the machine can dispatch, whose label no transition
// publishes and no external_labels entry declares.
func ValidateSelectorLabels(spec core.MachineSpec, defs []ToolDef) []core.MachineDiagnostic {
	labels := MachineLabels(spec)
	addDynamicDispatchNames(labels, spec, defs)
	seen := make(map[string]struct{})
	var diagnostics []core.MachineDiagnostic
	for i, transition := range spec.Transitions {
		if transition.ForEach == nil {
			continue
		}
		if _, repeated := seen[transition.ForEach.Items]; repeated {
			continue
		}
		seen[transition.ForEach.Items] = struct{}{}
		diagnostics = append(diagnostics,
			selectorDiagnostics(labels, transition.ForEach.Items, "", transition.State, i)...)
	}
	for _, def := range reachableToolDefs(spec, defs) {
		for _, ref := range def.SelectorRefs() {
			diagnostics = append(diagnostics,
				selectorDiagnostics(labels, ref, def.Name, "", -1)...)
		}
	}
	return diagnostics
}

// reachableToolDefs narrows the definitions to those a transition names as an
// action, so an unselected tool's config cannot fail a machine that never
// dispatches it.
// addDynamicDispatchNames widens the universe when a machine defers tool choice
// to runtime: a $tool transition can execute any selected tool, so each of
// those command names becomes addressable (srd038 R2.8).
func addDynamicDispatchNames(labels map[string]struct{}, spec core.MachineSpec, defs []ToolDef) {
	dynamic := false
	for _, transition := range spec.Transitions {
		if transition.Action == DynamicActionSentinel {
			dynamic = true
			break
		}
	}
	if !dynamic {
		return
	}
	for _, def := range defs {
		if def.Name != "" {
			labels[def.Name] = struct{}{}
		}
	}
}

func reachableToolDefs(spec core.MachineSpec, defs []ToolDef) []ToolDef {
	actions := make(map[string]bool, len(spec.Transitions))
	for _, transition := range spec.Transitions {
		if transition.Action != "" && transition.Action != "$tool" {
			actions[transition.Action] = true
		}
	}
	reachable := make([]ToolDef, 0, len(defs))
	for _, def := range defs {
		if actions[def.Name] {
			reachable = append(reachable, def)
		}
	}
	return reachable
}

func selectorDiagnostics(
	labels map[string]struct{},
	selector, tool, state string,
	transitionIndex int,
) []core.MachineDiagnostic {
	label, _, ok := core.ParseFromSelector(selector)
	if !ok {
		return nil
	}
	if _, declared := labels[label]; declared {
		return nil
	}
	return []core.MachineDiagnostic{{
		Severity:        core.MachineDiagnosticWarning,
		Code:            core.DiagnosticUnresolvedSelectorLabel,
		Message:         unresolvedLabelMessage(selector, label, tool, labels),
		State:           state,
		Tool:            tool,
		TransitionIndex: transitionIndex,
	}}
}

func unresolvedLabelMessage(selector, label, tool string, labels map[string]struct{}) string {
	where := "machine"
	if tool != "" {
		where = fmt.Sprintf("tool %q", tool)
	}
	message := fmt.Sprintf("%s selector %q names label %q, which no transition publishes",
		where, selector, label)
	if nearest := nearestLabel(label, labels); nearest != "" {
		return message + fmt.Sprintf("; closest declared label is %q", nearest)
	}
	return message
}

// nearestLabel returns the declared label closest to an unknown one, so the
// diagnostic can name a likely typo. It returns empty when nothing is close
// enough to be worth suggesting.
func nearestLabel(label string, labels map[string]struct{}) string {
	best, bestDistance := "", editDistanceLimit(label)+1
	candidates := make([]string, 0, len(labels))
	for candidate := range labels {
		candidates = append(candidates, candidate)
	}
	sort.Strings(candidates)
	for _, candidate := range candidates {
		if distance := typesys.EditDistance(label, candidate); distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	return best
}

// editDistanceLimit keeps suggestions honest: a third of the name, so a short
// label suggests only a near-miss and an unrelated label suggests nothing.
func editDistanceLimit(label string) int {
	limit := len([]rune(label)) / 3
	if limit < 1 {
		return 1
	}
	return limit
}

// ValidateSelectorLabelsStrict is the load-time gate: it turns the selector
// diagnostics into one error naming every unresolved reference. Promoted from
// a warning once the in-repo declarations were clean, which they were on the
// check's first run across all 53 boot-smoke profiles (GH-1966).
func ValidateSelectorLabelsStrict(spec core.MachineSpec, defs []ToolDef) error {
	diagnostics := ValidateSelectorLabels(spec, defs)
	if len(diagnostics) == 0 {
		return nil
	}
	messages := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		messages = append(messages, diagnostic.Message)
	}
	return fmt.Errorf("unresolved selector labels: %s", strings.Join(messages, "; "))
}
