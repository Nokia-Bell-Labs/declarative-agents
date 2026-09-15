// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Declaration type units in the closure: resolution into tool schemas, the
// load errors srd051 names, and how usedness credits a type unit that
// contributes no tools.

func TestLoadClosureResolvesTypeReferencesInToolSchemas(t *testing.T) {
	root := writeUsednessClosureFixture(t, "selected")
	writeLoadFixture(t, root, "types.yaml", `unit: shapes
types:
  - name: Row
    schema: {type: object, properties: {text: {type: string}}}
`)
	writeLoadFixture(t, root, "declarations.yaml", `unit: root
imports: [types.yaml]
tools:
  - name: selected
    binary: echo
    output:
      schema: {$type: shapes.Row}
`)

	closure, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.NoError(t, err)
	require.Len(t, closure.Selected, 1)
	schema := closure.Selected[0].Output.Schema
	require.NotContains(t, schema, "$type", "consumers see a plain schema")
	require.Equal(t, "object", schema["type"])
	require.Contains(t, schema["properties"].(map[string]any), "text")

	declared, ok := closure.Types.Resolve("shapes.Row")
	require.True(t, ok, "the registry stays available for the dump and later checks")
	require.Equal(t, "Row", declared.Name)
}

func TestLoadClosureRejectsUnknownTypeReference(t *testing.T) {
	root := writeUsednessClosureFixture(t, "selected")
	writeLoadFixture(t, root, "types.yaml", `unit: shapes
types:
  - name: Row
    schema: {type: object}
`)
	writeLoadFixture(t, root, "declarations.yaml", `unit: root
imports: [types.yaml]
tools:
  - name: selected
    binary: echo
    output:
      schema: {$type: shapes.Missing}
`)

	_, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.ErrorContains(t, err, `tool "selected" output schema`)
	require.ErrorContains(t, err, `unknown type reference "shapes.Missing"`)
}

func TestLoadClosureRejectsSchemaOutsideTypeSubset(t *testing.T) {
	root := writeUsednessClosureFixture(t, "selected")
	writeLoadFixture(t, root, "types.yaml", `unit: shapes
types:
  - name: Row
    schema: {type: object, additionalProperties: false}
`)
	writeLoadFixture(t, root, "declarations.yaml", `unit: root
imports: [types.yaml]
tools:
  - {name: selected, binary: echo, output: {schema: {$type: shapes.Row}}}
`)

	_, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.ErrorContains(t, err, "load declaration types")
	require.ErrorContains(t, err, `keyword "additionalProperties"`)
}

// A type unit contributes no tools, so usedness has to credit it through a
// schema reference or every profile importing one would fail.
func TestLoadClosureTreatsReferencedTypeUnitAsUsed(t *testing.T) {
	root := writeUsednessClosureFixture(t, "selected")
	writeLoadFixture(t, root, "types.yaml", `unit: shapes
types:
  - name: Row
    schema: {type: object}
`)
	writeLoadFixture(t, root, "declarations.yaml", `unit: root
imports: [types.yaml]
tools:
  - {name: selected, binary: echo, output: {schema: {$type: shapes.Row}}}
`)

	_, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.NoError(t, err)
}

func TestLoadClosureRejectsTypeUnitNoToolReferences(t *testing.T) {
	root := writeUsednessClosureFixture(t, "selected")
	writeLoadFixture(t, root, "types.yaml", `unit: shapes
types:
  - name: Row
    schema: {type: object}
`)
	writeLoadFixture(t, root, "declarations.yaml", `unit: root
imports: [types.yaml]
tools:
  - {name: selected, binary: echo}
`)

	_, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.ErrorContains(t, err, "unused declaration imports")
	require.ErrorContains(t, err, filepath.Join(root, "types.yaml"))
}

// A signature's types are references like any other, so a type unit reached
// only through a signature earns its import.
func TestLoadClosureCreditsTypeUnitReachedThroughSignature(t *testing.T) {
	root := writeUsednessClosureFixture(t, "selected")
	writeLoadFixture(t, root, "types.yaml", `unit: shapes
types:
  - name: Row
    schema: {type: object, properties: {text: {type: string}}}
`)
	writeLoadFixture(t, root, "declarations.yaml", `unit: root
imports: [types.yaml]
tools:
  - name: selected
    binary: echo
    signature:
      output: shapes.Row
      emits: [Done]
`)

	closure, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.NoError(t, err)
	require.Len(t, closure.Selected, 1)
	def := closure.Selected[0]
	require.Equal(t, "object", def.Output.Schema["type"],
		"the signature output resolves into the schema every reader uses")
	require.Equal(t, []string{"Done"}, def.Emits,
		"the signature's signals reach the legacy field")
}
