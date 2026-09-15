// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func signedDeclaration(category string) ToolDeclaration {
	return ToolDeclaration{
		Name: "word_tool", Type: "builtin", Category: category,
		Signature: &ToolDeclSignature{
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
