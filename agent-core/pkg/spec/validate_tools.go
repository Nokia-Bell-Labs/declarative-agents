// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func checkToolSelectionDeclared(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range sortedKeys(corpus.ToolSelections) {
		selected := corpus.ToolSelections[agentName]
		for _, toolName := range selected {
			if _, ok := corpus.ToolDeclarations[toolName]; !ok {
				findings = append(findings, Finding{
					Check:   "tool-selection-undeclared",
					Level:   "error",
					Message: fmt.Sprintf("agent %s selects tool %q which has no declaration", agentName, toolName),
				})
			}
		}
	}
	return findings
}

func checkToolDeclarationVocabulary(corpus *Corpus) []Finding {
	var findings []Finding
	for name, declaration := range corpus.ToolDeclarations {
		if message := invalidToolDeclaration(declaration); message != "" {
			findings = append(findings, Finding{
				Check: "tool-declaration-invalid", Level: "error",
				Message: fmt.Sprintf("tool %q: %s", name, message),
			})
		}
		for _, declaredError := range declaration.Errors {
			if declaredError.Signal == "" {
				findings = append(findings, Finding{
					Check: "tool-declaration-invalid", Level: "error",
					Message: fmt.Sprintf("tool %q has an error contract with no signal", name),
				})
			}
		}
		for _, overlap := range declaration.Relationships.Overlaps {
			if overlap.Tool == "" {
				findings = append(findings, Finding{
					Check: "tool-declaration-invalid", Level: "error",
					Message: fmt.Sprintf("tool %q has an overlap with no tool name", name),
				})
			}
		}
	}
	return findings
}

func invalidToolDeclaration(declaration ToolDeclaration) string {
	switch declaration.Type {
	case "", "exec":
		if declaration.Init != "" {
			return fmt.Sprintf("exec declaration has builtin init %q", declaration.Init)
		}
	case "builtin":
		if declaration.Init == "" {
			return "builtin declaration has no init"
		}
	default:
		return fmt.Sprintf("unknown type %q", declaration.Type)
	}
	switch declaration.Visibility {
	case "", "internal", "external":
	default:
		return fmt.Sprintf("unknown visibility %q", declaration.Visibility)
	}
	switch declaration.Reversibility.Classification {
	case "", "reversible", "compensatable", "irreversible":
	default:
		return fmt.Sprintf(
			"unknown reversibility classification %q",
			declaration.Reversibility.Classification,
		)
	}
	return ""
}

// checkSelectedToolContractCompleteness audits selected tool declarations in the
// public spec corpus. Runtime ToolDef contract validation is owned by
// internal/tools/catalog; this package keeps a public mirror for corpus files.

func checkSelectedToolContractCompleteness(corpus *Corpus) []Finding {
	consumers := selectedToolConsumers(corpus)
	var findings []Finding
	for _, toolName := range sortedKeys(consumers) {
		td, ok := corpus.ToolDeclarations[toolName]
		if !ok {
			continue
		}
		missing := missingToolContractFields(td)
		if len(missing) == 0 {
			continue
		}
		level := "error"
		if td.Contract == "legacy" {
			level = "warning"
		}
		findings = append(findings, Finding{
			Check: "tool-contract-incomplete",
			Level: level,
			Message: fmt.Sprintf(
				"selected tool %q from %s used by %s is missing contract fields: %s",
				toolName,
				sourceOrUnknown(td.SourceFile),
				strings.Join(consumers[toolName], ", "),
				strings.Join(missing, ", "),
			),
		})
	}
	return findings
}

func selectedToolConsumers(corpus *Corpus) map[string][]string {
	consumers := make(map[string][]string)
	for _, selectionName := range sortedKeys(corpus.ToolSelections) {
		seenInSelection := make(map[string]bool)
		for _, toolName := range corpus.ToolSelections[selectionName] {
			if toolName == "" || seenInSelection[toolName] {
				continue
			}
			seenInSelection[toolName] = true
			consumers[toolName] = append(consumers[toolName], selectionName)
		}
	}
	return consumers
}

func missingToolContractFields(td ToolDeclaration) []string {
	checks := []struct {
		field   string
		present bool
	}{
		{"category", td.Category != ""},
		{"problem", td.Problem != ""},
		{"goals", len(td.Goals) > 0},
		{"requirements.input", len(td.Requirements.Input) > 0},
		{"requirements.output", len(td.Requirements.Output) > 0},
		{"requirements.errors", len(td.Requirements.Errors) > 0},
		{"non_goals", len(td.NonGoals) > 0},
		{"emits", len(td.Emits) > 0},
		{"output.schema", len(td.Output.Schema) > 0},
		{"side_effects", td.SideEffects.LegacyText != "" || len(td.SideEffects.Items) > 0},
		{"reversibility.classification", td.Reversibility.Classification != ""},
		{"undo.strategy", td.Undo.Strategy != ""},
		{"errors", len(td.Errors) > 0},
		{"relationships", len(td.Relationships.Before) > 0 || len(td.Relationships.After) > 0 || len(td.Relationships.Overlaps) > 0},
	}
	missing := make([]string, 0, len(checks))
	for _, check := range checks {
		if !check.present {
			missing = append(missing, check.field)
		}
	}
	return missing
}

