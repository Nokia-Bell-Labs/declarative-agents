// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestValidateParseRetryWiringAcceptsExplicitRetryAndExhaustion(t *testing.T) {
	spec := retryMachineSpec()

	require.NoError(t, ValidateParseRetryWiring(spec, []ToolDef{{Name: "report_parse_error"}}))
}

func TestValidateParseRetryWiringReportsActionableProfileErrors(t *testing.T) {
	spec := retryMachineSpec()
	spec.Transitions = []core.TransitionSpec{
		{State: "Parsing", Signal: "ParseFailed", Next: "Composing", Action: "invoke_llm"},
	}

	err := ValidateParseRetryWiring(spec, []ToolDef{{Name: "invoke_llm"}})

	require.Error(t, err)
	require.Contains(t, err.Error(), `select "report_parse_error" in the profile tools`)
	require.Contains(t, err.Error(), `route a ParseFailed transition through action "report_parse_error"`)
	require.Contains(t, err.Error(), "remove the parse retry budget")
}

func TestValidateParseRetryWiringRequiresExhaustionRouteFromReportDestination(t *testing.T) {
	spec := retryMachineSpec()
	spec.Transitions = spec.Transitions[:1]

	err := ValidateParseRetryWiring(spec, []ToolDef{{Name: "report_parse_error"}})

	require.Error(t, err)
	require.Contains(t, err.Error(), `route BudgetExhausted from state "Composing"`)
}

func TestValidateParseRetryWiringIgnoresMachinesWithoutParseRetryBudget(t *testing.T) {
	spec := retryMachineSpec()
	spec.BudgetSpec.MaxConsecutiveParseErrors = 0

	require.NoError(t, ValidateParseRetryWiring(spec, nil))
}

func retryMachineSpec() core.MachineSpec {
	return core.MachineSpec{
		Name:           "retrying",
		TerminalStates: []string{"Failed"},
		BudgetSpec:     &core.BudgetSpec{MaxConsecutiveParseErrors: 3},
		Transitions: []core.TransitionSpec{
			{State: "Parsing", Signal: "ParseFailed", Next: "Composing", Action: "report_parse_error"},
			{State: "Composing", Signal: "BudgetExhausted", Next: "Failed"},
		},
	}
}
