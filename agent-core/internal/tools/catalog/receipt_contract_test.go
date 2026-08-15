// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

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
		Reversibility: ToolReversibility{Classification: "reversible"},
		Undo:          ToolUndoContract{Strategy: "workspace_restore"},
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
		Reversibility: ToolReversibility{Classification: "reversible"},
		Undo:          ToolUndoContract{Strategy: "noop"},
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
		Reversibility: ToolReversibility{Classification: "reversible"},
		Undo:          ToolUndoContract{Strategy: "noop"},
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
		Reversibility: ToolReversibility{Classification: "reversible"},
		Undo:          ToolUndoContract{Strategy: "noop"},
		SideEffects:   ToolSideEffects{Items: []ToolSideEffect{{Kind: "filesystem_read"}}},
	}

	finding := ValidateReceiptContract(def)

	assert.Empty(t, finding.ToolName)
}

func TestValidateReceiptContractIgnoresReadOnlyStateThroughMutatingKind(t *testing.T) {
	t.Parallel()

	// A read through a kind that is not itself whitelisted (an external_api GET, a
	// child_process status read) mutates nothing when it declares state: read_only,
	// so it needs no rollback receipt.
	for _, kind := range []string{"external_api", "child_process"} {
		def := ToolDef{
			Name:          "read_" + kind,
			Reversibility: ToolReversibility{Classification: "reversible"},
			Undo:          ToolUndoContract{Strategy: "noop"},
			SideEffects:   ToolSideEffects{Items: []ToolSideEffect{{Kind: kind, State: "read_only"}}},
		}

		finding := ValidateReceiptContract(def)

		assert.Empty(t, finding.ToolName, "kind %q with state read_only should not trip the contract", kind)
	}
}

func TestValidateReceiptContractIgnoresProcessLocalListenerShutdown(t *testing.T) {
	t.Parallel()

	// Stopping a network listener is process-local; it does not persist across a
	// restart, so a reversible server-lifecycle word producing it needs no receipt.
	def := ToolDef{
		Name:          "stop_server",
		Reversibility: ToolReversibility{Classification: "reversible"},
		Undo:          ToolUndoContract{Strategy: "noop"},
		SideEffects:   ToolSideEffects{Items: []ToolSideEffect{{Kind: "network_listener_shutdown", State: "listener_stopped"}}},
	}

	finding := ValidateReceiptContract(def)

	assert.Empty(t, finding.ToolName)
}

func TestValidateReceiptContractFailsMutatingStateThroughSameKind(t *testing.T) {
	t.Parallel()

	// The same kind without a read_only state is still a mutation needing a receipt.
	def := ToolDef{
		Name:          "write_external",
		Reversibility: ToolReversibility{Classification: "reversible"},
		Undo:          ToolUndoContract{Strategy: "noop"},
		SideEffects:   ToolSideEffects{Items: []ToolSideEffect{{Kind: "external_api", State: "records_added"}}},
	}

	finding := ValidateReceiptContract(def)

	require.NotEmpty(t, finding.ToolName)
	assert.Contains(t, finding.Message, "no receipt-consuming undo")
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

type receiptGuardBuilder struct {
	name       string
	returnsNil bool
}

func (b receiptGuardBuilder) Build(core.Result) core.Command {
	return receiptGuardCommand{name: b.name}
}

func (b receiptGuardBuilder) BuildReverser() core.Command {
	if b.returnsNil {
		return nil
	}
	return receiptGuardCommand{name: b.name}
}

type receiptGuardCommand struct{ name string }

func (c receiptGuardCommand) Name() string                 { return c.name }
func (c receiptGuardCommand) Execute() core.Result         { return core.Result{Signal: core.ToolDone} }
func (c receiptGuardCommand) Undo(core.Result) core.Result { return core.Result{Signal: core.ToolDone} }

type receiptGuardNonReverser struct{ name string }

func (b receiptGuardNonReverser) Build(core.Result) core.Command {
	return receiptGuardCommand(b)
}

func TestValidateReceiptFamiliesGuardsSelectedMutatorPlumbing(t *testing.T) {
	t.Parallel()

	good := reversibleWriteDef()
	good.Name = "persist_alias"
	noBuilder := reversibleWriteDef()
	noBuilder.Name = "missing_alias"
	noReverser := reversibleWriteDef()
	noReverser.Name = "plain_alias"
	nilReverser := reversibleWriteDef()
	nilReverser.Name = "nil_alias"
	wrongAlias := reversibleWriteDef()
	wrongAlias.Name = "declared_alias"

	registry := core.NewRegistry()
	registry.Register(good.ToToolSpec(), receiptGuardBuilder{name: good.Name})
	registry.Register(noReverser.ToToolSpec(), receiptGuardNonReverser{name: noReverser.Name})
	registry.Register(nilReverser.ToToolSpec(), receiptGuardBuilder{name: nilReverser.Name, returnsNil: true})
	registry.Register(wrongAlias.ToToolSpec(), receiptGuardBuilder{name: "init_name"})

	err := ValidateReceiptFamilies([]ReceiptFamily{{
		Name: "representative",
		Definitions: []ToolDef{
			good, noBuilder, noReverser, nilReverser, wrongAlias,
		},
	}}, registry)

	require.Error(t, err)
	for _, want := range []string{
		`family "representative" declaration "missing_alias": no builder registered`,
		`family "representative" declaration "plain_alias": builder does not implement Reverser`,
		`family "representative" declaration "nil_alias": BuildReverser returned nil`,
		`family "representative" declaration "declared_alias": fresh reverser name "init_name" does not preserve declaration alias`,
	} {
		assert.Contains(t, err.Error(), want)
	}
	assert.NotContains(t, err.Error(), good.Name)
}

func TestValidateReceiptFamiliesAllowsReceiptlessNoopAndIrreversible(t *testing.T) {
	t.Parallel()

	noop := ToolDef{
		Name:          "observe",
		Reversibility: ToolReversibility{Classification: "reversible"},
		Undo:          ToolUndoContract{Strategy: "noop"},
		SideEffects:   ToolSideEffects{Items: []ToolSideEffect{{Kind: "state_read"}}},
	}
	irreversible := ToolDef{
		Name:          "publish",
		Reversibility: ToolReversibility{Classification: "irreversible"},
		Undo:          ToolUndoContract{Strategy: "irreversible"},
		SideEffects:   ToolSideEffects{Items: []ToolSideEffect{{Kind: "external_api", State: "published"}}},
	}

	require.NoError(t, ValidateReceiptFamilies([]ReceiptFamily{{
		Name: "controls", Definitions: []ToolDef{noop, irreversible},
	}}, nil))
}
