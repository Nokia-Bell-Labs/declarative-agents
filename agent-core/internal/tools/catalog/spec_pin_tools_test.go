// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Pinning tests for application-dsl-spec 1.0 statements whose loader rules no
// other test exercised (R-TOOL-001, R-TOOL-002, R-TOOL-004, R-PROF-005).

func TestSpecPinBuiltinToolRequiresInit(t *testing.T) {
	t.Parallel()
	_, err := ParseToolDefs([]byte("tools:\n  - name: done\n    type: builtin\n"))
	require.ErrorContains(t, err, `builtin tool "done" has no init field`)
}

func TestSpecPinUnknownToolTypeIsRejected(t *testing.T) {
	t.Parallel()
	_, err := ParseToolDefs([]byte("tools:\n  - name: probe\n    type: boundary\n    binary: echo\n"))
	require.ErrorContains(t, err, `unknown type "boundary"`)
}

func TestSpecPinDeclarationCannotCarryToolsAndTypes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	top := writeToolImportFixture(t, root, "top.yaml",
		"unit: top\ntools:\n  - {name: probe, binary: echo}\ntypes:\n  - {name: Text, schema: {type: string}}\n")

	_, err := newToolImportResolver(nil).loadRoots([]string{top})

	require.ErrorContains(t, err, "cannot carry both tools and types")
}

func TestSpecPinBuiltinToolRejectsExecInputAndOutputMode(t *testing.T) {
	t.Parallel()
	for name, fields := range map[string]string{
		"stdin source": "    stdin_source: $from(search).output\n",
		"output mode":  "    output: {mode: structured}\n",
	} {
		_, err := ParseToolDefs([]byte("tools:\n  - name: probe\n    type: builtin\n    init: done\n" + fields))
		require.ErrorContains(t, err, "cannot declare exec input or output mode", name)
	}
}

func TestSpecPinProfileLibraryRootNamesADirectory(t *testing.T) {
	t.Parallel()
	for name, libraries := range map[string]string{
		"empty directory": "libraries: {shared: \"\"}\n",
		"agent-core root": "libraries: {agent-core: ../lib}\n",
		"non-kebab name":  "libraries: {Shared_Lib: ../lib}\n",
	} {
		root := t.TempDir()
		path := writeToolImportFixture(t, root, filepath.Join("agents", "one", "profile.yaml"),
			"name: one\nmachine: machine.yaml\ntools: [tools.yaml]\n"+libraries)

		_, err := LoadProfile(path)

		require.Error(t, err, name)
		require.ErrorContains(t, err, "library root", name)
	}
}

func TestSpecPinTypeAndFragmentUnitsDeclareUnit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	types := writeToolImportFixture(t, root, "types.yaml", "types:\n  - {name: Text, schema: {type: string}}\n")
	_, err := newToolImportResolver(nil).loadRoots([]string{types})
	require.ErrorContains(t, err, "must declare unit")

	_, err = loadFragmentRoot(t, map[string]string{
		"declarations.yaml": "params:\n- {name: p, type: string}\ntools:\n- name: x\n  binary: echo\n  description: $param(p)\n",
	})
	require.ErrorContains(t, err, "must declare unit")
}
