// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"errors"
	"fmt"
)

const UndoMementoVersion = 1

// UndoMementoKind classifies how rollback can treat a command.
type UndoMementoKind string

const (
	UndoMementoNoop          UndoMementoKind = "noop"
	UndoMementoReversible    UndoMementoKind = "reversible"
	UndoMementoCompensatable UndoMementoKind = "compensatable"
	UndoMementoIrreversible  UndoMementoKind = "irreversible"
)

var (
	ErrUndoMementoMissing      = errors.New("undo memento missing")
	ErrUndoMementoIncompatible = errors.New("undo memento incompatible")
)

// UndoMemento is the versioned JSON contract commands use to describe how a
// completed dispatch can be undone without serializing the live Command object.
type UndoMemento struct {
	Version     int             `json:"version"`
	Kind        UndoMementoKind `json:"kind"`
	CommandName string          `json:"command_name"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	Description string          `json:"description,omitempty"`
}

// UndoMementoProvider is implemented by commands that can expose a serializable
// rollback snapshot for checkpoint history.
type UndoMementoProvider interface {
	UndoMemento() (UndoMemento, error)
}

func NoopUndoMemento(commandName string) UndoMemento {
	return UndoMemento{
		Version:     UndoMementoVersion,
		Kind:        UndoMementoNoop,
		CommandName: commandName,
		Description: "command has no rollback-managed state",
	}
}

func IrreversibleUndoMemento(commandName, reason string) UndoMemento {
	return UndoMemento{
		Version:     UndoMementoVersion,
		Kind:        UndoMementoIrreversible,
		CommandName: commandName,
		Description: reason,
	}
}

func NewUndoMemento(commandName string, kind UndoMementoKind, payload any) (UndoMemento, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return UndoMemento{}, fmt.Errorf("%w: marshal payload for %s: %v", ErrUndoMementoIncompatible, commandName, err)
	}
	memento := UndoMemento{
		Version:     UndoMementoVersion,
		Kind:        kind,
		CommandName: commandName,
		Payload:     data,
	}
	if err := ValidateUndoMemento(memento); err != nil {
		return UndoMemento{}, err
	}
	return memento, nil
}

func ValidateUndoMemento(memento UndoMemento) error {
	if memento.Version == 0 && memento.Kind == "" && memento.CommandName == "" && len(memento.Payload) == 0 && memento.Description == "" {
		return ErrUndoMementoMissing
	}
	if memento.Version != UndoMementoVersion {
		return fmt.Errorf("%w: unsupported version %d", ErrUndoMementoIncompatible, memento.Version)
	}
	if memento.CommandName == "" {
		return fmt.Errorf("%w: missing command_name", ErrUndoMementoIncompatible)
	}
	switch memento.Kind {
	case UndoMementoNoop, UndoMementoIrreversible:
		return nil
	case UndoMementoReversible, UndoMementoCompensatable:
		if len(memento.Payload) == 0 {
			return fmt.Errorf("%w: %s memento for %s requires payload", ErrUndoMementoMissing, memento.Kind, memento.CommandName)
		}
		if !json.Valid(memento.Payload) {
			return fmt.Errorf("%w: invalid payload JSON for %s", ErrUndoMementoIncompatible, memento.CommandName)
		}
		return nil
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrUndoMementoIncompatible, memento.Kind)
	}
}

func cloneUndoMemento(memento *UndoMemento) *UndoMemento {
	if memento == nil {
		return nil
	}
	clone := *memento
	clone.Payload = append(json.RawMessage(nil), memento.Payload...)
	return &clone
}
