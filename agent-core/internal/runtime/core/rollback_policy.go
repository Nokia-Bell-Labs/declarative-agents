// Copyright (c) 2026 Nokia. All rights reserved.

package core

import "sort"

// undoStrategiesByReversibility is the canonical declared strategy vocabulary.
// Tool implementations may interpret a subset, but declarations and the public
// corpus audit use this one compatibility table.
var undoStrategiesByReversibility = map[string]map[string]bool{
	"irreversible": {
		"irreversible": true,
	},
	"reversible": {
		"noop": true, "reversible": true,
		"snapshot_restore": true, "restore_snapshot": true,
		"file_snapshot_restore": true, "workspace_restore": true,
		"file_snapshot_restore_and_workspace_restore":          true,
		"session_state_restore":                                true,
		"conversation_truncate":                                true,
		"conversation_restore":                                 true,
		"parse_retry_counter_restore":                          true,
		"parse_retry_counter_restore_when_tracker_enabled":     true,
		"pipeline_state_restore":                               true,
		"evaluator_session_restore":                            true,
		"point_context_restore":                                true,
		"owned_artifact_removal":                               true,
		"owned_artifact_removal_and_evaluator_session_restore": true,
		"owned_artifact_removal_and_point_context_restore":     true,
		"queue_event_restore":                                  true,
		"validation_state_restore":                             true,
	},
	"compensatable": {
		"compensatable": true, "boundary_compensation": true,
		"compensating_action": true, "child_command_undo": true,
		"workspace_restore": true, "pipeline_state_restore_only": true,
		"child_agent_workspace_restore":                          true,
		"child_eval_artifact_compensation":                       true,
		"close_or_delete_created_issue":                          true,
		"nested_machine_rollback":                                true,
		"point_workspace_restore_and_child_process_compensation": true,
		"resume_or_checkpoint_rollback":                          true,
		"server_shutdown_or_user_action_compensation":            true,
	},
}

var execUndoStrategies = map[string]bool{
	"": true, "noop": true, "workspace_restore": true,
	"compensating_action": true, "irreversible": true,
}

// UndoStrategyAllowed reports whether strategy is compatible with the declared
// reversibility tier. An unknown tier remains an error owned by the separate
// ToolDef-vocabulary gate.
func UndoStrategyAllowed(reversibility, strategy string) bool {
	allowed, ok := undoStrategiesByReversibility[reversibility]
	return !ok || allowed[strategy]
}

// KnownUndoStrategy reports whether any declared tier recognizes strategy.
func KnownUndoStrategy(strategy string) bool {
	if strategy == "" {
		return true
	}
	for _, allowed := range undoStrategiesByReversibility {
		if allowed[strategy] {
			return true
		}
	}
	return false
}

// UndoStrategySupported reports whether the runtime kind can honor strategy.
// Builtin words own typed Undo implementations, so their strategy names
// describe those implementations. Exec words share one interpreter and must
// stay within its closed set.
func UndoStrategySupported(toolType, strategy string) bool {
	if toolType == "" || toolType == "exec" {
		return execUndoStrategies[strategy]
	}
	return KnownUndoStrategy(strategy)
}

// SupportedUndoStrategies returns the sorted runtime vocabulary for diagnostics.
func SupportedUndoStrategies(toolType string) []string {
	source := execUndoStrategies
	if toolType != "" && toolType != "exec" {
		source = make(map[string]bool)
		for _, strategies := range undoStrategiesByReversibility {
			for strategy := range strategies {
				source[strategy] = true
			}
		}
	}
	out := make([]string, 0, len(source))
	for strategy := range source {
		if strategy != "" {
			out = append(out, strategy)
		}
	}
	sort.Strings(out)
	return out
}
