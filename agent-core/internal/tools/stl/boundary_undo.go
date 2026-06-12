// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// BoundaryCompensationPayload is the shared rollback payload for tools that
// cross a boundary: child agents, nested machines, UI/human approval, or
// external issue/workspace systems.
type BoundaryCompensationPayload struct {
	BoundaryCompensation BoundaryCompensation `json:"boundary_compensation"`
}

type BoundaryCompensation struct {
	Strategy           string   `json:"strategy"`
	Reason             string   `json:"reason,omitempty"`
	Requires           []string `json:"requires,omitempty"`
	WorkspacePaths     []string `json:"workspace_paths,omitempty"`
	ArtifactPaths      []string `json:"artifact_paths,omitempty"`
	ChildMachine       string   `json:"child_machine,omitempty"`
	ChildTools         string   `json:"child_tools,omitempty"`
	ChildRunID         string   `json:"child_run_id,omitempty"`
	ServerAddr         string   `json:"server_addr,omitempty"`
	UserAction         string   `json:"user_action,omitempty"`
	IssueID            string   `json:"issue_id,omitempty"`
	CheckpointRequired bool     `json:"checkpoint_required,omitempty"`
}

func boundaryCompensationMemento(commandName string, payload BoundaryCompensationPayload, description string) (core.UndoMemento, error) {
	return BoundaryCompensationMemento(commandName, payload, description)
}

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

func boundaryCompensationUndo(commandName, description string) core.Result {
	return BoundaryCompensationUndo(commandName, description)
}

func BoundaryCompensationUndo(commandName, description string) core.Result {
	err := fmt.Errorf("undo %s requires boundary compensation: %s", commandName, description)
	return core.Result{
		Signal:      core.CommandError,
		CommandName: commandName,
		Output:      err.Error(),
		Err:         err,
	}
}
