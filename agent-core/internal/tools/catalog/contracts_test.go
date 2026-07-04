// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestValidateToolContractsReportsMissingFields(t *testing.T) {
	t.Parallel()
	defs := []ToolDef{{Name: "read", Type: "builtin", Init: "file_read"}}

	findings := ValidateToolContracts(defs, ContractValidationOptions{})

	require.NotEmpty(t, findings)
	assertFinding(t, findings, "read", "problem", ContractSeverityWarning)
	assertFinding(t, findings, "read", "goals", ContractSeverityWarning)
	assertFinding(t, findings, "read", "requirements.input", ContractSeverityWarning)
	assertFinding(t, findings, "read", "output.schema", ContractSeverityWarning)
	assertFinding(t, findings, "read", "reversibility.classification", ContractSeverityWarning)
	assertFinding(t, findings, "read", "relationships", ContractSeverityInfo)
}

func TestValidateToolContractsStrictAndLegacyModes(t *testing.T) {
	t.Parallel()
	migrated := ToolDef{Name: "parse_response", Type: "builtin", Init: "parse_response", Contract: ToolContractMigrated}
	legacy := ToolDef{Name: "legacy_tool", Type: "builtin", Init: "legacy_tool", Contract: ToolContractLegacy}

	migratedFindings := ValidateToolContracts([]ToolDef{migrated}, ContractValidationOptions{MinimumLevel: ContractSeverityError})
	legacyFindings := ValidateToolContracts([]ToolDef{legacy}, ContractValidationOptions{
		Strict:       true,
		MinimumLevel: ContractSeverityError,
	})

	require.NotEmpty(t, migratedFindings)
	assertFinding(t, migratedFindings, "parse_response", "emits", ContractSeverityError)
	assertFinding(t, migratedFindings, "parse_response", "side_effects", ContractSeverityError)
	assert.Empty(t, legacyFindings)
}

func TestValidateToolContractsSideEffectModes(t *testing.T) {
	t.Parallel()
	defs := []ToolDef{{
		Name:        "copy_dir",
		Binary:      "cp",
		SideEffects: ToolSideEffects{LegacyText: "creates files"},
	}}

	findings := ValidateToolContracts(defs, ContractValidationOptions{RequireStructuredSideEffects: true})

	assertFinding(t, findings, "copy_dir", "side_effects", ContractSeverityWarning)
}

func TestAuditToolContractsClassifiesCoverage(t *testing.T) {
	t.Parallel()
	defs := []ToolDef{
		completeToolDef("parse_csv"),
		{
			Name:        "write",
			Type:        "builtin",
			Init:        "file_write",
			Category:    "word",
			Description: "Write a file.",
			Parameters:  map[string]interface{}{"type": "object"},
			Emits:       []string{"ToolDone", "ToolFailed"},
			Output: ToolOutputContract{
				Schema: map[string]interface{}{"type": "object"},
			},
			SideEffects: ToolSideEffects{
				Items: []ToolSideEffect{{Kind: "filesystem_write", Target: "workspace"}},
			},
		},
		{Name: "parse_response", Type: "builtin", Init: "parse_response", Visibility: "internal"},
	}

	audit := AuditToolContracts(defs, ContractValidationOptions{IncludeInternal: true})

	require.Len(t, audit, 3)
	assertAuditEntry(t, audit, "parse_csv", ContractAuditComplete, "")
	assertAuditEntry(t, audit, "write", ContractAuditPartial, "declare filesystem effects")
	assertAuditEntry(t, audit, "parse_response", ContractAuditMissing, "classify as word")
}

func TestValidateResultSchemaCompatibilityReportsRequiredFieldMismatch(t *testing.T) {
	t.Parallel()
	spec := core.MachineSpec{
		Name:           "schema-chain",
		States:         core.StateSpecsFromNames("Start", "Planning", "Done"),
		TerminalStates: []string{"Done"},
		Signals:        core.SignalSpecsFromNames("Seed", "PlanReady", "Materialized"),
		Transitions: []core.TransitionSpec{
			{State: "Start", Signal: "Seed", Next: "Planning", Action: "produce_plan"},
			{State: "Planning", Signal: "PlanReady", Next: "Done", Action: "materialize_plan"},
		},
	}
	defs := []ToolDef{
		{
			Name:  "produce_plan",
			Emits: []string{"PlanReady"},
			Output: ToolOutputContract{Schema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"title": map[string]interface{}{"type": "string"}},
			}},
		},
		{
			Name: "materialize_plan",
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"plan_id": map[string]interface{}{"type": "string"}},
				"required":   []interface{}{"plan_id"},
			},
		},
	}

	findings := ValidateResultSchemaCompatibility(spec, defs, ContractValidationOptions{Strict: true})

	require.Len(t, findings, 1)
	assert.Equal(t, "produce_plan", findings[0].ToolName)
	assert.Equal(t, "schema_compatibility", findings[0].Category)
	assert.Equal(t, ContractSeverityError, findings[0].Severity)
	assert.Contains(t, findings[0].Message, `does not provide required field "plan_id"`)
}

