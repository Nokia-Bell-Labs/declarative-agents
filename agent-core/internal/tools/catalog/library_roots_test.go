// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

// A library is read-only to its importers (srd056 R2.4). The root registry is
// process-scoped, so these tests do not run in parallel.

func TestLibraryFileCannotOverrideAnImport(t *testing.T) {
	library := t.TempDir()
	t.Cleanup(func() { corepath.SetLibraryRoots(nil) })
	corepath.SetLibraryRoots(map[string]string{"shared": library})
	writeToolImportFixture(t, library, "base.yaml", toolUnit("base", "", "word"))
	writeToolImportFixture(t, library, "over.yaml",
		"unit: over\nimports: [base.yaml]\ntools:\n  - {name: word, binary: echo, override: true}\n")
	top := writeToolImportFixture(t, t.TempDir(), "declarations.yaml",
		"unit: top\nimports: [/opt/shared/over.yaml]\ntools: []\n")

	_, err := newToolImportResolver(nil).loadRoots([]string{top})

	require.ErrorContains(t, err, `library root "shared"`)
	require.ErrorContains(t, err, "read-only")
}

func TestLibraryRelativeImportCannotLeaveItsRoot(t *testing.T) {
	library := t.TempDir()
	t.Cleanup(func() { corepath.SetLibraryRoots(nil) })
	corepath.SetLibraryRoots(map[string]string{"shared": library})
	writeToolImportFixture(t, filepath.Dir(library), "outside.yaml", toolUnit("outside", "", "leak"))
	writeToolImportFixture(t, library, "escaping.yaml",
		"unit: escaping\nimports: [../outside.yaml]\ntools: []\n")
	top := writeToolImportFixture(t, t.TempDir(), "declarations.yaml",
		"unit: top\nimports: [/opt/shared/escaping.yaml]\ntools: []\n")

	_, err := newToolImportResolver(nil).loadRoots([]string{top})

	require.ErrorIs(t, err, corepath.ErrEscapesLibraryRoot)
	require.ErrorContains(t, err, `"shared"`)
}

func TestProfileDeclaresLibraryRootsAndMapsItsOwnPaths(t *testing.T) {
	root := t.TempDir()
	writeToolImportFixture(t, root, "agents/one/profile.yaml",
		"name: one\nmachine: machine.yaml\ntools: [tools.yaml]\n"+
			"tool_declarations: [/opt/shared/units/words.yaml]\nlibraries: {shared: ../../lib}\n")

	profile, err := LoadProfile(filepath.Join(root, "agents", "one", "profile.yaml"))

	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "lib"), profile.Libraries["shared"], "the directory resolves against the profile")
	require.Equal(t, filepath.Join(root, "lib", "units", "words.yaml"), profile.ToolDeclarations[0],
		"a profile-level path under a declared root maps to its directory")
}
