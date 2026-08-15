// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// nonMutatingSideEffectKinds are side-effect kinds that observe but do not mutate
// world state, so a reversible tool producing only these needs no rollback receipt.
var nonMutatingSideEffectKinds = map[string]bool{
	"":                          true,
	"none":                      true,
	"filesystem_read":           true,
	"state_read":                true,
	"stdout":                    true,
	"stderr_write":              true,
	"human_boundary":            true,
	"network_listener_shutdown": true, // process-local; a stopped listener does not persist across a restart
}

// nonMutatingSideEffect reports whether a side effect observes but does not mutate
// world state. An effect is non-mutating when its kind is a read/observe kind, or
// when it explicitly declares state: read_only -- a read through any kind (an
// external_api GET, a child_process status read) mutates nothing to roll back, so
// it needs no rollback receipt regardless of the kind label.
func nonMutatingSideEffect(effect ToolSideEffect) bool {
	return nonMutatingSideEffectKinds[effect.Kind] || effect.State == "read_only"
}

// ValidateReceiptPresence reports an error finding when a tool declared reversible
// or compensatable with state-mutating side effects produces a successful result
// that carries no opaque Receipt. Rolling such an effect back after a process
// restart requires the tool to have encoded its rollback context into
// Result.Receipt during Execute (#44 R4; srd035-checkpoint-port R3).
//
// This checks presence, not sufficiency: whether the receipt actually restores
// the prior state is each tool's own round-trip test responsibility.
func ValidateReceiptPresence(def ToolDef, result core.Result) ContractFinding {
	if result.Err != nil || result.Signal == core.CommandError {
		return ContractFinding{}
	}
	if !declaresReversibleMutation(def) {
		return ContractFinding{}
	}
	if result.Receipt != "" {
		return ContractFinding{}
	}
	return ContractFinding{
		ToolName: def.Name,
		Field:    "receipt",
		Severity: ContractSeverityError,
		Category: contractCategory(def),
		Message: fmt.Sprintf("tool %q is declared %s but produced a state-mutating result without an opaque receipt",
			def.Name, def.Reversibility.Classification),
		Remediation: "encode the tool's rollback context into Result.Receipt during Execute so a fresh instance can reverse the effect via receipt-consuming Undo",
	}
}

func declaresReversibleMutation(def ToolDef) bool {
	switch def.Reversibility.Classification {
	case "reversible", "compensatable":
		return hasStateMutatingEffect(def)
	default:
		return false
	}
}

func hasStateMutatingEffect(def ToolDef) bool {
	for _, effect := range def.SideEffects.Items {
		if !nonMutatingSideEffect(effect) {
			return true
		}
	}
	return false
}

// ValidateReceiptContract is the static, declaration-time counterpart to
// ValidateReceiptPresence. It reports an error finding when a tool declared
// reversible or compensatable with state-mutating side effects declares no
// receipt-consuming undo, so its persisted effect could never be reversed after
// a restart (srd025 R3.5, R3.2). It is checked over ToolDefs at load/audit time,
// where no runtime Result is available, so a broken declaration fails validation
// before the tool ever runs.
func ValidateReceiptContract(def ToolDef) ContractFinding {
	if !declaresReversibleMutation(def) {
		return ContractFinding{}
	}
	if declaresReceiptConsumingUndo(def) {
		return ContractFinding{}
	}
	return ContractFinding{
		ToolName: def.Name,
		Field:    "undo",
		Severity: ContractSeverityError,
		Category: contractCategory(def),
		Message: fmt.Sprintf("tool %q is declared %s with state-mutating effects but declares no receipt-consuming undo; its persisted effect could not be reversed after a restart",
			def.Name, def.Reversibility.Classification),
		Remediation: "declare an undo strategy other than noop that consumes the tool's opaque receipt (srd025 R3.2), or reclassify the tool as irreversible",
	}
}

// declaresReceiptConsumingUndo reports whether the tool declares an undo that can
// reverse a persisted mutation. A missing or noop strategy cannot, so it is not
// a receipt-consuming undo.
func declaresReceiptConsumingUndo(def ToolDef) bool {
	strategy := def.Undo.Strategy
	return strategy != "" && strategy != "noop"
}

// ValidateReceiptContracts aggregates ValidateReceiptContract over the selected
// tools into a single error, for the canonical load/audit gate that fails a
// profile whose reversible tools cannot honor the receipt-driven rollback model
// (srd025 R3.5).
func ValidateReceiptContracts(defs []ToolDef) error {
	var msgs []string
	for _, def := range defs {
		if finding := ValidateReceiptContract(def); finding.ToolName != "" {
			msgs = append(msgs, finding.Message)
		}
	}
	if len(msgs) > 0 {
		return fmt.Errorf("receipt-contract validation failed: %s", strings.Join(msgs, "; "))
	}
	return nil
}

// ReceiptFamily groups selected declarations by their concrete builder family.
// The family name is included in every finding so a factory regression identifies
// both the registration boundary and the declaration alias that cannot roll back.
type ReceiptFamily struct {
	Name        string
	Definitions []ToolDef
}

// ValidateReceiptFamilies is the static builder-side counterpart to the
// declaration checks above. For each selected state-mutating declaration whose
// undo is non-noop, it verifies that registry reconstruction produced a builder,
// that the builder exposes a fresh Reverser, and that the fresh command retains
// the declaration alias.
//
// This deliberately does not execute commands or interpret receipts. Family
// round-trip tests remain responsible for proving that successful execution
// emits a sufficient receipt and that a fresh reverser consumes it (srd025 R3.5).
func ValidateReceiptFamilies(
	families []ReceiptFamily,
	resolver core.CommandResolver,
) error {
	var msgs []string
	for _, family := range families {
		for _, def := range family.Definitions {
			if !declaresReversibleMutation(def) ||
				!declaresReceiptConsumingUndo(def) {
				continue
			}
			if err := validateReceiptFamilyBuilder(family.Name, def, resolver); err != nil {
				msgs = append(msgs, err.Error())
			}
		}
	}
	if len(msgs) > 0 {
		return fmt.Errorf("receipt-family validation failed: %s", strings.Join(msgs, "; "))
	}
	return nil
}

func validateReceiptFamilyBuilder(
	family string,
	def ToolDef,
	resolver core.CommandResolver,
) error {
	prefix := fmt.Sprintf("family %q declaration %q", family, def.Name)
	if resolver == nil {
		return fmt.Errorf("%s: no registry", prefix)
	}
	builder, ok := resolver.Resolve(def.Name)
	if !ok {
		return fmt.Errorf("%s: no builder registered", prefix)
	}
	reverser, ok := builder.(core.Reverser)
	if !ok {
		return fmt.Errorf("%s: builder does not implement Reverser", prefix)
	}
	command := reverser.BuildReverser()
	if command == nil {
		return fmt.Errorf("%s: BuildReverser returned nil", prefix)
	}
	if command.Name() != def.Name {
		return fmt.Errorf(
			"%s: fresh reverser name %q does not preserve declaration alias",
			prefix, command.Name(),
		)
	}
	return nil
}
