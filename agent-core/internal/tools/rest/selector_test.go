// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func commandStateDigest(output string) core.ResultDigest {
	return core.ResultDigest{
		Output:           output,
		RedactionVersion: core.OutputRedactionVersion1,
		RedactionStatus:  core.OutputRedactionApplied,
	}
}

func TestResponseSelectorsShareNestedMapSemantics(t *testing.T) {
	t.Parallel()
	items := []interface{}{map[string]interface{}{"id": "first"}}
	payload := map[string]interface{}{
		"id": "top",
		"data": map[string]interface{}{
			"id":   "nested",
			"meta": map[string]interface{}{"owner": "alice"},
		},
		"items":       items,
		"literal.key": "dotted",
	}
	tests := []struct {
		name     string
		selector string
		want     interface{}
	}{
		{name: "top level", selector: "$.id", want: "top"},
		{name: "nested object", selector: "$.data.id", want: "nested"},
		{name: "deep object", selector: "$.data.meta.owner", want: "alice"},
		{name: "whole array", selector: "$.items", want: items},
		{name: "array traversal unsupported", selector: "$.items.0.id"},
		{name: "missing path", selector: "$.data.missing"},
		{name: "empty component", selector: "$.data..id"},
		{name: "dotted key has no escape grammar", selector: `$.literal\.key`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, selectorValue(tt.selector, payload), "client response mapping")
			assert.Equal(t, tt.want, responseValue(tt.selector, payload), "server response mapping")
			assert.Equal(t, tt.want, machineSelectorValue(tt.selector, payload), "machine response mapping")
		})
	}
}

func TestCurrentAndFromSelectorsResolveEquivalentPaths(t *testing.T) {
	t.Parallel()
	output := `{"data":{"id":"doc-1"}}`
	source := map[string]interface{}{"data": map[string]interface{}{"id": "doc-1"}}
	current, ok := resolveResultSelector("$.data.id", source)
	require.True(t, ok)
	view := core.NewCommandStateView(core.Execution{{
		CommandName: "load",
		Result:      commandStateDigest(output),
	}})
	prior, err := core.ResolveFromSelector(view, "$from(load).data.id")
	require.NoError(t, err)
	assert.Equal(t, current, prior)
}

func TestValidateSelectorFormRejectsMalformedComponents(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		source   string
		selector string
	}{
		{name: "current empty path", source: bodySourcePreviousResult, selector: "$."},
		{name: "current empty component", source: bodySourcePreviousResult, selector: "$.data..id"},
		{name: "current whitespace", source: bodySourcePreviousResult, selector: "$.data. id"},
		{name: "from empty path", source: bodySourceCommandState, selector: "$from(load)."},
		{name: "from empty component", source: bodySourceCommandState, selector: "$from(load).data..id"},
		{name: "from malformed parenthesis", source: bodySourceCommandState, selector: "$from(load)).data.id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Error(t, validateSelectorForm("probe", tt.source, tt.selector))
		})
	}
}
