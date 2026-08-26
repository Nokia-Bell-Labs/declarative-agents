// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestValidateToolPhasesRejectsUnknownState(t *testing.T) {
	t.Parallel()
	machine := core.MachineSpec{
		States: core.StateSpecsFromNames("Composing", "Done"),
	}
	err := ValidateToolPhases(machine, []ToolDef{{
		Name: "write", Phases: []string{"Compsing"},
	}})
	require.ErrorContains(t, err, `tool "write" declares unknown phase "Compsing"`)
}

func TestValidateToolPhasesAcceptsDeclaredAndUnscopedWords(t *testing.T) {
	t.Parallel()
	machine := core.MachineSpec{
		States: core.StateSpecsFromNames("Composing", "Done"),
	}
	require.NoError(t, ValidateToolPhases(machine, []ToolDef{
		{Name: "write", Phases: []string{"Composing"}},
		{Name: "done"},
	}))
}

func TestValidateToolPhasesRejectsActualSelectorStateDifferentFromDynamicTarget(t *testing.T) {
	t.Parallel()
	machine := dynamicValidationMachine()
	defs := dynamicValidationDefs("Answering", "Reporting")

	err := ValidateToolPhases(machine, defs)

	require.ErrorContains(t, err,
		`invoke_llm selector tool "select_tool" manifest_state "Answering" disagrees with $tool target "Reporting" after Answering/ToolReady`)
}

func TestValidateToolPhasesAcceptsManifestStateMatchingDynamicTarget(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidateToolPhases(
		dynamicValidationMachine(), dynamicValidationDefs("Reporting", "Reporting"),
	))
}

func TestValidateToolPhasesRejectsActualParserStateDifferentFromDynamicTarget(t *testing.T) {
	t.Parallel()
	err := ValidateToolPhases(
		dynamicValidationMachine(), dynamicValidationDefs("Reporting", "Answering"),
	)
	require.ErrorContains(t, err,
		`parse_response tool "parse_tool" manifest_state "Answering" disagrees with $tool target "Reporting" after Answering/ToolReady`)
}

func TestValidateToolPhasesRejectsWordScopedToNoState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		tool    ToolDef
		message string
	}{
		{
			name: "explicit phase intersection",
			tool: ToolDef{
				Name: "scoped_write", Type: "builtin", Init: "file_write",
				Phases: []string{"Answering"}, Emits: []string{"ToolDone", "ToolFailed"},
			},
			message: `tool "scoped_write" derives no dynamic phase: target "Reporting" is excluded by explicit phases [Answering]`,
		},
		{
			name: "no emitted signals",
			tool: ToolDef{
				Name: "silent", Type: "builtin", Init: "silent",
			},
			message: `tool "silent" derives no dynamic phase: target "Reporting" cannot route a tool with no declared emitted signals`,
		},
		{
			name: "unroutable emitted signal",
			tool: ToolDef{
				Name: "search", Type: "builtin", Init: "search",
				Emits: []string{"SearchDone"},
			},
			message: `tool "search" derives no dynamic phase: target "Reporting" has no transition for emitted signals [SearchDone]`,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateToolPhases(dynamicValidationMachine(), append(
				dynamicValidationDefs("Reporting", "Reporting"), tt.tool,
			))
			require.ErrorContains(t, err, tt.message)
		})
	}
}

func TestValidateToolPhasesIgnoresUnrelatedInvokeManifestStatesAndOrder(t *testing.T) {
	t.Parallel()
	defs := dynamicValidationDefs("Reporting", "Reporting")
	defs = append(defs, ToolDef{
		Name: "alternate_model", Type: "builtin", Init: "invoke_llm",
		Visibility: "internal", Config: map[string]interface{}{"manifest_state": "Answering"},
	})

	require.NoError(t, ValidateToolPhases(dynamicValidationMachine(), defs))

	for left, right := 0, len(defs)-1; left < right; left, right = left+1, right-1 {
		defs[left], defs[right] = defs[right], defs[left]
	}
	require.NoError(t, ValidateToolPhases(dynamicValidationMachine(), defs))
}

func dynamicValidationMachine() core.MachineSpec {
	return core.MachineSpec{
		Name:           "dynamic-validation",
		States:         core.StateSpecsFromNames("Preparing", "Selecting", "Answering", "Reporting", "Done", "Failed"),
		TerminalStates: []string{"Done", "Failed"},
		Signals:        core.SignalSpecsFromNames("Seed", "LLMResponded", "ToolReady", "ToolDone", "ToolFailed", "SearchDone"),
		Transitions: []core.TransitionSpec{
			{State: "Preparing", Signal: "Seed", Next: "Selecting", Action: "select_tool"},
			{State: "Selecting", Signal: "LLMResponded", Next: "Answering", Action: "parse_tool"},
			{State: "Answering", Signal: "ToolReady", Next: "Reporting", Action: "$tool"},
			{State: "Reporting", Signal: "ToolDone", Next: "Done"},
			{State: "Reporting", Signal: "ToolFailed", Next: "Failed"},
		},
	}
}

func dynamicValidationDefs(selectorState, parserState string) []ToolDef {
	return []ToolDef{
		{
			Name: "select_tool", Type: "builtin", Init: "invoke_llm",
			Visibility: "internal", Emits: []string{"LLMResponded"},
			Config: map[string]interface{}{"manifest_state": selectorState},
		},
		{
			Name: "parse_tool", Type: "builtin", Init: "parse_response",
			Visibility: "internal", Emits: []string{"ToolReady"},
			Config: map[string]interface{}{"manifest_state": parserState},
		},
		{
			Name: "write", Type: "builtin", Init: "file_write",
			Emits: []string{"ToolDone", "ToolFailed"},
		},
	}
}
