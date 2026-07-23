// Copyright (c) 2026 Nokia. All rights reserved.

// Package compose provides the compose builtin: a word that renders a template
// from command-state $from(label).path selectors, so a later word (for example
// invoke_llm) receives one composed input assembled from non-adjacent prior
// steps without carry_forward chains (srd038-command-state-store).
//
// A template may substitute a value raw ("{{ key }}") or JSON-encoded
// ("{{ json key }}"). The encoded form is what makes a rendered JSON document
// safe: values that carry quotes or newlines are escaped rather than breaking
// the document, and an unresolved selector encodes to "" rather than a hole.
package compose

import (
	"encoding/json"
	"errors"
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
func (b Builder) Build(prev core.Result) core.Command {
	return &composeCmd{
		name:     b.ToolName,
		template: b.Template,
		inputs:   b.Inputs,
		signal:   b.Signal,
		prev:     prev,
	}
}

type composeCmd struct {
	name     string
	template string
	inputs   map[string]string
	signal   core.Signal
	prev     core.Result
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
	var missing []error
	for _, key := range sortedKeys(c.inputs) {
		selector := c.inputs[key]
		value, err := c.resolve(selector)
		if err != nil {
			missing = append(missing, fmt.Errorf("%s: %w", key, err))
			value = ""
		}
		rendered = substitute(rendered, key, stringify(value), jsonify(value))
	}
	signal := c.signal
	if signal == "" {
		signal = defaultComposeSignal
	}
	res := core.Result{Signal: signal, CommandName: c.Name(), Output: rendered}
	if len(missing) > 0 {
		res.Err = fmt.Errorf("compose: unresolved selectors: %w", errors.Join(missing...))
	}
	return res
}

// resolve reads one input selector. A $from(label).path selector reads a
// labeled prior step; a $.path selector reads the previous Result, and a bare
// $. reads that Result's output verbatim.
//
// The previous-result form exists because a word's output is not always
// reachable by label: the chatbot's answer comes from whichever chat-LLM word
// the router dispatched, so no single $from(label) names it (srd002 R2).
func (c *composeCmd) resolve(selector string) (interface{}, error) {
	if strings.HasPrefix(selector, "$from(") {
		return core.ResolveFromSelector(c.view, selector)
	}
	// A bare $. is the whole previous output. ParseSelector requires a path, so
	// this form is recognized here rather than there: the output of a model word
	// is prose, not a JSON object with a path to walk.
	if selector == "$." {
		return c.prev.Output, nil
	}
	parsed, ok := core.ParseSelector(selector)
	if !ok || parsed.Label != "" {
		return nil, fmt.Errorf("selector %q is not a $. or $from(label).path selector", selector)
	}
	if len(parsed.Path) == 0 {
		return c.prev.Output, nil
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(c.prev.Output), &decoded); err != nil {
		return nil, fmt.Errorf("selector %q: the previous step's output is not a JSON object", selector)
	}
	value, found := parsed.Resolve(decoded)
	if !found {
		return nil, fmt.Errorf("selector %q: path not found in the previous step's output", selector)
	}
	return value, nil
}

// substitute replaces a key's placeholders. "{{ key }}" inserts the value as
// it renders; "{{ json key }}" inserts its JSON encoding, so a template that
// builds a JSON document stays valid whatever the value holds. Without the json
// form a string carrying a quote, backslash, or newline — an LLM answer,
// routinely — breaks the document it is substituted into.
func substitute(template, key, raw, encoded string) string {
	template = strings.ReplaceAll(template, "{{ json "+key+" }}", encoded)
	template = strings.ReplaceAll(template, "{{json "+key+"}}", encoded)
	template = strings.ReplaceAll(template, "{{ "+key+" }}", raw)
	return strings.ReplaceAll(template, "{{"+key+"}}", raw)
}

// jsonify renders a resolved value as a JSON literal. An unresolved selector
// arrives as the empty string and encodes to "", so a rendered JSON document
// stays parseable rather than being left with a hole where the value belonged.
func jsonify(v interface{}) string {
	if v == nil {
		return `""`
	}
	data, err := json.Marshal(v)
	if err != nil {
		return `""`
	}
	return string(data)
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
