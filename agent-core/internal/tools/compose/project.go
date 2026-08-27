// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package compose

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// ProjectBuilder extracts one configured field from every selected array item.
type ProjectBuilder struct {
	ToolName string
	Items    string
	Field    string
	Signal   core.Signal
}

// ValidateProjectConfig checks the command-state selector, local item path, and
// success signal before the word can be registered.
func ValidateProjectConfig(toolName, items, field, signal string) error {
	if err := validateArraySelector(toolName, "items", items); err != nil {
		return err
	}
	if field == "" {
		return fmt.Errorf("tool %q: config requires field", toolName)
	}
	if err := validateItemPath(toolName, "field", "value", field); err != nil {
		return err
	}
	if signal == "" {
		return fmt.Errorf("tool %q: config requires signal", toolName)
	}
	return nil
}

func (b ProjectBuilder) Build(_ core.Result) core.Command {
	return &projectCmd{
		name: b.ToolName, items: b.Items, field: b.Field, signal: b.Signal,
	}
}

type projectCmd struct {
	name, items, field string
	signal             core.Signal
	view               core.CommandStateView
}

func (c *projectCmd) Name() string                            { return c.name }
func (c *projectCmd) SetCommandState(v core.CommandStateView) { c.view = v }
func (c *projectCmd) Undo(_ core.Result) core.Result          { return core.NoopUndo(c.Name()) }

var _ core.CommandStateAware = (*projectCmd)(nil)

func (c *projectCmd) Execute() core.Result {
	items, err := selectedArray(c.view, c.items)
	if err != nil {
		return transformFault(c.Name(), err)
	}
	projected := make([]interface{}, 0, len(items))
	for index, item := range items {
		value, err := resolveItemPath(item, c.field)
		if err != nil {
			return transformFault(c.Name(), fmt.Errorf("items[%d]: %w", index, err))
		}
		projected = append(projected, value)
	}
	return transformResult(c.Name(), c.signal, map[string]interface{}{
		"items": projected,
		"count": len(projected),
	})
}
