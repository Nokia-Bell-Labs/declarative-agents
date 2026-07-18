// Copyright (c) 2026 Nokia. All rights reserved.

// Package compose provides the compose builtin: a word that renders a template
// from command-state $from(label).path selectors, so a later word (for example
// invoke_llm) receives one composed input assembled from non-adjacent prior
// steps without carry_forward chains (srd038-command-state-store).
package compose

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const defaultComposeSignal = core.Signal("Composed")

// Builder constructs compose commands from a template and $from input selectors.
type Builder struct {
	ToolName string
	Template string
	Inputs   map[string]string
	Signal   core.Signal
}

// Build returns a compose command. The engine injects the command-state view
// before dispatch (core.CommandStateAware).
func (b Builder) Build(_ core.Result) core.Command {
	return &composeCmd{
		name:     b.ToolName,
		template: b.Template,
		inputs:   b.Inputs,
		signal:   b.Signal,
	}
}

type composeCmd struct {
	name     string
	template string
	inputs   map[string]string
	signal   core.Signal
	view     core.CommandStateView
}

func (c *composeCmd) Name() string { return c.name }

// SetCommandState receives the read-only command-state view so the template's
// $from selectors resolve against prior steps.
func (c *composeCmd) SetCommandState(view core.CommandStateView) { c.view = view }

var _ core.CommandStateAware = (*composeCmd)(nil)

func (c *composeCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

// Execute renders the template, substituting each {{ key }} placeholder with the
// value its $from selector resolves to. A selector that does not resolve renders
// empty and is reported, so a degraded upstream step (for example a missing RAG
// chunk set) still yields a prompt rather than failing the run
// (srd038-command-state-store R1.5).
func (c *composeCmd) Execute() core.Result {
	rendered := c.template
	var missing []string
	for _, key := range sortedKeys(c.inputs) {
		selector := c.inputs[key]
		value, err := core.ResolveFromSelector(c.view, selector)
		if err != nil {
			missing = append(missing, fmt.Sprintf("%s: %v", key, err))
			value = ""
		}
		rendered = substitute(rendered, key, stringify(value))
	}
	signal := c.signal
	if signal == "" {
		signal = defaultComposeSignal
	}
	res := core.Result{Signal: signal, CommandName: c.Name(), Output: rendered}
	if len(missing) > 0 {
		res.Err = fmt.Errorf("compose: unresolved selectors: %s", strings.Join(missing, "; "))
	}
	return res
}

// substitute replaces "{{ key }}" and "{{key}}" placeholders with value.
func substitute(template, key, value string) string {
	template = strings.ReplaceAll(template, "{{ "+key+" }}", value)
	return strings.ReplaceAll(template, "{{"+key+"}}", value)
}

// stringify renders a resolved value: strings pass through; anything else is
// JSON-encoded so structured chunks read predictably in the prompt.
func stringify(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(data)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
