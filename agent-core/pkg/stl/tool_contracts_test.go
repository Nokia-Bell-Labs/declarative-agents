// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		Relationships: ToolRelationships{
			Before: []string{"read"},
		},
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
