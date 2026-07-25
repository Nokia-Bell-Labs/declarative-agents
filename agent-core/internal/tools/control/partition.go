// Copyright (c) 2026 Nokia. All rights reserved.

package control

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// PartitionBuilder constructs a command that divides an ordered array by one
// declared scalar comparison.
type PartitionBuilder struct {
	ToolName    string
	Items       string
	Field       string
	Op          string
	Right       string
	OperandType string
	Satisfied   core.Signal
}

func ValidatePartitionConfig(toolName, items, field, op, right, operandType, satisfied string) error {
	if _, _, ok := core.ParseFromSelector(items); !ok {
		return fmt.Errorf("tool %q: config items must be a $from(label).path selector", toolName)
	}
	if field == "" {
		return fmt.Errorf("tool %q: config requires field", toolName)
	}
	if parsed, ok := core.ParseSelector("$." + field); !ok || parsed.Label != "" {
		return fmt.Errorf("tool %q: config field %q is not a dotted path", toolName, field)
	}
	if !knownOps[op] {
		return fmt.Errorf("tool %q: unknown operator %q", toolName, op)
	}
	if !unaryOps[op] && right == "" {
		return fmt.Errorf("tool %q: operator %q needs a right operand", toolName, op)
	}
	if strings.HasPrefix(right, fromPrefix) {
		if _, _, ok := core.ParseFromSelector(right); !ok {
			return fmt.Errorf("tool %q: right %q is not a valid $from(label).path selector", toolName, right)
		}
	}
	switch operandType {
	case "", OperandNumber, OperandString:
	default:
		return fmt.Errorf("tool %q: unknown operand type %q", toolName, operandType)
	}
	if satisfied == "" {
		return fmt.Errorf("tool %q: config requires satisfied signal", toolName)
	}
	return nil
}

func (b PartitionBuilder) Build(_ core.Result) core.Command {
	operandType := b.OperandType
	if operandType == "" {
		operandType = OperandNumber
	}
	return &partitionCmd{
		name: b.ToolName, items: b.Items, field: b.Field, op: b.Op,
		right: b.Right, operandType: operandType, satisfied: b.Satisfied,
	}
}

type partitionCmd struct {
	name, items, field, op, right, operandType string
	satisfied                                  core.Signal
	view                                       core.CommandStateView
}

func (c *partitionCmd) Name() string                            { return c.name }
func (c *partitionCmd) SetCommandState(v core.CommandStateView) { c.view = v }
func (c *partitionCmd) Undo(_ core.Result) core.Result          { return core.NoopUndo(c.Name()) }

var _ core.CommandStateAware = (*partitionCmd)(nil)

func (c *partitionCmd) Execute() core.Result {
	items, right, err := c.resolveInputs()
	if err != nil {
		return c.fault(err)
	}
	matched, unmatched, err := c.split(items, right)
	if err != nil {
		return c.fault(err)
	}
	output, err := json.Marshal(map[string]interface{}{
		"matched": matched, "unmatched": unmatched,
		"matched_count": len(matched), "unmatched_count": len(unmatched),
	})
	if err != nil {
		return c.fault(fmt.Errorf("encode output: %w", err))
	}
	return core.Result{Signal: c.satisfied, CommandName: c.Name(), Output: string(output)}
}

func (c *partitionCmd) resolveInputs() ([]interface{}, interface{}, error) {
	value, err := core.ResolveFromSelector(c.view, c.items)
	if err != nil {
		return nil, nil, err
	}
	items, ok := value.([]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("items selector %q resolved to %T, want array", c.items, value)
	}
	var right interface{} = c.right
	if strings.HasPrefix(c.right, fromPrefix) {
		right, err = core.ResolveFromSelector(c.view, c.right)
		if err != nil {
			return nil, nil, err
		}
	}
	return items, right, nil
}

func (c *partitionCmd) split(items []interface{}, right interface{}) ([]interface{}, []interface{}, error) {
	matched := make([]interface{}, 0, len(items))
	unmatched := make([]interface{}, 0, len(items))
	for i, item := range items {
		left, err := fieldValue(item, c.field)
		if err != nil {
			return nil, nil, fmt.Errorf("items[%d]: %w", i, err)
		}
		held := false
		if unaryOps[c.op] {
			held = isEmpty(left) == (c.op == OpEmpty)
		} else {
			held, err = compareValues(c.op, c.operandType, left, right)
			if err != nil {
				return nil, nil, fmt.Errorf("items[%d]: %w", i, err)
			}
		}
		if held {
			matched = append(matched, item)
		} else {
			unmatched = append(unmatched, item)
		}
	}
	return matched, unmatched, nil
}

func fieldValue(item interface{}, field string) (interface{}, error) {
	current := item
	for _, component := range strings.Split(field, ".") {
		object, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("field %q cannot be read from %T", field, current)
		}
		current, ok = object[component]
		if !ok {
			return nil, fmt.Errorf("field %q is absent", field)
		}
	}
	return current, nil
}

func (c *partitionCmd) fault(err error) core.Result {
	wrapped := fmt.Errorf("%s: %w", c.Name(), err)
	return core.Result{Signal: core.CommandError, CommandName: c.Name(), Output: wrapped.Error(), Err: wrapped}
}
