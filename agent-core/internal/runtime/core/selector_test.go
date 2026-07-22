// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSelectorGrammar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		label string
		path  []string
		valid bool
	}{
		{name: "current field", value: "$.a", path: []string{"a"}, valid: true},
		{name: "current nested", value: "$.a.b", path: []string{"a", "b"}, valid: true},
		{name: "prior field", value: "$from(step).a", label: "step", path: []string{"a"}, valid: true},
		{name: "hyphenated label", value: "$from(embed-query).mapped.embedding", label: "embed-query", path: []string{"mapped", "embedding"}, valid: true},
		{name: "unicode component", value: "$.δοκιμή", path: []string{"δοκιμή"}, valid: true},
		{name: "empty current path", value: "$."},
		{name: "empty leading component", value: "$..a"},
		{name: "empty middle component", value: "$.a..b"},
		{name: "empty trailing component", value: "$.a."},
		{name: "empty prior label", value: "$from().a"},
		{name: "empty prior path", value: "$from(step)."},
		{name: "prior empty leading component", value: "$from(step)..a"},
		{name: "prior empty middle component", value: "$from(step).a..b"},
		{name: "missing close parenthesis", value: "$from(step.a"},
		{name: "missing path separator", value: "$from(step)a"},
		{name: "extra close parenthesis", value: "$from(step)).a"},
		{name: "parenthesis in label", value: "$from(st(ep).a"},
		{name: "dot in label", value: "$from(step.one).a"},
		{name: "ascii whitespace", value: "$.a b"},
		{name: "unicode whitespace", value: "$.a\u00a0b"},
		{name: "control character", value: "$.a\x00b"},
		{name: "wrong prefix", value: "a.b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parsed, ok := ParseSelector(tt.value)
			require.Equal(t, tt.valid, ok)
			if !ok {
				return
			}
			assert.Equal(t, tt.label, parsed.Label)
			assert.Equal(t, tt.path, parsed.Path)
		})
	}
}

func TestParsedSelectorResolvesNestedObject(t *testing.T) {
	t.Parallel()
	parsed, ok := ParseSelector("$.data.id")
	require.True(t, ok)
	value, ok := parsed.Resolve(map[string]interface{}{
		"data": map[string]interface{}{"id": "doc-1"},
	})
	require.True(t, ok)
	assert.Equal(t, "doc-1", value)
}

func FuzzParseSelectorCanonicalGrammar(f *testing.F) {
	for _, seed := range []string{
		"$.a",
		"$.a.b",
		"$from(step).a",
		"$from(step).a..b",
		"$..a",
		"",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		parsed, ok := ParseSelector(raw)
		if !ok {
			return
		}
		require.NotEmpty(t, parsed.Path)
		for _, component := range parsed.Path {
			require.NotEmpty(t, component)
			for _, r := range component {
				require.False(t, unicode.IsSpace(r) || unicode.IsControl(r))
			}
		}
		canonical := "$." + strings.Join(parsed.Path, ".")
		if parsed.Label != "" {
			canonical = "$from(" + parsed.Label + ")." + strings.Join(parsed.Path, ".")
		}
		require.Equal(t, raw, canonical)
	})
}
