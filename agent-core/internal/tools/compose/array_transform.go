// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package compose

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// FlatMapBuilder flattens parallel arrays from parent items into ordered rows.
type FlatMapBuilder struct {
	ToolName      string
	Items         string
	ElementFields map[string]string
	CarryFields   map[string]string
	Signal        core.Signal
}

// ReorderByIndexBuilder orders candidates by indexes from a selected row array.
type ReorderByIndexBuilder struct {
	ToolName   string
	Items      string
	Order      string
	IndexField string
	Signal     core.Signal
}

func ValidateFlatMapConfig(
	toolName, items string,
	elementFields, carryFields map[string]string,
	signal string,
) error {
	if err := validateArraySelector(toolName, "items", items); err != nil {
		return err
	}
	if len(elementFields) == 0 {
		return fmt.Errorf("tool %q: config requires element_fields", toolName)
	}
	if signal == "" {
		return fmt.Errorf("tool %q: config requires signal", toolName)
	}
	for _, output := range sortedFieldNames(elementFields) {
		if err := validateItemPath(toolName, "element_fields", output, elementFields[output]); err != nil {
			return err
		}
	}
	for _, output := range sortedFieldNames(carryFields) {
		if _, exists := elementFields[output]; exists {
			return fmt.Errorf("tool %q: output field %q occurs in both element_fields and carry_fields", toolName, output)
		}
		if err := validateItemPath(toolName, "carry_fields", output, carryFields[output]); err != nil {
			return err
		}
	}
	return nil
}

func ValidateReorderByIndexConfig(toolName, items, order, indexField, signal string) error {
	if err := validateArraySelector(toolName, "items", items); err != nil {
		return err
	}
	if err := validateArraySelector(toolName, "order", order); err != nil {
		return err
	}
	if err := validateItemPath(toolName, "index_field", "index", indexField); err != nil {
		return err
	}
	if signal == "" {
		return fmt.Errorf("tool %q: config requires signal", toolName)
	}
	return nil
}

func validateArraySelector(toolName, field, selector string) error {
	if _, _, ok := core.ParseFromSelector(selector); !ok {
		return fmt.Errorf("tool %q: config %s must be a $from(label).path selector", toolName, field)
	}
	return nil
}

func validateItemPath(toolName, configField, output, path string) error {
	parsed, ok := core.ParseSelector("$." + path)
	if !ok || parsed.Label != "" {
		return fmt.Errorf("tool %q: config %s[%q] path %q is not a dotted item path",
			toolName, configField, output, path)
	}
	return nil
}

func (b FlatMapBuilder) Build(_ core.Result) core.Command {
	return &flatMapCmd{
		name: b.ToolName, items: b.Items, elementFields: b.ElementFields,
		carryFields: b.CarryFields, signal: b.Signal,
	}
}

func (b ReorderByIndexBuilder) Build(_ core.Result) core.Command {
	return &reorderByIndexCmd{
		name: b.ToolName, items: b.Items, order: b.Order,
		indexField: b.IndexField, signal: b.Signal,
	}
}

type flatMapCmd struct {
	name          string
	items         string
	elementFields map[string]string
	carryFields   map[string]string
	signal        core.Signal
	view          core.CommandStateView
}

func (c *flatMapCmd) Name() string                            { return c.name }
func (c *flatMapCmd) SetCommandState(v core.CommandStateView) { c.view = v }
func (c *flatMapCmd) Undo(_ core.Result) core.Result          { return core.NoopUndo(c.Name()) }

var _ core.CommandStateAware = (*flatMapCmd)(nil)

func (c *flatMapCmd) Execute() core.Result {
	parents, err := selectedArray(c.view, c.items)
	if err != nil {
		return transformFault(c.Name(), err)
	}
	rows := make([]interface{}, 0)
	for parentIndex, parent := range parents {
		parentRows, err := c.flattenParent(parent, parentIndex)
		if err != nil {
			return transformFault(c.Name(), err)
		}
		rows = append(rows, parentRows...)
	}
	return transformResult(c.Name(), c.signal, map[string]interface{}{
		"items": rows,
		"count": len(rows),
	})
}

func (c *flatMapCmd) flattenParent(parent interface{}, parentIndex int) ([]interface{}, error) {
	elementNames := sortedFieldNames(c.elementFields)
	elements, width, err := c.resolveElementArrays(parent, parentIndex, elementNames)
	if err != nil {
		return nil, err
	}
	carryNames := sortedFieldNames(c.carryFields)
	carried, err := c.resolveCarryFields(parent, parentIndex, carryNames)
	if err != nil {
		return nil, err
	}
	return zipElementRows(flatMapFields{
		elementNames: elementNames, elements: elements,
		carryNames: carryNames, carried: carried, width: width,
	}), nil
}

func (c *flatMapCmd) resolveElementArrays(
	parent interface{}, parentIndex int, names []string,
) (map[string][]interface{}, int, error) {
	elements := make(map[string][]interface{}, len(names))
	width := -1
	for _, name := range names {
		value, err := resolveItemPath(parent, c.elementFields[name])
		if err != nil {
			return nil, 0, fmt.Errorf("items[%d] element field %q: %w", parentIndex, name, err)
		}
		values, ok := value.([]interface{})
		if !ok {
			return nil, 0, fmt.Errorf(
				"items[%d] element field %q resolved to %T, want array", parentIndex, name, value)
		}
		if width >= 0 && len(values) != width {
			return nil, 0, fmt.Errorf(
				"items[%d] element field %q has length %d, want %d", parentIndex, name, len(values), width)
		}
		width = len(values)
		elements[name] = values
	}
	return elements, width, nil
}

