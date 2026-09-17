// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestValidate_SelectedToolContractCompletenessErrorsForActiveMigratedTool(t *testing.T) {
	corpus := &Corpus{
		ToolSelections: map[string][]string{
			"agent": {"sparse"},
		},
		ToolDeclarations: map[string]ToolDeclaration{
			"sparse": {
				Name:     "sparse",
				Contract: "migrated",
				Emits:    []string{"ToolDone"},
			},
		},
	}

	findings := checkSelectedToolContractCompleteness(corpus)

	require.Len(t, findings, 1)
	assert.Equal(t, "tool-contract-incomplete", findings[0].Check)
	assert.Equal(t, "error", findings[0].Level)
	assert.Contains(t, findings[0].Message, "sparse")
	assert.Contains(t, findings[0].Message, "problem")
	assert.Contains(t, findings[0].Message, "output.schema")
}

func TestValidate_SelectedToolContractCompletenessKeepsLegacyWarningOnly(t *testing.T) {
	corpus := &Corpus{
		ToolSelections: map[string][]string{
			"agent": {"legacy"},
		},
		ToolDeclarations: map[string]ToolDeclaration{
			"legacy": {
				Name:     "legacy",
				Contract: "legacy",
				Emits:    []string{"ToolDone"},
			},
		},
	}

	findings := checkSelectedToolContractCompleteness(corpus)

	require.Len(t, findings, 1)
	assert.Equal(t, "warning", findings[0].Level)
	assert.Contains(t, findings[0].Message, "legacy")
}

func TestValidate_SelectedToolContractCompletenessIgnoresUnselectedTool(t *testing.T) {
	corpus := &Corpus{
		ToolSelections: map[string][]string{
			"agent": {"complete"},
		},
		ToolDeclarations: map[string]ToolDeclaration{
			"complete": completeToolDeclaration("complete"),
			"sparse":   {Name: "sparse"},
		},
	}

	findings := checkSelectedToolContractCompleteness(corpus)

	require.Empty(t, findings)
}

func TestValidate_SelectedToolContractCompletenessPassesCompleteTool(t *testing.T) {
	corpus := &Corpus{
		ToolSelections: map[string][]string{
			"agent":       {"complete"},
			"agent:point": {"complete"},
		},
		ToolDeclarations: map[string]ToolDeclaration{
			"complete": completeToolDeclaration("complete"),
		},
	}

	findings := checkSelectedToolContractCompleteness(corpus)

	require.Empty(t, findings)
}

// TestValidate_SelectedWordMustDeclareASignature covers the promotion in
// GH-2009. A word whose prose contract is complete is still incomplete without
// a signature: nothing states the type it returns, so the label it publishes
// stays untyped and every selector into it stays unchecked.
func TestValidate_SelectedWordMustDeclareASignature(t *testing.T) {
	unsigned := completeToolDeclaration("unsigned")
	unsigned.Signature = nil
	corpus := &Corpus{
		ToolSelections:   map[string][]string{"agent": {"unsigned"}},
		ToolDeclarations: map[string]ToolDeclaration{"unsigned": unsigned},
	}

	findings := checkSelectedToolContractCompleteness(corpus)

	require.Len(t, findings, 1)
	require.Contains(t, findings[0].Message, "signature")
}

// TestValidate_BoundaryWordMustDeclareASignature is srd051 R6.14 as promoted
// by GH-2150: a boundary or stateful_internal tool without a signature fails
// even with its prose complete, and one naming only its signals passes, because
// the output type stays optional (R6.12).
func TestValidate_BoundaryWordMustDeclareASignature(t *testing.T) {
	for _, category := range []string{"boundary", "stateful_internal"} {
		unsigned := completeToolDeclaration("boundary_word")
		unsigned.Signature = nil
		unsigned.Category = category
		corpus := &Corpus{
			ToolSelections:   map[string][]string{"agent": {"boundary_word"}},
			ToolDeclarations: map[string]ToolDeclaration{"boundary_word": unsigned},
		}
		findings := checkSelectedToolContractCompleteness(corpus)
		require.Len(t, findings, 1, category)
		require.Contains(t, findings[0].Message, "signature")

		signed := unsigned
		signed.Signature = &catalog.ToolSignature{Emits: unsigned.Emits}
		corpus.ToolDeclarations["boundary_word"] = signed
		require.Empty(t, checkSelectedToolContractCompleteness(corpus), category)
	}
}
