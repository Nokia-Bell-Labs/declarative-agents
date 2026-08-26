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

func TestValidateToolPhasesRejectsManifestStateDifferentFromDynamicTarget(t *testing.T) {
	t.Parallel()
	machine := dynamicValidationMachine()
	defs := dynamicValidationDefs("Answering")

	err := ValidateToolPhases(machine, defs)

	require.ErrorContains(t, err,
		`invoke_llm tool "select_tool" manifest_state "Answering" disagrees with $tool target "Reporting" after Answering/ToolReady`)
}

func TestValidateToolPhasesAcceptsManifestStateMatchingDynamicTarget(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidateToolPhases(
		dynamicValidationMachine(), dynamicValidationDefs("Reporting"),
	))
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
				dynamicValidationDefs("Reporting"), tt.tool,
			))
			require.ErrorContains(t, err, tt.message)
		})
	}
}

func TestValidateToolPhasesRejectsDeclarationOrderDependentManifestStates(t *testing.T) {
	t.Parallel()
	defs := dynamicValidationDefs("Reporting")
	defs = append(defs, ToolDef{
		Name: "alternate_model", Type: "builtin", Init: "invoke_llm",
		Visibility: "internal", Config: map[string]interface{}{"manifest_state": "Answering"},
	})

	err := ValidateToolPhases(dynamicValidationMachine(), defs)

	require.ErrorContains(t, err,
		`invoke_llm tool "alternate_model" manifest_state "Answering" disagrees with $tool target "Reporting"`)
}

func dynamicValidationMachine() core.MachineSpec {
	return core.MachineSpec{
		Name:           "dynamic-validation",
		States:         core.StateSpecsFromNames("Answering", "Reporting", "Done", "Failed"),
		TerminalStates: []string{"Done", "Failed"},
		Signals:        core.SignalSpecsFromNames("ToolReady", "ToolDone", "ToolFailed", "SearchDone"),
		Transitions: []core.TransitionSpec{
			{State: "Answering", Signal: "ToolReady", Next: "Reporting", Action: "$tool"},
			{State: "Reporting", Signal: "ToolDone", Next: "Done"},
			{State: "Reporting", Signal: "ToolFailed", Next: "Failed"},
		},
	}
}

func dynamicValidationDefs(manifestState string) []ToolDef {
	return []ToolDef{
		{
			Name: "select_tool", Type: "builtin", Init: "invoke_llm",
			Visibility: "internal", Config: map[string]interface{}{"manifest_state": manifestState},
		},
		{
			Name: "parse_tool", Type: "builtin", Init: "parse_response",
			Visibility: "internal",
		},
		{
			Name: "write", Type: "builtin", Init: "file_write",
			Emits: []string{"ToolDone", "ToolFailed"},
		},
	}
}
