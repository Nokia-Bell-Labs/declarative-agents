// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	toolundo "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/undo"
)

type (
	BoundaryCompensationPayload = toolundo.BoundaryCompensationPayload
	BoundaryCompensation        = toolundo.BoundaryCompensation
)

func BoundaryCompensationMemento(commandName string, payload BoundaryCompensationPayload, description string) (core.UndoMemento, error) {
	return toolundo.BoundaryCompensationMemento(commandName, payload, description)
}

func BoundaryCompensationUndo(commandName, description string) core.Result {
	return toolundo.BoundaryCompensationUndo(commandName, description)
}