func sortedKeys(values map[string][]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sourceOrUnknown(source string) string {
	if source == "" {
		return "<unknown source>"
	}
	return source
}

// checkToolEmitsSignalSet verifies that the tools a machine dispatches emit
// signals the machine can route. A machine dispatches a tool as a named
// transition action, or, when it uses $tool dynamic dispatch, any selected tool.
// Tools a profile selects only for REST or machine_request bindings run in
// request-scoped sentences and route their signals there, so they are out of
// scope for the dispatching machine's signal set.
func checkToolEmitsSignalSet(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		signalSet := make(map[string]bool, len(ms.Signals))
		for _, sig := range ms.Signals {
			signalSet[sig.Name] = true
		}

		dispatchesAny := false
		actionTools := make(map[string]bool)
		for _, tr := range ms.Transitions {
			switch tr.Action {
			case "":
				// A transition with no action dispatches no tool.
			case "$tool":
				dispatchesAny = true
			default:
				actionTools[tr.Action] = true
			}
		}

		for _, toolName := range corpus.ToolSelections[agentName] {
			if !dispatchesAny && !actionTools[toolName] {
				continue
			}
			td, ok := corpus.ToolDeclarations[toolName]
			if !ok {
				continue
			}
			for _, emitted := range td.Emits {
				if !signalSet[emitted] {
					findings = append(findings, Finding{
						Check:   "tool-emits-unknown-signal",
						Level:   "warning",
						Message: fmt.Sprintf("agent %s tool %q emits signal %q not in machine signal set", agentName, toolName, emitted),
					})
				}
			}
		}
	}
	return findings
}

// checkToolUndoConsistency verifies that undo strategy aligns with
// reversibility classification.

func checkToolUndoConsistency(corpus *Corpus) []Finding {
	var findings []Finding
	for name, td := range corpus.ToolDeclarations {
		rev := td.Reversibility.Classification
		strat := td.Undo.Strategy
		if rev != "" && strat != "" {
			if !core.UndoStrategyAllowed(rev, strat) {
				findings = append(findings, Finding{
					Check:   "tool-undo-mismatch",
					Level:   "warning",
					Message: fmt.Sprintf("tool %q reversibility is %q but undo strategy is %q", name, rev, strat),
				})
			}
			if !core.UndoStrategySupported(td.Type, strat) {
				findings = append(findings, Finding{
					Check: "tool-undo-unsupported-runtime", Level: "error",
					Message: fmt.Sprintf(
						"tool %q type %q cannot execute undo strategy %q",
						name, td.Type, strat,
					),
				})
			}
		}
		if td.Undo.Payload != "" && len(td.Undo.Captures) == 0 {
			findings = append(findings, Finding{
				Check:   "tool-undo-payload-no-captures",
				Level:   "warning",
				Message: fmt.Sprintf("tool %q has undo payload %q but no captures listed", name, td.Undo.Payload),
			})
		}
	}
	return findings
}

// checkToolSideEffectVocab verifies that side_effects kind values use
// the known vocabulary.

func checkToolSideEffectVocab(corpus *Corpus) []Finding {
	var findings []Finding
	for name, td := range corpus.ToolDeclarations {
		for _, se := range td.SideEffects.Items {
			if se.Kind != "" && !KnownSideEffectKinds[se.Kind] {
				findings = append(findings, Finding{
					Check:   "tool-unknown-side-effect-kind",
					Level:   "error",
					Message: fmt.Sprintf("tool %q side_effects kind %q not in known vocabulary", name, se.Kind),
				})
			}
			if replacement := deprecatedSideEffectTargets[se.Target]; replacement != "" {
				findings = append(findings, Finding{
					Check: "tool-unknown-side-effect-target", Level: "error",
					Message: fmt.Sprintf(
						"tool %q side_effects target %q is invalid; use %q",
						name, se.Target, replacement,
					),
				})
			}
		}
	}
	return findings
}

var deprecatedSideEffectTargets = map[string]string{
	"pipeline_graph": "requirement_graph",
}

// checkToolBoundaryCategory verifies that tools with boundary-class
// side effects declare category: boundary.

func checkToolBoundaryCategory(corpus *Corpus) []Finding {
	boundaryKinds := map[string]bool{
		"child_agent_execution":     true,
		"child_process":             true,
		"nested_machine_execution":  true,
		"external_api":              true,
		"external_api_call":         true,
		"network_listen":            true,
		"network_listener_shutdown": true,
		"human_boundary":            true,
	}
	var findings []Finding
	for name, td := range corpus.ToolDeclarations {
		hasBoundarySE := false
		for _, se := range td.SideEffects.Items {
			if boundaryKinds[se.Kind] {
				hasBoundarySE = true
				break
			}
		}
		if hasBoundarySE && td.Category != "boundary" {
			findings = append(findings, Finding{
				Check:   "tool-boundary-category-missing",
				Level:   "warning",
				Message: fmt.Sprintf("tool %q has boundary side effects but category is %q, expected %q", name, td.Category, "boundary"),
			})
		}
	}
	return findings
}

// checkMachineNameConsistency verifies that the machine.yaml name field
// matches the agent directory name.
