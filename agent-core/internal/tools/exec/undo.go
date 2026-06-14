// Copyright (c) 2026 Nokia. All rights reserved.

package exec

import (
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
)

type workspaceUndoPayload struct {
	WorkspaceRestore struct {
		Paths []string `json:"paths"`
	} `json:"workspace_restore"`
}

type boundaryCompensationPayload struct {
	BoundaryCompensation boundaryCompensation `json:"boundary_compensation"`
}

type boundaryCompensation struct {
	Strategy       string   `json:"strategy"`
	Reason         string   `json:"reason,omitempty"`
	Requires       []string `json:"requires,omitempty"`
	WorkspacePaths []string `json:"workspace_paths,omitempty"`
	IssueID        string   `json:"issue_id,omitempty"`
}

func (c *ExecCmd) UndoMemento() (core.UndoMemento, error) {
	switch c.def.Undo.Strategy {
	case "", "noop":
		return core.NoopUndoMemento(c.Name()), nil
	case "workspace_restore":
		return core.NewUndoMemento(c.Name(), core.UndoMementoReversible, workspaceRestorePayload(c.def))
	case "compensating_action":
		return c.compensatingUndoMemento()
	default:
		err := fmt.Errorf("unsupported undo strategy %q for %s", c.def.Undo.Strategy, c.Name())
		return core.UndoMemento{}, fmt.Errorf("%w: %s", core.ErrUndoMementoIncompatible, err)
	}
}

func (c *ExecCmd) compensatingUndoMemento() (core.UndoMemento, error) {
	payload := any(workspaceRestorePayload(c.def))
	if c.def.Undo.Payload == "boundary_compensation" {
		payload = execBoundaryCompensationPayload(c.def, c.params)
	}
	memento, err := core.NewUndoMemento(c.Name(), core.UndoMementoCompensatable, payload)
	if err != nil {
		return core.UndoMemento{}, err
	}
	memento.Description = c.def.Undo.Description
	return memento, nil
}

func execBoundaryCompensationPayload(def catalog.ToolDef, params map[string]string) boundaryCompensationPayload {
	workspacePayload := workspaceRestorePayload(def)
	compensation := boundaryCompensation{
		Strategy:       def.Undo.Strategy,
		Reason:         def.Undo.Description,
		Requires:       append([]string(nil), def.Undo.Requires...),
		WorkspacePaths: append([]string(nil), workspacePayload.WorkspaceRestore.Paths...),
		IssueID:        params["id"],
	}
	return boundaryCompensationPayload{BoundaryCompensation: compensation}
}

func workspaceRestorePayload(def catalog.ToolDef) workspaceUndoPayload {
	payload := workspaceUndoPayload{}
	for _, effect := range def.SideEffects.Items {
		payload.WorkspaceRestore.Paths = append(payload.WorkspaceRestore.Paths, effect.Paths...)
	}
	if len(payload.WorkspaceRestore.Paths) == 0 {
		payload.WorkspaceRestore.Paths = []string{"."}
	}
	return payload
}

func compensationUndo(commandName, description string) core.Result {
	err := fmt.Errorf("undo %s requires compensating action: %s", commandName, description)
	return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
}
