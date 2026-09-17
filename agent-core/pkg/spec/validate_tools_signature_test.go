// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func signedDeclaration(category string) ToolDeclaration {
	return ToolDeclaration{
		Name: "word_tool", Type: "builtin", Category: category,
		Signature: &catalog.ToolSignature{
			Input: "chat-types.Sources", Output: "chat-types.Rows",
			Emits: []string{"Done", "CommandError"},
		},
	}
}

func TestSignatureDischargesWordContractInTheCorpusAudit(t *testing.T) {
	t.Parallel()
	require.Empty(t, missingToolContractFields(signedDeclaration("word")),
		"a word tool needs only name, description, category, init, signature, config")
}

// srd051 R6.9. This is the check that actually enforces the side-effect
// obligation; catalog's contract validation does not test presence.
func TestSignatureNeverWaivesBoundarySideEffects(t *testing.T) {
	t.Parallel()
	missing := missingToolContractFields(signedDeclaration("boundary"))

	require.Contains(t, missing, "side_effects")
	require.Contains(t, missing, "reversibility.classification")
	require.Contains(t, missing, "undo.strategy")
	require.NotContains(t, missing, "problem",
		"the descriptive blocks default for every category")
}

func TestSignatureLeavesStatefulInternalSideEffectsRequired(t *testing.T) {
	t.Parallel()
	missing := missingToolContractFields(signedDeclaration("stateful_internal"))

	require.Contains(t, missing, "side_effects")
	require.NotContains(t, missing, "reversibility.classification")
	require.NotContains(t, missing, "undo.strategy")
}

func TestSignatureSuppliesEmitsAndOutputSchema(t *testing.T) {
	t.Parallel()
	require.NotContains(t, missingToolContractFields(signedDeclaration("word")), "emits")
	require.NotContains(t, missingToolContractFields(signedDeclaration("word")), "output.schema")

	bare := signedDeclaration("word")
	bare.Signature.Emits = nil
	bare.Signature.Output = ""
	missing := missingToolContractFields(bare)
	require.Contains(t, missing, "emits")
	require.Contains(t, missing, "output.schema")
}

func TestUnsignedDeclarationStillNeedsEveryBlock(t *testing.T) {
	t.Parallel()
	unsigned := signedDeclaration("word")
	unsigned.Signature = nil

	missing := missingToolContractFields(unsigned)

	for _, field := range []string{
		"problem", "goals", "non_goals", "emits", "output.schema",
		"side_effects", "reversibility.classification", "undo.strategy",
	} {
		require.Containsf(t, missing, field, "unsigned declarations are unchanged: %s", field)
	}
}

// TestToolDeclarationCarriesTheSignature guards the mapping the completeness
// check reads. A signature that stops at the audit model's door leaves a signed
// word looking like one with no contract at all.
func TestToolDeclarationCarriesTheSignature(t *testing.T) {
	t.Parallel()
	declaration := toolDeclarationFromDef(catalog.ToolDef{
		Name: "word_tool", Type: "builtin", Category: "word",
		Description: "Flatten retrieved chunks into rows.",
		Signature: &catalog.ToolSignature{
			Output: "chat-types.Rows", Emits: []string{"Flattened"},
		},
	})

	require.NotNil(t, declaration.Signature)
	require.Empty(t, missingToolContractFields(declaration))
}

// srd051 R6.14: a boundary tool is complete with a signature naming only its
// signals, its output schema, and the three explicit effect blocks. The
// signature discharges the prose; it never discharges an effect block.
func TestEmitsOnlySignatureCompletesABoundaryToolWithItsEffectBlocks(t *testing.T) {
	t.Parallel()
	complete := ToolDeclaration{
		Name: "read_counts", Type: "exec", Category: "boundary",
		Signature:     &catalog.ToolSignature{Emits: []string{"ToolDone", "ToolFailed"}},
		Output:        ToolDeclOutput{Schema: map[string]any{"type": "object"}},
		SideEffects:   ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "child_process"}}},
		Reversibility: ToolDeclReversibility{Classification: "reversible"},
		Undo:          ToolDeclUndo{Strategy: "noop"},
	}
	require.Empty(t, missingToolContractFields(complete))

	for field, strip := range map[string]func(*ToolDeclaration){
		"side_effects":                 func(d *ToolDeclaration) { d.SideEffects = ToolDeclSideEffects{} },
		"reversibility.classification": func(d *ToolDeclaration) { d.Reversibility = ToolDeclReversibility{} },
		"undo.strategy":                func(d *ToolDeclaration) { d.Undo = ToolDeclUndo{} },
	} {
		missingBlock := complete
		strip(&missingBlock)
		require.Equal(t, []string{field}, missingToolContractFields(missingBlock))
	}
}
