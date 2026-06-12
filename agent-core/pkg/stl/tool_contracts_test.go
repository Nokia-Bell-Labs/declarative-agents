// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

func TestValidateToolContractsReportsMissingFields(t *testing.T) {
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

func TestValidateToolContractsStrictEscalatesRequiredFindings(t *testing.T) {
	defs := []ToolDef{{Name: "write", Type: "builtin", Init: "file_write"}}

	findings := ValidateToolContracts(defs, ContractValidationOptions{
		Strict:       true,
		MinimumLevel: ContractSeverityError,
	})

	require.NotEmpty(t, findings)
	assertFinding(t, findings, "write", "problem", ContractSeverityError)
	assertNoFinding(t, findings, "write", "relationships")
}

func TestValidateToolContractsCanSkipInternalTools(t *testing.T) {
	defs := []ToolDef{
		{Name: "parse_response", Type: "builtin", Init: "parse_response", Visibility: "internal"},
		{Name: "read", Type: "builtin", Init: "file_read"},
	}

	findings := ValidateToolContracts(defs, ContractValidationOptions{})

	assertNoFinding(t, findings, "parse_response", "problem")
	assertFinding(t, findings, "read", "problem", ContractSeverityWarning)
}

func TestValidateToolContractsCanIncludeInternalTools(t *testing.T) {
	defs := []ToolDef{{Name: "parse_response", Type: "builtin", Init: "parse_response", Visibility: "internal"}}

	findings := ValidateToolContracts(defs, ContractValidationOptions{IncludeInternal: true})

	assertFinding(t, findings, "parse_response", "problem", ContractSeverityWarning)
}

func TestValidateToolContractsUsesExplicitCategory(t *testing.T) {
	defs := []ToolDef{{
		Name:       "run_point",
		Type:       "builtin",
		Init:       "run_point",
		Visibility: "internal",
		Category:   "boundary",
	}}

	findings := ValidateToolContracts(defs, ContractValidationOptions{IncludeInternal: true})

	require.NotEmpty(t, findings)
	for _, finding := range findings {
		assert.Equal(t, "boundary", finding.Category)
	}
}

func TestValidateToolContractsWarnsOnLegacySideEffects(t *testing.T) {
	defs := []ToolDef{{
		Name:        "copy_dir",
		Binary:      "cp",
		SideEffects: ToolSideEffects{LegacyText: "creates files"},
	}}

	findings := ValidateToolContracts(defs, ContractValidationOptions{})

	assertFinding(t, findings, "copy_dir", "side_effects", ContractSeverityWarning)
}

func TestValidateToolContractsRequireStructuredSideEffects(t *testing.T) {
	defs := []ToolDef{{
		Name:  "read",
		Type:  "builtin",
		Init:  "file_read",
		Goals: []string{"Read one file."},
	}}

	findings := ValidateToolContracts(defs, ContractValidationOptions{
		RequireStructuredSideEffects: true,
	})

	assertFinding(t, findings, "read", "side_effects", ContractSeverityWarning)
}

func TestValidateToolContractsCompleteDefinitionHasNoWarnings(t *testing.T) {
	defs := []ToolDef{completeToolDef("parse_csv")}

	findings := ValidateToolContracts(defs, ContractValidationOptions{
		MinimumLevel: ContractSeverityWarning,
	})

	assert.Empty(t, findings)
}

func TestValidateToolContractsMigratedToolsEscalateMissingFields(t *testing.T) {
	defs := []ToolDef{{
		Name:     "parse_response",
		Type:     "builtin",
		Init:     "parse_response",
		Contract: ToolContractMigrated,
	}}

	findings := ValidateToolContracts(defs, ContractValidationOptions{
		MinimumLevel: ContractSeverityError,
	})

	require.NotEmpty(t, findings)
	assertFinding(t, findings, "parse_response", "emits", ContractSeverityError)
	assertFinding(t, findings, "parse_response", "side_effects", ContractSeverityError)
	assertFinding(t, findings, "parse_response", "undo", ContractSeverityError)
	assertFinding(t, findings, "parse_response", "output.schema", ContractSeverityError)
	assertFinding(t, findings, "parse_response", "reversibility.classification", ContractSeverityError)
}

func TestValidateToolContractsLegacyToolsRemainWarnOnlyInStrictMode(t *testing.T) {
	defs := []ToolDef{{
		Name:     "legacy_tool",
		Type:     "builtin",
		Init:     "legacy_tool",
		Contract: ToolContractLegacy,
	}}

	findings := ValidateToolContracts(defs, ContractValidationOptions{
		Strict:       true,
		MinimumLevel: ContractSeverityError,
	})

	assert.Empty(t, findings)
}

func TestAuditToolContractsClassifiesCompletePartialAndMissing(t *testing.T) {
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

func TestAuditToolContractsCanSkipInternalTools(t *testing.T) {
	defs := []ToolDef{
		{Name: "parse_response", Type: "builtin", Init: "parse_response", Visibility: "internal"},
		completeToolDef("read"),
	}

	audit := AuditToolContracts(defs, ContractValidationOptions{})

	require.Len(t, audit, 1)
	assert.Equal(t, "read", audit[0].ToolName)
}

func TestValidateResultSchemaCompatibilityAcceptsDeterministicChain(t *testing.T) {
	spec := schemaCompatibilityMachine()
	defs := []ToolDef{
		{
			Name:  "produce_plan",
			Emits: []string{"PlanReady"},
			Output: ToolOutputContract{Schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"plan_id": map[string]interface{}{"type": "string"},
					"title":   map[string]interface{}{"type": "string"},
				},
				"required": []interface{}{"plan_id"},
			}},
		},
		{
			Name: "materialize_plan",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"plan_id": map[string]interface{}{"type": "string"},
				},
				"required": []interface{}{"plan_id"},
			},
		},
	}

	findings := ValidateResultSchemaCompatibility(spec, defs, ContractValidationOptions{})

	assert.Empty(t, findings)
}

