// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package compose

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

var itemPlaceholder = regexp.MustCompile(`\{\{\s*(json\s+)?([^\s{}]+)\s*\}\}`)

// RenderEachBuilder renders each item in an ordered array and joins the parts.
type RenderEachBuilder struct {
	ToolName     string
	Items        string
	ItemTemplate string
	Separator    string
	Signal       core.Signal
}

func ValidateRenderEachConfig(toolName, items, itemTemplate, signal string) error {
	if _, _, ok := core.ParseFromSelector(items); !ok {
		return fmt.Errorf("tool %q: config items must be a $from(label).path selector", toolName)
	}
	if itemTemplate == "" {
		return fmt.Errorf("tool %q: config requires item_template", toolName)
	}
	if signal == "" {
		return fmt.Errorf("tool %q: config requires signal", toolName)
	}
	for _, match := range itemPlaceholder.FindAllStringSubmatch(itemTemplate, -1) {
		if match[2] == "." {
			continue
		}
		if parsed, ok := core.ParseSelector("$." + match[2]); !ok || parsed.Label != "" {
			return fmt.Errorf("tool %q: item_template field %q is not a dotted path", toolName, match[2])
		}
	}
	if remainder := itemPlaceholder.ReplaceAllString(itemTemplate, ""); strings.Contains(remainder, "{{") {
		return fmt.Errorf("tool %q: item_template contains a malformed placeholder", toolName)
	}
	return nil
}

func (b RenderEachBuilder) Build(_ core.Result) core.Command {
	return &renderEachCmd{
		name: b.ToolName, items: b.Items, itemTemplate: b.ItemTemplate,
		separator: b.Separator, signal: b.Signal,
	}
}

type renderEachCmd struct {
	name, items, itemTemplate, separator string
	signal                               core.Signal
	view                                 core.CommandStateView
}

func (c *renderEachCmd) Name() string                            { return c.name }
func (c *renderEachCmd) SetCommandState(v core.CommandStateView) { c.view = v }
func (c *renderEachCmd) Undo(_ core.Result) core.Result          { return core.NoopUndo(c.Name()) }

var _ core.CommandStateAware = (*renderEachCmd)(nil)

func (c *renderEachCmd) Execute() core.Result {
	value, err := core.ResolveFromSelector(c.view, c.items)
	if err != nil {
		return c.fault(err)
	}
	items, ok := value.([]interface{})
	if !ok {
		return c.fault(fmt.Errorf("items selector %q resolved to %T, want array", c.items, value))
	}
	rendered := make([]string, 0, len(items))
	for i, item := range items {
		part, err := renderItem(c.itemTemplate, item)
		if err != nil {
			return c.fault(fmt.Errorf("items[%d]: %w", i, err))
		}
		rendered = append(rendered, part)
	}
	return core.Result{
		Signal: c.signal, CommandName: c.Name(), Output: strings.Join(rendered, c.separator),
	}
}

func renderItem(template string, item interface{}) (string, error) {
	var renderErr error
	out := itemPlaceholder.ReplaceAllStringFunc(template, func(placeholder string) string {
		if renderErr != nil {
			return ""
		}
		match := itemPlaceholder.FindStringSubmatch(placeholder)
		value := item
		if match[2] != "." {
			var err error
			value, err = itemFieldValue(item, match[2])
			if err != nil {
				renderErr = err
				return ""
			}
		}
		if match[1] != "" {
			return jsonify(value)
		}
		return stringify(value)
	})
	return out, renderErr
}

func itemFieldValue(item interface{}, field string) (interface{}, error) {
	return resolveItemPath(item, field)
}

func (c *renderEachCmd) fault(err error) core.Result {
	wrapped := fmt.Errorf("%s: %w", c.Name(), err)
	return core.Result{Signal: core.CommandError, CommandName: c.Name(), Output: wrapped.Error(), Err: wrapped}
}