func TestToolAuthoringOneVerbRule(t *testing.T) {
	t.Parallel()
	def := completeToolDef("load_suite")
	def.Category = "word"
	def.Description = "Load config and discover samples and initialize session state."
	def.Problem = "The evaluator needs one hidden step that loads, discovers, and initializes a suite."

	findings := ReviewToolAuthoring([]ToolDef{def}, ContractValidationOptions{Strict: true})

	assertFinding(t, findings, "load_suite", "description", ContractSeverityError)
}

func TestToolAuthoringIrreversibleConfirmation(t *testing.T) {
	t.Parallel()
	def := completeToolDef("publish_release")
	def.Category = "boundary"
	def.SideEffects = ToolSideEffects{Items: []ToolSideEffect{{
		Kind:        "external_service_write",
		Target:      "release_registry",
		Description: "Publishes immutable artifacts to external users.",
	}}}
	def.Reversibility = ToolReversibility{Classification: "irreversible", Undo: "none"}
	def.Undo = ToolUndoContract{Strategy: "none", Description: "Publication cannot be undone safely."}

	findings := ReviewToolAuthoring([]ToolDef{def}, ContractValidationOptions{Strict: true})

	assertFinding(t, findings, "publish_release", "reversibility.requires_confirmation", ContractSeverityError)

	def.Reversibility.RequiresConfirmation = true
	findings = ReviewToolAuthoring([]ToolDef{def}, ContractValidationOptions{Strict: true})
	assertNoFinding(t, findings, "publish_release", "reversibility.requires_confirmation")
}

func TestToolAuthoringRelationshipReview(t *testing.T) {
	t.Parallel()
	def := completeToolDef("collect_artifact_metadata")
	def.Relationships = ToolRelationships{
		Overlaps: []ToolOverlap{
			{Tool: "collect_metrics"},
			{Tool: "dump_config", Difference: "Reads static config rather than runtime artifact metadata."},
		},
	}

	findings := ReviewToolAuthoring([]ToolDef{def}, ContractValidationOptions{})

	assertFinding(t, findings, "collect_artifact_metadata", "relationships", ContractSeverityInfo)

	def.Relationships.Before = []string{"run_agent"}
	def.Relationships.After = []string{"summarize_point_results"}
	def.Relationships.Overlaps[0].Difference = "Reads artifact file metadata rather than monitor samples."
	findings = ReviewToolAuthoring([]ToolDef{def}, ContractValidationOptions{})
	assertNoFinding(t, findings, "collect_artifact_metadata", "relationships")
}

func completeToolDef(name string) ToolDef {
	return ToolDef{
		Name:        name,
		Binary:      "csvtool",
		Category:    "word",
		Description: "Parse CSV.",
		Problem:     "The agent needs structured CSV rows.",
		Goals:       []string{"Return rows deterministically."},
		Requirements: ToolRequirements{
			Input:  []string{"must accept one path"},
			Output: []string{"must return rows"},
			Errors: []string{"must fail on invalid CSV"},
		},
		NonGoals: []string{"Does not clean data."},
		Output: ToolOutputContract{
			Schema: map[string]interface{}{"type": "object"},
		},
		SideEffects: ToolSideEffects{
			Items: []ToolSideEffect{{Kind: "none"}},
		},
		Reversibility: ToolReversibility{Classification: "reversible"},
		Undo:          ToolUndoContract{Strategy: "noop", Description: "Read-only command has no state to restore."},
		Errors: []ToolErrorContract{{
			Signal:            "CommandError",
			Condition:         "CSV cannot be parsed",
			MessageShape:      "parse_csv: <error>",
			StateAfterFailure: "no state changed",
		}},
		Relationships: ToolRelationships{
			Before: []string{"read"},
		},
		Emits:      []string{"ToolDone", "CommandError"},
		Parameters: map[string]interface{}{"type": "object"},
	}
}

func assertFinding(t *testing.T, findings []ContractFinding, tool, field, severity string) {
	t.Helper()
	for _, finding := range findings {
		if finding.ToolName == tool && finding.Field == field {
			assert.Equal(t, severity, finding.Severity)
			assert.NotEmpty(t, finding.Message)
			assert.NotEmpty(t, finding.Remediation)
			return
		}
	}
	require.Failf(t, "missing finding", "tool=%s field=%s", tool, field)
}

func assertNoFinding(t *testing.T, findings []ContractFinding, tool, field string) {
	t.Helper()
	for _, finding := range findings {
		if finding.ToolName == tool && finding.Field == field {
			require.Failf(t, "unexpected finding", "tool=%s field=%s finding=%+v", tool, field, finding)
		}
	}
}

func assertAuditEntry(t *testing.T, audit []ContractAuditEntry, tool, status, migrationSubstring string) {
	t.Helper()
	for _, entry := range audit {
		if entry.ToolName != tool {
			continue
		}
		assert.Equal(t, status, entry.Status)
		if status == ContractAuditComplete {
			assert.Empty(t, entry.MissingFields)
			assert.Empty(t, entry.MigrationTarget)
		} else {
			assert.NotEmpty(t, entry.MissingFields)
			assert.Contains(t, entry.MigrationTarget, migrationSubstring)
		}
		return
	}
	require.Failf(t, "missing audit entry", "tool=%s", tool)
}
