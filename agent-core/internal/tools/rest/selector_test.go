// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
