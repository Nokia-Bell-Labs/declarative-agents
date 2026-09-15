// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func signedToolIn(category string) ToolDef {
	return ToolDef{
		Name: "word_tool", Type: "builtin", Init: "array_transform",
		Category:    category,
		Description: "Flatten compatible sources one row per chunk.",
		Signature: &ToolSignature{
			Input: "chat-types.Sources", Output: "chat-types.Rows",
			Emits: []string{"Done", "CommandError"},
		},
		Emits:         []string{"Done", "CommandError"},
		Output:        ToolOutputContract{Schema: map[string]interface{}{"type": "object"}},
		Relationships: ToolRelationships{After: []string{"load_sources"}},
		Errors:        []ToolErrorContract{{Signal: "CommandError", Condition: "the declared output cannot be produced"}},
	}
}

func contractFields(t *testing.T, def ToolDef) map[string]bool {
	t.Helper()
	missing := map[string]bool{}
	for _, finding := range ValidateToolContracts(
		[]ToolDef{def}, ContractValidationOptions{IncludeInternal: true},
	) {
		missing[finding.Field] = true
	}
	return missing
}

func TestSignatureDefaultsDischargeWordContract(t *testing.T) {
	t.Parallel()
	def := applyContractDefaults(signedToolIn("word"))

	require.Empty(t, contractFields(t, def),
		"a word tool with a signature satisfies every contract check")
	require.Equal(t, "reversible", def.Reversibility.Classification)
	require.Equal(t, "noop", def.Undo.Strategy)
	require.Len(t, def.SideEffects.Items, 1)
	require.Equal(t, "none", def.SideEffects.Items[0].Kind)
	require.NotEmpty(t, def.Problem)
	require.NotEmpty(t, def.Goals)
	require.NotEmpty(t, def.NonGoals)
	require.NotEmpty(t, def.Requirements.Input)
}

func TestSignatureDefaultsDischargeResponseContract(t *testing.T) {
	t.Parallel()
	def := applyContractDefaults(signedToolIn("response"))
	require.Empty(t, contractFields(t, def))
}

// srd051 R6.9: a boundary tool states what it writes, what that costs to
// reverse, and how, signature or not.
func TestSignatureNeverWaivesBoundaryObligations(t *testing.T) {
	t.Parallel()
	def := applyContractDefaults(signedToolIn("boundary"))

	missing := contractFields(t, def)

	// ValidateToolContracts does not check side_effects presence; that
	// obligation is the corpus audit's, covered in pkg/spec. What this checker
	// owns is reversibility and undo, and a signature does not discharge them
	// for boundary.
	require.True(t, missing["reversibility.classification"])
	require.True(t, missing["undo"])
	require.False(t, missing["problem"], "the descriptive blocks still default")

	sideEffects, reversibility, undo := SignatureDischarges("boundary")
	require.False(t, sideEffects, "the shared table discharges nothing for boundary")
	require.False(t, reversibility)
	require.False(t, undo)
}

func TestSignatureDefaultsLeaveStatefulInternalSideEffectsRequired(t *testing.T) {
	t.Parallel()
	def := applyContractDefaults(signedToolIn("stateful_internal"))

	missing := contractFields(t, def)

	require.False(t, missing["reversibility.classification"])
	require.False(t, missing["undo"])

	sideEffects, _, _ := SignatureDischarges("stateful_internal")
	require.False(t, sideEffects,
		"stateful_internal mutates state a category cannot name for it")
	require.Empty(t, def.SideEffects.Items,
		"and defaulting leaves it for the author to declare")
}

// srd051 R6.10: defaulting fills what an author omitted; it never replaces
// what an author stated.
func TestExplicitBlocksOverrideTheirDefaults(t *testing.T) {
	t.Parallel()
	def := signedToolIn("word")
	def.Problem = "an authored problem"
	def.Reversibility.Classification = "compensatable"
	def.Undo.Strategy = "restore"
	def.SideEffects.Items = []ToolSideEffect{{Kind: "external_api", State: "written"}}
	def.Goals = []string{"an authored goal"}

	def = applyContractDefaults(def)

	require.Equal(t, "an authored problem", def.Problem)
	require.Equal(t, "compensatable", def.Reversibility.Classification)
	require.Equal(t, "restore", def.Undo.Strategy)
	require.Equal(t, "external_api", def.SideEffects.Items[0].Kind)
	require.Equal(t, []string{"an authored goal"}, def.Goals)
}

func TestToolWithoutSignatureGetsNoDefaults(t *testing.T) {
	t.Parallel()
	def := signedToolIn("word")
	def.Signature = nil

	def = applyContractDefaults(def)

	require.Empty(t, def.Problem, "defaulting is a signature's effect, not a category's")
	require.Empty(t, def.Reversibility.Classification)
}

func TestSignatureDischargesMatchesTheTable(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		category                         string
		sideEffects, reversibility, undo bool
	}{
		{"word", true, true, true},
		{"response", true, true, true},
		{"stateful_internal", false, true, true},
		{"boundary", false, false, false},
		{"unknown_category", false, false, false},
	} {
		sideEffects, reversibility, undo := SignatureDischarges(row.category)
		require.Equalf(t, row.sideEffects, sideEffects, "%s side_effects", row.category)
		require.Equalf(t, row.reversibility, reversibility, "%s reversibility", row.category)
		require.Equalf(t, row.undo, undo, "%s undo", row.category)
	}
}
