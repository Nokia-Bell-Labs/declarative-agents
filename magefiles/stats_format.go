// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// formatWidth is the budget, in columns, for printing a value on one line.
// It measures the value only, not the key in front of it, so entries of the
// same shape format the same way regardless of how long their names are —
// that uniformity is what makes a long list of agents scannable. Lines can
// therefore run somewhat past this width.
const formatWidth = 110

// orderedObject is a JSON object that remembers the order its keys were
// written in. The sub-module stats targets encode structs, so field order
// carries meaning (a "total" precedes the breakdown it summarizes) that a
// map[string]any would discard.
type orderedObject struct {
	keys   []string
	values map[string]any
}

// decodeOrdered parses JSON while preserving object key order. Objects become
// *orderedObject and numbers stay json.Number, so re-encoding reproduces the
// original digits rather than a float rendering of them.
func decodeOrdered(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	v, err := decodeOrderedValue(dec)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func decodeOrderedValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}

	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}

	switch delim {
	case '{':
		obj := &orderedObject{values: map[string]any{}}
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyTok.(string)
			if !ok {
				return nil, fmt.Errorf("object key is %T, want string", keyTok)
			}
			val, err := decodeOrderedValue(dec)
			if err != nil {
				return nil, err
			}
			if _, seen := obj.values[key]; !seen {
				obj.keys = append(obj.keys, key)
			}
			obj.values[key] = val
		}
		if _, err := dec.Token(); err != nil { // closing brace
			return nil, err
		}
		return obj, nil
	case '[':
		arr := []any{}
		for dec.More() {
			val, err := decodeOrderedValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, val)
		}
		if _, err := dec.Token(); err != nil { // closing bracket
			return nil, err
		}
		return arr, nil
	}
	return nil, fmt.Errorf("unexpected delimiter %q", delim)
}

// formatJSON re-indents a JSON document so that any value whose one-line form
// fits within formatWidth stays on one line, and only larger values expand.
// The result is the same document encoding the same values: leaf objects such
// as {"files": 4, "lines": 145} read as single facts instead of sprawling
// over four lines each.
func formatJSON(raw []byte) ([]byte, error) {
	v, err := decodeOrdered(raw)
	if err != nil {
		return nil, fmt.Errorf("parse stats JSON: %w", err)
	}
	var b strings.Builder
	if err := writeValue(&b, v, 0); err != nil {
		return nil, err
	}
	b.WriteString("\n")
	return []byte(b.String()), nil
}

func writeValue(b *strings.Builder, v any, indent int) error {
	compact, err := compactValue(v)
	if err != nil {
		return err
	}
	if indent+len(compact) <= formatWidth || !isContainer(v) {
		b.WriteString(compact)
		return nil
	}

	pad := strings.Repeat(" ", indent)
	switch val := v.(type) {
	case *orderedObject:
		b.WriteString("{\n")
		for i, key := range val.keys {
			b.WriteString(pad + "  ")
			b.WriteString(encodeString(key))
			b.WriteString(": ")
			if err := writeValue(b, val.values[key], indent+2); err != nil {
				return err
			}
			if i < len(val.keys)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(pad + "}")
	case []any:
		b.WriteString("[\n")
		for i, item := range val {
			b.WriteString(pad + "  ")
			if err := writeValue(b, item, indent+2); err != nil {
				return err
			}
			if i < len(val)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(pad + "]")
	}
	return nil
}

// compactValue encodes v on a single line, with a space after each colon and
// comma so the compact form stays readable rather than dense.
func compactValue(v any) (string, error) {
	switch val := v.(type) {
	case *orderedObject:
		var parts []string
		for _, key := range val.keys {
			inner, err := compactValue(val.values[key])
			if err != nil {
				return "", err
			}
			parts = append(parts, encodeString(key)+": "+inner)
		}
		return "{" + strings.Join(parts, ", ") + "}", nil
	case []any:
		var parts []string
		for _, item := range val {
			inner, err := compactValue(item)
			if err != nil {
				return "", err
			}
			parts = append(parts, inner)
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case json.Number:
		return val.String(), nil
	case string:
		return encodeString(val), nil
	case nil:
		return "null", nil
	default:
		out, err := json.Marshal(val)
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
}

func isContainer(v any) bool {
	switch v.(type) {
	case *orderedObject, []any:
		return true
	}
	return false
}

func encodeString(s string) string {
	out, err := json.Marshal(s)
	if err != nil { // json.Marshal does not fail on strings
		return `""`
	}
	return string(out)
}
