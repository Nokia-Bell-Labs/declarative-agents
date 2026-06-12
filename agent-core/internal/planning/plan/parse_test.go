// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const validPlanYAML = `title: Implement config parser
summary: Parse YAML configuration files into typed structs.
files:
  - path: internal/config/config.go
    action: create
    note: Config struct and loader
  - path: internal/config/config_test.go
    action: create
requirements:
  - id: R1
    text: Define Config struct with typed fields
  - id: R2
    text: Parse YAML files into Config
design_decisions:
  - id: D1
    text: Use yaml.v3 for parsing
acceptance_criteria:
  - id: AC1
    text: Config loads from a valid YAML file
  - id: AC2
    text: Error returned for invalid YAML input
`

// TestRel00_1_UC001_PlanRoundTrip verifies ImplementationPlan marshals
// to YAML and unmarshals back to an identical struct (srd008 AC5).
func TestRel00_1_UC001_PlanRoundTrip(t *testing.T) {
	t.Parallel()
	original := ImplementationPlan{
		Title:   "Implement config parser",
		Summary: "Parse YAML configuration files into typed structs.",
		Files: []PlanFile{
			{Path: "internal/config/config.go", Action: "create", Note: "Config struct and loader"},
			{Path: "internal/config/config_test.go", Action: "create"},
		},
		Requirements: []PlanRequirement{
			{ID: "R1", Text: "Define Config struct with typed fields"},
			{ID: "R2", Text: "Parse YAML files into Config"},
		},
		DesignDecisions: []PlanDecision{
			{ID: "D1", Text: "Use yaml.v3 for parsing"},
		},
		AcceptanceCriteria: []PlanCriterion{
			{ID: "AC1", Text: "Config loads from a valid YAML file"},
			{ID: "AC2", Text: "Error returned for invalid YAML input"},
		},
	}

	data, err := yaml.Marshal(original)
	require.NoError(t, err)

	var roundTripped ImplementationPlan
	require.NoError(t, yaml.Unmarshal(data, &roundTripped))

	assert.Equal(t, original, roundTripped)
}

// TestRel00_1_UC001_ParsePlanHandlesCodeFences verifies ParsePlan
// accepts YAML both with and without markdown code fences (srd008 AC6).
func TestRel00_1_UC001_ParsePlanHandlesCodeFences(t *testing.T) {
	t.Parallel()

	barePlan, err := ParsePlan(validPlanYAML)
	require.NoError(t, err)

	fenced := "Here is the plan:\n\n```yaml\n" + validPlanYAML + "```\n\nLet me know if you need changes."
	fencedPlan, err := ParsePlan(fenced)
	require.NoError(t, err)

	assert.Equal(t, barePlan, fencedPlan)

	plainFenced := "```\n" + validPlanYAML + "```"
	plainPlan, err := ParsePlan(plainFenced)
	require.NoError(t, err)

	assert.Equal(t, barePlan, plainPlan)
}

// TestRel00_1_UC001_ParsePlanRejectsInvalidInput uses table-driven
// cases for empty, malformed, and incomplete inputs (srd008 AC7).
func TestRel00_1_UC001_ParsePlanRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		input       string
		errContains string
	}{
		{
			name:        "empty string",
			input:       "",
			errContains: "empty",
		},
		{
			name:        "whitespace only",
			input:       "   \n  \t  ",
			errContains: "empty",
		},
		{
			name:        "invalid YAML",
			input:       "title: [broken\n  : :",
			errContains: "YAML",
		},
		{
			name: "missing title",
			input: `requirements:
  - id: R1
    text: do something
acceptance_criteria:
  - id: AC1
    text: verify it`,
			errContains: "title",
		},
		{
			name: "missing requirements",
			input: `title: Some plan
acceptance_criteria:
  - id: AC1
    text: verify it`,
			errContains: "requirements",
		},
		{
			name: "missing acceptance_criteria",
			input: `title: Some plan
requirements:
  - id: R1
    text: do something`,
			errContains: "acceptance_criteria",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParsePlan(tc.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errContains)
		})
	}
}
