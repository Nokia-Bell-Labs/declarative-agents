// Copyright (c) 2026 Nokia. All rights reserved.

package undo

import (
	"encoding/json"
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
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
