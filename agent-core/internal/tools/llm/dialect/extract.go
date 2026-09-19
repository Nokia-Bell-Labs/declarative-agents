// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package dialect

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// A response selector is the shared $. selector grammar (core.ParseSelector)
// with three components that apply where the walk meets an array: a decimal
// index takes one element, * takes every element, and key=value keeps the
// elements whose key equals value. The walk continues into each element kept,
// so $.message.content.type=text.text reads the text of every text block.
type selector []string

func parseSelector(raw string) (selector, error) {
	parsed, ok := core.ParseSelector(raw)
	if !ok || parsed.Label != "" {
		return nil, fmt.Errorf("selector %q is not a $. selector", raw)
	}
	return selector(parsed.Path), nil
}

func (s selector) values(document interface{}) []interface{} {
	current := []interface{}{document}
	for _, component := range s {
		var next []interface{}
		for _, value := range current {
			next = append(next, step(value, component)...)
		}
		current = next
	}
	return current
}

func step(value interface{}, component string) []interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		if item, ok := typed[component]; ok {
			return []interface{}{item}
		}
	case []interface{}:
		return stepArray(typed, component)
	}
	return nil
}

func stepArray(items []interface{}, component string) []interface{} {
	if component == "*" {
		return items
	}
	if index, err := strconv.Atoi(component); err == nil {
		if index >= 0 && index < len(items) {
			return []interface{}{items[index]}
		}
		return nil
	}
	key, want, filter := strings.Cut(component, "=")
	if !filter {
		return nil
	}
	var kept []interface{}
	for _, item := range items {
		if object, ok := item.(map[string]interface{}); ok && fmt.Sprint(object[key]) == want {
			kept = append(kept, item)
		}
	}
	return kept
}

// ErrNoText marks a reply with no text from a dialect that requires text.
var ErrNoText = errors.New("response contains no text")

// Reply is what a dialect reads out of one response body.
type Reply struct {
	Text      string
	TokensIn  int
	TokensOut int
}

// ExtractReply decodes a response body and reads the text and usage the
// dialect's selectors name. Text selected from several places is joined in
// order; a dialect that requires text fails on a reply with none.
func (c Chat) ExtractReply(body []byte) (Reply, error) {
	var document interface{}
	if err := json.Unmarshal(body, &document); err != nil {
		return Reply{}, fmt.Errorf("decode %s response: %w", c.ProviderName, err)
	}
	text, err := c.text(document)
	if err != nil {
		return Reply{}, err
	}
	in, err := c.count(document, c.Response.InputTokens)
	if err != nil {
		return Reply{}, err
	}
	out, err := c.count(document, c.Response.OutputTokens)
	if err != nil {
		return Reply{}, err
	}
	return Reply{Text: text, TokensIn: in, TokensOut: out}, nil
}

func (c Chat) text(document interface{}) (string, error) {
	sel, err := parseSelector(c.Response.Text)
	if err != nil {
		return "", err
	}
	var text strings.Builder
	for _, value := range sel.values(document) {
		if part, ok := value.(string); ok {
			text.WriteString(part)
		}
	}
	if c.Response.TextRequired && text.Len() == 0 {
		return "", fmt.Errorf("%s %w at %s", c.ProviderName, ErrNoText, c.Response.Text)
	}
	return text.String(), nil
}

func (c Chat) count(document interface{}, raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	sel, err := parseSelector(raw)
	if err != nil {
		return 0, err
	}
	values := sel.values(document)
	if len(values) == 0 {
		return 0, nil
	}
	number, ok := values[0].(float64)
	if !ok {
		return 0, fmt.Errorf("%s usage at %s is %T, not a number", c.ProviderName, raw, values[0])
	}
	return int(number), nil
}