func TestValidateResultSchemaCompatibilityReportsRequiredFieldMismatch(t *testing.T) {
	spec := schemaCompatibilityMachine()
	defs := []ToolDef{
		{
			Name:  "produce_plan",
			Emits: []string{"PlanReady"},
			Output: ToolOutputContract{Schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{"type": "string"},
				},
			}},
		},
		{
			Name: "materialize_plan",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"plan_id": map[string]interface{}{"type": "string"},
				},
				"required": []interface{}{"plan_id"},
			},
		},
	}

	findings := ValidateResultSchemaCompatibility(spec, defs, ContractValidationOptions{Strict: true})

	require.Len(t, findings, 1)
	assert.Equal(t, "produce_plan", findings[0].ToolName)
	assert.Equal(t, "schema_compatibility", findings[0].Category)
	assert.Equal(t, ContractSeverityError, findings[0].Severity)
	assert.Contains(t, findings[0].Message, `does not provide required field "plan_id"`)
	assert.Contains(t, findings[0].Message, `materialize_plan`)
}

func TestValidateResultSchemaCompatibilityDocumentsDynamicDispatchLimitation(t *testing.T) {
	spec := core.MachineSpec{
		Name:           "dynamic",
		States:         core.StateSpecsFromNames("Start", "Selecting", "Done"),
		TerminalStates: []string{"Done"},
		Signals:        core.SignalSpecsFromNames("Seed", "ToolReady", "ToolDone"),
		Transitions: []core.TransitionSpec{
			{State: "Start", Signal: "Seed", Next: "Selecting", Action: "parse_response"},
			{State: "Selecting", Signal: "ToolReady", Next: "Selecting", Action: "$tool"},
			{State: "Selecting", Signal: "ToolDone", Next: "Done"},
		},
	}
	defs := []ToolDef{{
		Name:  "parse_response",
		Emits: []string{"ToolReady"},
		Output: ToolOutputContract{Schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tool":       map[string]interface{}{"type": "string"},
				"parameters": map[string]interface{}{"type": "object"},
			},
		}},
	}}

	findings := ValidateResultSchemaCompatibility(spec, defs, ContractValidationOptions{})

	require.Len(t, findings, 1)
	assert.Equal(t, ContractSeverityInfo, findings[0].Severity)
	assert.Equal(t, "dynamic_dispatch", findings[0].Field)
	assert.Contains(t, findings[0].Message, "dynamic $tool dispatch")
}

func schemaCompatibilityMachine() core.MachineSpec {
	return core.MachineSpec{
		Name:           "schema-chain",
		States:         core.StateSpecsFromNames("Start", "Planning", "Done"),
		TerminalStates: []string{"Done"},
		Signals:        core.SignalSpecsFromNames("Seed", "PlanReady", "Materialized"),
		Transitions: []core.TransitionSpec{
			{State: "Start", Signal: "Seed", Next: "Planning", Action: "produce_plan"},
			{State: "Planning", Signal: "PlanReady", Next: "Done", Action: "materialize_plan"},
		},
	}
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
			require.Failf(t, "unexpected finding", "tool=%s field=%s", tool, field)
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
