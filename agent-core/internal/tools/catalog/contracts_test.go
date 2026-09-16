// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

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
