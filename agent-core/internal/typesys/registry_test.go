// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package typesys_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/typesys"
)

func chatTypes() typesys.TypeUnitFile {
	return typesys.TypeUnitFile{
		Unit: "chat-types",
		Types: []typesys.TypeDecl{
			{
				Name:        "ChunkRow",
				Description: "One retrieved chunk flattened for scoring.",
				Schema: map[string]any{
					"type":     "object",
					"required": []any{"text"},
					"properties": map[string]any{
						"text": map[string]any{"type": "string"},
					},
				},
			},
			{
				Name: "ScoredChunk",
				Schema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"chunk": map[string]any{"$type": "chat-types.ChunkRow"},
						"score": map[string]any{"type": "number"},
					},
				},
			},
		},
	}
}

func TestBuildIndexesTypesByQualifiedName(t *testing.T) {
	t.Parallel()
	registry, err := typesys.Build(chatTypes())
	require.NoError(t, err)

	declared, ok := registry.Resolve("chat-types.ChunkRow")
	require.True(t, ok)
	require.Equal(t, "One retrieved chunk flattened for scoring.", declared.Description)
	require.Equal(t, []string{"chat-types.ChunkRow", "chat-types.ScoredChunk"}, registry.Refs())
}

func TestBuildRejectsDuplicateNameAcrossUnits(t *testing.T) {
	t.Parallel()
	other := typesys.TypeUnitFile{
		Unit:  "rollout-types",
		Types: []typesys.TypeDecl{{Name: "ChunkRow", Schema: map[string]any{"type": "string"}}},
	}

	_, err := typesys.Build(chatTypes(), other)

	require.ErrorContains(t, err, `type "ChunkRow" is declared by unit`)
	require.ErrorContains(t, err, "chat-types")
	require.ErrorContains(t, err, "rollout-types")
}

func TestBuildRejectsSchemaOutsideSubsetNamingUnitAndType(t *testing.T) {
	t.Parallel()
	unit := typesys.TypeUnitFile{
		Unit: "bad-types",
		Types: []typesys.TypeDecl{
			{Name: "Weird", Schema: map[string]any{"type": "string", "pattern": "^a"}},
		},
	}

	_, err := typesys.Build(unit)

	require.ErrorContains(t, err, `unit "bad-types" type "Weird"`)
	require.ErrorContains(t, err, `keyword "pattern"`)
}

func TestResolveSchemaExpandsReferenceNestedInProperties(t *testing.T) {
	t.Parallel()
	registry, err := typesys.Build(chatTypes())
	require.NoError(t, err)

	resolved, err := registry.ResolveSchema(map[string]any{
		"$type": "chat-types.ScoredChunk",
	})
	require.NoError(t, err)

	properties := resolved["properties"].(map[string]any)
	chunk := properties["chunk"].(map[string]any)
	require.NotContains(t, chunk, "$type", "the reference expanded rather than surviving")
	require.Equal(t, "object", chunk["type"])
	require.Contains(t, chunk["properties"].(map[string]any), "text")
}

func TestResolveSchemaExpandsReferenceInsideItems(t *testing.T) {
	t.Parallel()
	registry, err := typesys.Build(chatTypes())
	require.NoError(t, err)

	resolved, err := registry.ResolveSchema(map[string]any{
		"type":  "array",
		"items": map[string]any{"$type": "chat-types.ChunkRow"},
	})
	require.NoError(t, err)

	items := resolved["items"].(map[string]any)
	require.Equal(t, "object", items["type"])
}

func TestResolveSchemaReportsUnknownReference(t *testing.T) {
	t.Parallel()
	registry, err := typesys.Build(chatTypes())
	require.NoError(t, err)

	_, err = registry.ResolveSchema(map[string]any{"$type": "chat-types.Missing"})

	require.ErrorContains(t, err, `unknown type reference "chat-types.Missing"`)
	require.ErrorContains(t, err, `no type "Missing" in unit "chat-types"`)
}

func TestResolveSchemaReportsSelfReferenceAsCycle(t *testing.T) {
	t.Parallel()
	unit := typesys.TypeUnitFile{
		Unit: "loop-types",
		Types: []typesys.TypeDecl{{
			Name: "Node",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"next": map[string]any{"$type": "loop-types.Node"}},
			},
		}},
	}
	registry, err := typesys.Build(unit)
	require.NoError(t, err)

	_, err = registry.ResolveSchema(map[string]any{"$type": "loop-types.Node"})

	require.ErrorContains(t, err, "type reference cycle")
	require.ErrorContains(t, err, "loop-types.Node -> loop-types.Node")
}

func TestResolveSchemaReportsCycleAcrossTwoUnits(t *testing.T) {
	t.Parallel()
	left := typesys.TypeUnitFile{
		Unit: "left-types",
		Types: []typesys.TypeDecl{{
			Name: "Left",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"right": map[string]any{"$type": "right-types.Right"}},
			},
		}},
	}
	right := typesys.TypeUnitFile{
		Unit: "right-types",
		Types: []typesys.TypeDecl{{
			Name: "Right",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"left": map[string]any{"$type": "left-types.Left"}},
			},
		}},
	}
	registry, err := typesys.Build(left, right)
	require.NoError(t, err)

	_, err = registry.ResolveSchema(map[string]any{"$type": "left-types.Left"})

	require.ErrorContains(t, err, "type reference cycle")
	require.ErrorContains(t, err, "left-types.Left -> right-types.Right -> left-types.Left")
}

func TestResolveSchemaLeavesTheDeclarationUnmutated(t *testing.T) {
	t.Parallel()
	unit := chatTypes()
	registry, err := typesys.Build(unit)
	require.NoError(t, err)

	first, err := registry.ResolveSchema(map[string]any{"$type": "chat-types.ScoredChunk"})
	require.NoError(t, err)
	second, err := registry.ResolveSchema(map[string]any{"$type": "chat-types.ScoredChunk"})
	require.NoError(t, err)

	require.Equal(t, first, second, "repeated resolution yields the same result")

	declared, ok := registry.Resolve("chat-types.ScoredChunk")
	require.True(t, ok)
	chunk := declared.Schema["properties"].(map[string]any)["chunk"].(map[string]any)
	require.Equal(t, "chat-types.ChunkRow", chunk["$type"],
		"the declaration still carries its reference after resolution")

	// Mutating a resolved schema must not reach the declaration behind it.
	first["properties"].(map[string]any)["chunk"].(map[string]any)["type"] = "mutated"
	again, err := registry.ResolveSchema(map[string]any{"$type": "chat-types.ScoredChunk"})
	require.NoError(t, err)
	require.Equal(t, second, again)
}

func TestResolveSchemaClonesValuesItDoesNotRewrite(t *testing.T) {
	t.Parallel()
	registry, err := typesys.Build(chatTypes())
	require.NoError(t, err)
	declaration := map[string]any{"type": "object", "required": []any{"text"}}

	resolved, err := registry.ResolveSchema(declaration)
	require.NoError(t, err)

	resolved["required"].([]any)[0] = "changed"
	require.Equal(t, "text", declaration["required"].([]any)[0],
		"a resolved schema shares no slice with its declaration")
}

func TestResolveNilSchemaIsNotAnError(t *testing.T) {
	t.Parallel()
	registry, err := typesys.Build()
	require.NoError(t, err)
	resolved, err := registry.ResolveSchema(nil)
	require.NoError(t, err)
	require.Nil(t, resolved)
}
