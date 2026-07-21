// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func reversibleWriteDef() ToolDef {
	return ToolDef{
		Name:          "write",
		Type:          "builtin",
		Reversibility: ToolReversibility{Classification: "reversible", Undo: "workspace_restore"},
		SideEffects:   ToolSideEffects{Items: []ToolSideEffect{{Kind: "filesystem_write"}}},
	}
}

func TestValidateReceiptPresenceFailsReversibleToolWithoutReceipt(t *testing.T) {
	t.Parallel()

	finding := ValidateReceiptPresence(reversibleWriteDef(), core.Result{Signal: core.ToolDone})

	require.NotEmpty(t, finding.ToolName)
	assert.Equal(t, "write", finding.ToolName)
	assert.Equal(t, "receipt", finding.Field)
	assert.Equal(t, ContractSeverityError, finding.Severity)
	assert.Contains(t, finding.Message, "without an opaque receipt")
}

func TestValidateReceiptPresencePassesWhenReceiptPresent(t *testing.T) {
	t.Parallel()

	finding := ValidateReceiptPresence(reversibleWriteDef(),
		core.Result{Signal: core.ToolDone, Receipt: `{"path":"a.txt"}`})

	assert.Empty(t, finding.ToolName)
}

func TestValidateReceiptPresenceIgnoresFailedResults(t *testing.T) {
	t.Parallel()

	finding := ValidateReceiptPresence(reversibleWriteDef(),
		core.Result{Signal: core.CommandError, Err: assertErr{}})

	assert.Empty(t, finding.ToolName)
}

func TestValidateReceiptPresenceIgnoresReadOnlyReversibleTool(t *testing.T) {
	t.Parallel()

	def := ToolDef{
		Name:          "find",
		Reversibility: ToolReversibility{Classification: "reversible", Undo: "noop"},
		SideEffects:   ToolSideEffects{Items: []ToolSideEffect{{Kind: "filesystem_read"}}},
	}

	finding := ValidateReceiptPresence(def, core.Result{Signal: core.ToolDone})

	assert.Empty(t, finding.ToolName)
}

func TestValidateReceiptPresenceIgnoresIrreversibleTool(t *testing.T) {
	t.Parallel()

	def := ToolDef{
		Name:          "shutdown",
		Reversibility: ToolReversibility{Classification: "irreversible"},
		SideEffects:   ToolSideEffects{Items: []ToolSideEffect{{Kind: "state_mutation"}}},
	}

	finding := ValidateReceiptPresence(def, core.Result{Signal: core.ToolDone})

	assert.Empty(t, finding.ToolName)
}

type assertErr struct{}

func (assertErr) Error() string { return "boom" }

func TestValidateReceiptContractPassesReceiptBackedTool(t *testing.T) {
	t.Parallel()

	finding := ValidateReceiptContract(reversibleWriteDef())

	assert.Empty(t, finding.ToolName)
}

func TestValidateReceiptContractFailsMutatingReversibleWithNoopUndo(t *testing.T) {
	t.Parallel()

	def := ToolDef{
		Name:          "write",
		Reversibility: ToolReversibility{Classification: "reversible", Undo: "noop"},
		SideEffects:   ToolSideEffects{Items: []ToolSideEffect{{Kind: "filesystem_write"}}},
	}

	finding := ValidateReceiptContract(def)

	require.NotEmpty(t, finding.ToolName)
	assert.Equal(t, "undo", finding.Field)
	assert.Equal(t, ContractSeverityError, finding.Severity)
	assert.Contains(t, finding.Message, "no receipt-consuming undo")
}

func TestValidateReceiptContractIgnoresReadOnlyReversibleTool(t *testing.T) {
	t.Parallel()

	def := ToolDef{
		Name:          "find",
		Reversibility: ToolReversibility{Classification: "reversible", Undo: "noop"},
		SideEffects:   ToolSideEffects{Items: []ToolSideEffect{{Kind: "filesystem_read"}}},
	}

	finding := ValidateReceiptContract(def)

	assert.Empty(t, finding.ToolName)
}

func TestValidateReceiptContractsAggregatesSelectedTools(t *testing.T) {
	t.Parallel()

	bad := ToolDef{
		Name:          "rest_set_issue",
		Reversibility: ToolReversibility{Classification: "compensatable"},
		SideEffects:   ToolSideEffects{Items: []ToolSideEffect{{Kind: "resource_mutation"}}},
	}

	// A good selection passes.
	require.NoError(t, ValidateReceiptContracts([]ToolDef{reversibleWriteDef()}))

	// A selection containing an invalid reversible declaration fails, naming it.
	err := ValidateReceiptContracts([]ToolDef{reversibleWriteDef(), bad})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rest_set_issue")
	assert.Contains(t, err.Error(), "no receipt-consuming undo")
}
