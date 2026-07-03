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
	Strategy           string                 `json:"strategy"`
	Reason             string                 `json:"reason,omitempty"`
	Requires           []string               `json:"requires,omitempty"`
	WorkspacePaths     []string               `json:"workspace_paths,omitempty"`
	ArtifactPaths      []string               `json:"artifact_paths,omitempty"`
	ChildProfile       string                 `json:"child_profile,omitempty"`
	ChildMachine       string                 `json:"child_machine,omitempty"`
	ChildTools         string                 `json:"child_tools,omitempty"`
	ChildRunID         string                 `json:"child_run_id,omitempty"`
	ServerAddr         string                 `json:"server_addr,omitempty"`
	UserAction         string                 `json:"user_action,omitempty"`
	IssueID            string                 `json:"issue_id,omitempty"`
	CheckpointRequired bool                   `json:"checkpoint_required,omitempty"`
	RestRef            string                 `json:"rest_ref,omitempty"`
	Resource           string                 `json:"resource,omitempty"`
	Operation          string                 `json:"operation,omitempty"`
	Parameters         map[string]interface{} `json:"parameters,omitempty"`
	ResourceID         string                 `json:"resource_id,omitempty"`
	RequestID          string                 `json:"request_id,omitempty"`
	IdempotencyToken   string                 `json:"idempotency_token,omitempty"`
	Compensation       map[string]interface{} `json:"compensation,omitempty"`
}

// BoundaryCompensationMemento creates a compensatable undo memento.
func BoundaryCompensationMemento(commandName string, payload BoundaryCompensationPayload, description string) (core.UndoMemento, error) {
	if payload.BoundaryCompensation.Strategy == "" {
		return core.UndoMemento{}, fmt.Errorf("%w: missing boundary compensation strategy for %s", core.ErrUndoMementoIncompatible, commandName)
	}
	memento, err := core.NewUndoMemento(commandName, core.UndoMementoCompensatable, payload)
	if err != nil {
		return core.UndoMemento{}, err
	}
	memento.Description = description
	return memento, nil
}

// BoundaryCompensationUndo reports that a boundary compensation is required.
func BoundaryCompensationUndo(commandName, description string) core.Result {
	err := fmt.Errorf("undo %s requires boundary compensation: %s", commandName, description)
	return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
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

// DecodeBoundaryCompensation decodes a boundary compensation undo memento.
func DecodeBoundaryCompensation(memento core.UndoMemento) (BoundaryCompensation, error) {
	if err := core.ValidateUndoMemento(memento); err != nil {
		return BoundaryCompensation{}, err
	}
	var payload BoundaryCompensationPayload
	if err := json.Unmarshal(memento.Payload, &payload); err != nil {
		return BoundaryCompensation{}, fmt.Errorf("%w: decode boundary compensation for %s: %v", core.ErrUndoMementoIncompatible, memento.CommandName, err)
	}
	if payload.BoundaryCompensation.Strategy == "" {
		return BoundaryCompensation{}, fmt.Errorf("%w: missing boundary compensation for %s", core.ErrUndoMementoIncompatible, memento.CommandName)
	}
	return payload.BoundaryCompensation, nil
}
