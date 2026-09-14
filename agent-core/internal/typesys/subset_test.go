// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package typesys_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/typesys"
)

func TestValidateSubsetAcceptsEveryAllowedKeyword(t *testing.T) {
	t.Parallel()
	require.NoError(t, typesys.ValidateSubset(map[string]any{
		"type":        "object",
		"description": "a chunk",
		"required":    []any{"text"},
		"properties": map[string]any{
			"text":   map[string]any{"type": "string", "enum": []any{"a", "b"}},
			"counts": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
		},
	}))
}

func TestValidateSubsetRejectsKeywordOutsideSubset(t *testing.T) {
	t.Parallel()
	err := typesys.ValidateSubset(map[string]any{"type": "string", "pattern": "^a"})
	require.ErrorContains(t, err, `keyword "pattern" is outside the declaration type subset`)
}

func TestValidateSubsetRejectsKeywordNestedInProperties(t *testing.T) {
	t.Parallel()
	err := typesys.ValidateSubset(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{"type": "string", "minLength": 1},
		},
	})
	require.ErrorContains(t, err, `keyword "minLength"`)
	require.ErrorContains(t, err, "properties.text",
		"the error locates the violation rather than only naming the keyword")
}

func TestValidateSubsetRejectsKeywordNestedInItems(t *testing.T) {
	t.Parallel()
	err := typesys.ValidateSubset(map[string]any{
		"type":  "array",
		"items": map[string]any{"type": "object", "additionalProperties": false},
	})
	require.ErrorContains(t, err, `keyword "additionalProperties"`)
	require.ErrorContains(t, err, "items")
}

func TestValidateSubsetRejectsTypeOutsideSubset(t *testing.T) {
	t.Parallel()
	err := typesys.ValidateSubset(map[string]any{"type": "null"})
	require.ErrorContains(t, err, `type "null" is outside the declaration type subset`)
}

func TestValidateSubsetAcceptsBareReference(t *testing.T) {
	t.Parallel()
	require.NoError(t, typesys.ValidateSubset(map[string]any{"$type": "chat-types.ChunkRow"}))
}

func TestValidateSubsetRejectsReferenceWithSiblingKeyword(t *testing.T) {
	t.Parallel()
	err := typesys.ValidateSubset(map[string]any{
		"$type":    "chat-types.ChunkRow",
		"required": []any{"text"},
	})
	require.ErrorContains(t, err, `reference cannot carry sibling keyword "required"`)
}
