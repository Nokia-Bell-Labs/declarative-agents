// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package typesys_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/typesys"
)

// rowsSchema is an object holding a scalar, a nested object, and an array of
// objects, so one fixture covers every branch the walker takes.
func rowsSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status": map[string]any{"type": "string"},
			"source": map[string]any{
				"type":       "object",
				"properties": map[string]any{"document": map[string]any{"type": "string"}},
			},
			"rows": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":       "object",
					"properties": map[string]any{"text": map[string]any{"type": "string"}},
				},
			},
		},
	}
}

func TestCheckPathAcceptsValidPaths(t *testing.T) {
	t.Parallel()
	for _, path := range [][]string{
		{"status"},
		{"source"},
		{"source", "document"},
		{"rows"},
		{"rows", "text"},
		{"rows", "0", "text"},
		{"rows", "12"},
	} {
		require.NoErrorf(t, typesys.CheckPath(rowsSchema(), path),
			"path %s should resolve", strings.Join(path, "."))
	}
}

func TestCheckPathRejectsUnknownField(t *testing.T) {
	t.Parallel()
	err := typesys.CheckPath(rowsSchema(), []string{"statuss"})
	require.ErrorContains(t, err, `has no field "statuss"`)
	require.ErrorContains(t, err, `closest declared field is "status"`)
}

func TestCheckPathRejectsUnknownNestedField(t *testing.T) {
	t.Parallel()
	err := typesys.CheckPath(rowsSchema(), []string{"source", "documnet"})
	require.ErrorContains(t, err, "source has no field")
	require.ErrorContains(t, err, `closest declared field is "document"`)
}

func TestCheckPathRejectsUnknownFieldUnderArrayItems(t *testing.T) {
	t.Parallel()
	err := typesys.CheckPath(rowsSchema(), []string{"rows", "txet"})
	require.ErrorContains(t, err, `has no field "txet"`)
	require.ErrorContains(t, err, `closest declared field is "text"`)
}

func TestCheckPathRejectsWalkingPastAScalar(t *testing.T) {
	t.Parallel()
	err := typesys.CheckPath(rowsSchema(), []string{"status", "length"})
	require.ErrorContains(t, err, "status is a string")
	require.ErrorContains(t, err, `no field "length"`)
}

func TestCheckPathRejectsNegativeArrayIndex(t *testing.T) {
	t.Parallel()
	err := typesys.CheckPath(rowsSchema(), []string{"rows", "-1"})
	require.ErrorContains(t, err, "must not be negative")
}

func TestCheckPathListsFieldsWhenNothingIsClose(t *testing.T) {
	t.Parallel()
	err := typesys.CheckPath(rowsSchema(), []string{"completely_unrelated"})
	require.ErrorContains(t, err, "declared fields are rows, source, status")
}

// Gradual typing: nothing is known about an untyped label, so nothing is
// reported against it.
func TestCheckPathSkipsUntypedSchemas(t *testing.T) {
	t.Parallel()
	require.NoError(t, typesys.CheckPath(nil, []string{"anything", "at", "all"}))
	require.NoError(t, typesys.CheckPath(map[string]any{}, []string{"anything"}))
}

func TestCheckPathStopsAtAFieldWithNoDeclaredShape(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"opaque": "not a schema"},
	}
	require.NoError(t, typesys.CheckPath(schema, []string{"opaque", "whatever"}),
		"a field declared without a shape is untyped from there down")
}

func TestCheckPathAcceptsAnEmptyPath(t *testing.T) {
	t.Parallel()
	require.NoError(t, typesys.CheckPath(rowsSchema(), nil))
}

// TestCheckPathAcceptsTheWholeOutputSelector covers srd038's $from(label).$,
// which reads the step's output itself. ResolveFromSelector returns the output
// without decoding it, so no type can fail the path, and a string-typed label
// read that way is correct authoring rather than a missing field.
func TestCheckPathAcceptsTheWholeOutputSelector(t *testing.T) {
	t.Parallel()
	for name, schema := range map[string]map[string]any{
		"string": {"type": "string"},
		"object": {"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string"}}},
		"array":  {"type": "array"},
	} {
		require.NoError(t, typesys.CheckPath(schema, []string{"$"}), name)
	}
	require.Error(t, typesys.CheckPath(map[string]any{"type": "string"}, []string{"$", "a"}),
		"only a lone $ is the whole output; a longer path names a field")
}
