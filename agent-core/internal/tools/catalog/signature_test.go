// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/typesys"
)

func signatureRegistry(t *testing.T) *typesys.Registry {
	t.Helper()
	registry, err := typesys.Build(typesys.TypeUnitFile{
		Path: "/units/chat-types.yaml",
		Unit: "chat-types",
		Types: []typesys.TypeDecl{
			{Name: "Sources", Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
			}},
			{Name: "Rows", Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"rows": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
			}},
		},
	})
	require.NoError(t, err)
	return registry
}

func signedTool() ToolDef {
	return ToolDef{
		Name: "flatten_chunks", Type: "builtin", Init: "array_transform",
		Category:    "word",
		Description: "Flatten compatible sources one row per chunk.",
		Signature: &ToolSignature{
			Input:  "chat-types.Sources",
			Output: "chat-types.Rows",
			Emits:  []string{"ChunksFlattened", "CommandError"},
		},
	}
}

func TestSignatureTypesResolveIntoTheLegacyFields(t *testing.T) {
	t.Parallel()
	resolved, refs, err := ResolveToolSchemas(
		[]ToolDef{signedTool()}, signatureRegistry(t),
	)
	require.NoError(t, err)
	require.Len(t, resolved, 1)

	def := resolved[0]
	require.Equal(t, "object", def.Output.Schema["type"])
	require.Contains(t, def.Output.Schema["properties"].(map[string]any), "rows")
	require.Equal(t, "object", def.Parameters["type"])
	require.Contains(t, def.Parameters["properties"].(map[string]any), "paths")

	require.Equal(t, []string{"ChunksFlattened", "CommandError"}, def.Emits,
		"every existing reader of Emits sees the signature's signals")
	require.ElementsMatch(t,
		[]string{"chat-types.Rows", "chat-types.Sources"}, refs["flatten_chunks"],
		"both signature types count toward closure usedness")
}

func TestSignatureRejectsConflictingEmits(t *testing.T) {
	t.Parallel()
	def := signedTool()
	def.Emits = []string{"SomethingElse"}

	_, _, err := ResolveToolSchemas([]ToolDef{def}, signatureRegistry(t))
	require.NoError(t, err, "resolution itself does not police the conflict")

	require.ErrorContains(t, validateToolDefs([]ToolDef{def}),
		"declares signature.emits")
	require.ErrorContains(t, validateToolDefs([]ToolDef{def}), "which differ")
}

func TestSignatureAcceptsIdenticalLegacyEmits(t *testing.T) {
	t.Parallel()
	def := signedTool()
	def.Emits = []string{"ChunksFlattened", "CommandError"}
	require.NoError(t, validateToolDefs([]ToolDef{def}),
		"an identical legacy list lets the migration convert one file at a time")
}

func TestSignatureRejectsOutputDeclaredTwice(t *testing.T) {
	t.Parallel()
	def := signedTool()
	def.Output.Schema = map[string]any{"type": "object"}
	require.ErrorContains(t, validateToolDefs([]ToolDef{def}),
		"declares both signature.output and output.schema")
}

func TestSignatureRejectsInputDeclaredTwice(t *testing.T) {
	t.Parallel()
	def := signedTool()
	def.Parameters = map[string]any{"type": "object"}
	require.ErrorContains(t, validateToolDefs([]ToolDef{def}),
		"declares both signature.input and parameters")
}

func TestSignatureUnknownTypeUsesTheOrdinaryReferenceError(t *testing.T) {
	t.Parallel()
	def := signedTool()
	def.Signature.Output = "chat-types.Absent"

	_, _, err := ResolveToolSchemas([]ToolDef{def}, signatureRegistry(t))

	require.ErrorContains(t, err, `tool "flatten_chunks" signature output`)
	require.ErrorContains(t, err, `unknown type reference "chat-types.Absent"`,
		"srd051 R6.5: no second error form for signature references")
}

func TestToolWithoutSignatureIsUnchanged(t *testing.T) {
	t.Parallel()
	def := ToolDef{
		Name: "plain", Type: "builtin", Init: "file_read",
		Emits:      []string{"Done"},
		Parameters: map[string]any{"type": "object"},
	}
	resolved, _, err := ResolveToolSchemas([]ToolDef{def}, signatureRegistry(t))
	require.NoError(t, err)
	require.Equal(t, []string{"Done"}, resolved[0].Emits)
	require.Equal(t, "object", resolved[0].Parameters["type"])
}
