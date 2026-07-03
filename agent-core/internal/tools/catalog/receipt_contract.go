// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// nonMutatingSideEffectKinds are side-effect kinds that observe but do not mutate
// world state, so a reversible tool producing only these needs no rollback receipt.
var nonMutatingSideEffectKinds = map[string]bool{
	"":                true,
	"none":            true,
	"filesystem_read": true,
	"state_read":      true,
	"stdout":          true,
	"stderr_write":    true,
	"human_boundary":  true,
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
		if !nonMutatingSideEffectKinds[effect.Kind] {
			return true
		}
	}
	return false
}
