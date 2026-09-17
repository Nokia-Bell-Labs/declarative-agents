// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

// Declared library roots in the closure (srd056 R2, R3). The registry is
// process-scoped, so these tests do not run in parallel.

func libraryProfile(name string) string {
	return filepath.Join("..", "..", "testdata", "integration", "profiles", name, "profile.yaml")
}

func TestLoadClosureResolvesADeclaredLibraryRoot(t *testing.T) {
	t.Cleanup(func() { corepath.SetLibraryRoots(nil); corepath.SetLibraryOverrides(nil) })
	shared, err := filepath.Abs(filepath.Join("..", "..", "testdata", "integration", "library"))
	require.NoError(t, err)

	for _, name := range []string{"library-one", "library-two"} {
		closure, err := LoadClosure(libraryProfile(name), Options{})
		require.NoError(t, err, name)
		require.Contains(t, closure.Files, filepath.Join(shared, "units", "shared-words.yaml"),
			"the rooted unit is a closure file, keyed by the directory the root maps to")
		require.Contains(t, closure.Files, filepath.Join(shared, "types", "shared-types.yaml"),
			"the unit's own relative import resolves inside the root")
		require.Len(t, closure.Selected, 1)
		require.Equal(t, "say_shared", closure.Selected[0].Name)

		var dump bytes.Buffer
		require.NoError(t, DumpConfig(closure, &dump))
		var decoded struct {
			Files []dumpFile `yaml:"files"`
		}
		require.NoError(t, yaml.Unmarshal(dump.Bytes(), &decoded))
		roots := map[string]string{}
		for _, file := range decoded.Files {
			roots[filepath.Base(file.Path)] = file.Root
		}
		require.Equal(t, "shared", roots["shared-words.yaml"], "the dump marks a declared-root file with its root")
		require.Empty(t, roots["declarations.yaml"])
	}
	require.Empty(t, corepath.LibraryRoots(), "roots do not outlive the closure that declared them")
}

func TestLoadClosureAppliesALibraryOverride(t *testing.T) {
	t.Cleanup(func() { corepath.SetLibraryRoots(nil); corepath.SetLibraryOverrides(nil) })
	other := t.TempDir()
	writeLoadFixture(t, other, "shared-words.yaml", "unit: shared-words\ntools:\n  - name: say_shared\n    binary: echo\n")
	require.NoError(t, mkdirAll(filepath.Join(other, "units")))
	writeLoadFixture(t, filepath.Join(other, "units"), "shared-words.yaml",
		"unit: shared-words\ntools:\n  - name: say_shared\n    binary: echo\n")
	corepath.SetLibraryOverrides(map[string]string{"shared": other})

	closure, err := LoadClosure(libraryProfile("library-one"), Options{})

	require.NoError(t, err)
	require.Contains(t, closure.Files, filepath.Join(other, "units", "shared-words.yaml"),
		"--library replaces the declared directory for this run")
}

func TestLoadClosureReportsAnUndeclaredLibraryPrefix(t *testing.T) {
	t.Cleanup(func() { corepath.SetLibraryRoots(nil) })
	root := writeUsednessClosureFixture(t, "selected")
	writeLoadFixture(t, root, "declarations.yaml",
		"unit: root\nimports: [/opt/mesh-lib/words.yaml]\ntools:\n  - name: selected\n    binary: echo\n")

	_, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	var undeclared corepath.UndeclaredRootError
	require.ErrorAs(t, err, &undeclared)
	require.Equal(t, "mesh-lib", undeclared.Name)
	require.ErrorContains(t, err, filepath.Join(root, "declarations.yaml"))
}

func TestLoadClosureRejectsAReservedOrMalformedRootName(t *testing.T) {
	for _, name := range []string{"agent-core", "Mesh_Lib"} {
		root := writeUsednessClosureFixture(t, "other")
		writeLoadFixture(t, root, "declarations.yaml", "tools:\n  - name: other\n    binary: echo\n")
		writeLoadFixture(t, root, "profile.yaml", "name: usedness\nmachine: machine.yaml\ntools: [tools.yaml]\n"+
			"tool_declarations: [declarations.yaml]\nlibraries: {"+name+": lib}\n")

		_, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

		require.ErrorContains(t, err, "library root", name)
		require.ErrorContains(t, err, filepath.Join(root, "profile.yaml"))
	}
}

func mkdirAll(path string) error { return os.MkdirAll(path, 0o755) }