func (c *flatMapCmd) resolveCarryFields(
	parent interface{}, parentIndex int, names []string,
) (map[string]interface{}, error) {
	carried := make(map[string]interface{}, len(names))
	for _, name := range names {
		value, err := resolveItemPath(parent, c.carryFields[name])
		if err != nil {
			return nil, fmt.Errorf("items[%d] carry field %q: %w", parentIndex, name, err)
		}
		carried[name] = value
	}
	return carried, nil
}

type flatMapFields struct {
	elementNames []string
	elements     map[string][]interface{}
	carryNames   []string
	carried      map[string]interface{}
	width        int
}

func zipElementRows(fields flatMapFields) []interface{} {
	rows := make([]interface{}, 0, fields.width)
	for elementIndex := 0; elementIndex < fields.width; elementIndex++ {
		row := make(map[string]interface{}, len(fields.elementNames)+len(fields.carryNames))
		for _, name := range fields.carryNames {
			row[name] = fields.carried[name]
		}
		for _, name := range fields.elementNames {
			row[name] = fields.elements[name][elementIndex]
		}
		rows = append(rows, row)
	}
	return rows
}

type reorderByIndexCmd struct {
	name       string
	items      string
	order      string
	indexField string
	signal     core.Signal
	view       core.CommandStateView
}

func (c *reorderByIndexCmd) Name() string                            { return c.name }
func (c *reorderByIndexCmd) SetCommandState(v core.CommandStateView) { c.view = v }
func (c *reorderByIndexCmd) Undo(_ core.Result) core.Result          { return core.NoopUndo(c.Name()) }

var _ core.CommandStateAware = (*reorderByIndexCmd)(nil)

func (c *reorderByIndexCmd) Execute() core.Result {
	items, err := selectedArray(c.view, c.items)
	if err != nil {
		return transformFault(c.Name(), err)
	}
	order, err := selectedArray(c.view, c.order)
	if err != nil {
		return transformFault(c.Name(), err)
	}
	reordered := make([]interface{}, 0, len(order))
	rows := make([]interface{}, 0, len(order))
	seen := make(map[int]bool, len(order))
	for rowIndex, row := range order {
		value, err := resolveItemPath(row, c.indexField)
		if err != nil {
			return transformFault(c.Name(), fmt.Errorf("order[%d]: %w", rowIndex, err))
		}
		index, err := candidateIndex(value)
		if err != nil {
			return transformFault(c.Name(), fmt.Errorf("order[%d] field %q: %w", rowIndex, c.indexField, err))
		}
		if index < 0 || index >= len(items) {
			return transformFault(c.Name(), fmt.Errorf(
				"order[%d] field %q index %d is outside candidates length %d",
				rowIndex, c.indexField, index, len(items)))
		}
		if seen[index] {
			return transformFault(c.Name(), fmt.Errorf(
				"order[%d] field %q repeats candidate index %d", rowIndex, c.indexField, index))
		}
		seen[index] = true
		reordered = append(reordered, items[index])
		rows = append(rows, row)
	}
	return transformResult(c.Name(), c.signal, map[string]interface{}{
		"items": reordered,
		"rows":  rows,
		"count": len(reordered),
	})
}

func selectedArray(view core.CommandStateView, selector string) ([]interface{}, error) {
	value, err := core.ResolveFromSelector(view, selector)
	if err != nil {
		return nil, err
	}
	items, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("selector %q resolved to %T, want array", selector, value)
	}
	return items, nil
}

func resolveItemPath(item interface{}, path string) (interface{}, error) {
	current := item
	for _, component := range strings.Split(path, ".") {
		switch value := current.(type) {
		case map[string]interface{}:
			next, ok := value[component]
			if !ok {
				return nil, fmt.Errorf("field %q is absent", path)
			}
			current = next
		case []interface{}:
			index, err := strconv.Atoi(component)
			if err != nil || index < 0 {
				return nil, fmt.Errorf("field %q component %q is not an array index", path, component)
			}
			if index >= len(value) {
				return nil, fmt.Errorf("field %q index %d is outside array length %d", path, index, len(value))
			}
			current = value[index]
		default:
			return nil, fmt.Errorf("field %q cannot be read from %T", path, current)
		}
	}
	return current, nil
}

func candidateIndex(value interface{}) (int, error) {
	number, ok := value.(float64)
	if !ok {
		return 0, fmt.Errorf("resolved to %T, want integer", value)
	}
	index := int(number)
	if number != float64(index) {
		return 0, fmt.Errorf("resolved to %v, want integer", number)
	}
	return index, nil
}

func sortedFieldNames(fields map[string]string) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func transformResult(name string, signal core.Signal, output map[string]interface{}) core.Result {
	data, err := json.Marshal(output)
	if err != nil {
		return transformFault(name, fmt.Errorf("encode output: %w", err))
	}
	return core.Result{Signal: signal, CommandName: name, Output: string(data)}
}

func transformFault(name string, err error) core.Result {
	wrapped := fmt.Errorf("%s: %w", name, err)
	return core.Result{Signal: core.CommandError, CommandName: name, Output: wrapped.Error(), Err: wrapped}
}
