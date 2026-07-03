// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	toolundo "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
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

func EncodeBoundaryReceipt(payload BoundaryCompensationPayload) string {
	return toolundo.EncodeBoundaryReceipt(payload)
}

func DecodeBoundaryReceipt(receipt string) (BoundaryCompensation, bool, error) {
	return toolundo.DecodeBoundaryReceipt(receipt)
}
