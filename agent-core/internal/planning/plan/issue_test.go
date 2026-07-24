// Copyright (c) 2026 Nokia. All rights reserved.

package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestFormatIssueDescriptionMapsPlanContract(t *testing.T) {
	t.Parallel()
	input := ImplementationPlan{
		Title: "Implement parser",
		Files: []PlanFile{
			{Path: "parser.go", Action: "create"},
			{Path: "parser_test.go", Action: "create"},
		},
		Requirements:       []PlanRequirement{{ID: "R1", Text: "Parse input"}},
		AcceptanceCriteria: []PlanCriterion{{ID: "AC1", Text: "Tests pass"}},
	}
	formatted, err := FormatIssueDescription(input)
	require.NoError(t, err)

	var output issueDescription
	require.NoError(t, yaml.Unmarshal([]byte(formatted), &output))
	assert.Equal(t, "code", output.DeliverableType)
	assert.Equal(t, []string{"parser.go", "parser_test.go"}, output.RequiredReading)
	assert.Equal(t, input.Files, output.Files)
	assert.Equal(t, input.Requirements, output.Requirements)
	assert.Equal(t, input.AcceptanceCriteria, output.AcceptanceCriteria)
}

func TestFormatIssueDescriptionOmitsEmptyDecisions(t *testing.T) {
	t.Parallel()
	formatted, err := FormatIssueDescription(ImplementationPlan{})
	require.NoError(t, err)
	assert.NotContains(t, formatted, "design_decisions")

	withDecision, err := FormatIssueDescription(ImplementationPlan{
		DesignDecisions: []PlanDecision{{ID: "D1", Text: "Use YAML"}},
	})
	require.NoError(t, err)
	assert.Contains(t, withDecision, "design_decisions")
	assert.Contains(t, withDecision, "Use YAML")
}
