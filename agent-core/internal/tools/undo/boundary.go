// Copyright (c) 2026 Nokia. All rights reserved.

package undo

import (
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// BoundaryCompensationPayload is the shared rollback payload for boundary tools.
type BoundaryCompensationPayload struct {
	BoundaryCompensation BoundaryCompensation `json:"boundary_compensation"`
}

// BoundaryCompensation describes compensation data for boundary effects.
type BoundaryCompensation struct {
	Strategy string                 `json:"strategy"`
	Reason   string                 `json:"reason,omitempty"`
	Requires []string               `json:"requires,omitempty"`
	Data     map[string]interface{} `json:"data,omitempty"`
}

// BoundaryCompensationUndo reports that a boundary compensation is required.
func BoundaryCompensationUndo(commandName, description string) core.Result {
	return core.Result{
		Signal:      core.CompensationRequired,
		CommandName: commandName,
		Output:      fmt.Sprintf("undo %s requires boundary compensation: %s", commandName, description),
	}
}

// BoundaryCompensationResult returns a structured pending-compensation result.
// Lifecycle rollback can project the same strategy, requirements, and concrete
// boundary data into its pending_compensation report without interpreting the
// originating tool's opaque receipt.
func BoundaryCompensationResult(commandName string, compensation BoundaryCompensation) core.Result {
	output := EncodeBoundaryReceipt(BoundaryCompensationPayload{
		BoundaryCompensation: compensation,
	})
	if output == "" {
		return BoundaryCompensationUndo(commandName, compensation.Reason)
	}
	return core.Result{
		Signal:      core.CompensationRequired,
		CommandName: commandName,
		Output:      output,
	}
}

// EncodeBoundaryReceipt serializes a boundary compensation payload into an opaque,
// tool-owned receipt for Result.Receipt. It returns "" when there is no strategy
// (nothing to compensate), so read-only or non-compensatable results carry no
// receipt (srd035-checkpoint-port R3; #44 R2). The receipt is opaque to the engine
// and adapters; only the originating boundary tool decodes it.
func EncodeBoundaryReceipt(payload BoundaryCompensationPayload) string {
	if payload.BoundaryCompensation.Strategy == "" {
		return ""
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(b)
}

// DecodeBoundaryReceipt decodes a boundary compensation from an opaque receipt.
// The second return reports whether a compensatable payload was present.
func DecodeBoundaryReceipt(receipt string) (BoundaryCompensation, bool, error) {
	if receipt == "" {
		return BoundaryCompensation{}, false, nil
	}
	var payload BoundaryCompensationPayload
	if err := json.Unmarshal([]byte(receipt), &payload); err != nil {
		return BoundaryCompensation{}, false, fmt.Errorf("decode boundary compensation receipt: %w", err)
	}
	if payload.BoundaryCompensation.Strategy == "" {
		return BoundaryCompensation{}, false, nil
	}
	return payload.BoundaryCompensation, true, nil
}
