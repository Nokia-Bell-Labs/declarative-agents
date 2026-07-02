// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"fmt"
	"sort"
	"strings"
)

func checkToolSelectionDeclared(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range sortedToolSelectionKeys(corpus.ToolSelections) {
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
	for _, selectionName := range sortedToolSelectionKeys(corpus.ToolSelections) {
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
		{"side_effects", len(td.SideEffects.Items) > 0},
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

func sortedToolSelectionKeys(selections map[string][]string) []string {
	keys := make([]string, 0, len(selections))
	for key := range selections {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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

// checkToolEmitsSignalSet verifies that tool emits signals are valid
// signal names in the machine that uses the tool.

func checkToolEmitsSignalSet(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		signalSet := make(map[string]bool, len(ms.Signals))
		for _, sig := range ms.Signals {
			signalSet[sig.Name] = true
		}
		selected := corpus.ToolSelections[agentName]
		for _, toolName := range selected {
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
			if !undoStrategyAllowed(rev, strat) {
				findings = append(findings, Finding{
					Check:   "tool-undo-mismatch",
					Level:   "warning",
					Message: fmt.Sprintf("tool %q reversibility is %q but undo strategy is %q", name, rev, strat),
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

func undoStrategyAllowed(reversibility, strategy string) bool {
	allowed, ok := undoStrategiesByReversibility[reversibility]
	if !ok {
		return true
	}
	return allowed[strategy]
}

var undoStrategiesByReversibility = map[string]map[string]bool{
	"irreversible": {
		"irreversible": true,
	},
	"reversible": {
		"noop":              true,
		"reversible":        true,
		"snapshot_restore":  true,
		"workspace_restore": true,
		"file_snapshot_restore_and_workspace_restore":      true,
		"session_state_restore":                            true,
		"conversation_truncate":                            true,
		"conversation_restore":                             true,
		"parse_retry_counter_restore":                      true,
		"parse_retry_counter_restore_when_tracker_enabled": true,
		"pipeline_state_restore":                           true,
		"evaluator_session_restore":                        true,
		"point_context_restore":                            true,
		"validation_state_restore":                         true,
	},
	"compensatable": {
		"compensatable":                                          true,
		"boundary_compensation":                                  true,
		"compensating_action":                                    true,
		"child_command_undo":                                     true,
		"workspace_restore":                                      true,
		"pipeline_state_restore_only":                            true,
		"child_agent_workspace_restore":                          true,
		"child_eval_artifact_compensation":                       true,
		"close_or_delete_created_issue":                          true,
		"nested_machine_rollback":                                true,
		"point_workspace_restore_and_child_process_compensation": true,
		"resume_or_checkpoint_rollback":                          true,
		"server_shutdown_or_user_action_compensation":            true,
	},
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
		}
	}
	return findings
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
