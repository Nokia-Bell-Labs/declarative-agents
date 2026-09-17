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

// Library-rooted imports in the closure (srd056 R1, R3.1, R3.2). The install
// root is process-scoped, so these tests do not run in parallel.

// fakeAgentCoreLibrary lays a type unit down where a checkout keeps agent-core's
// library and maps /opt/agent-core onto it for the test.
func fakeAgentCoreLibrary(t *testing.T) string {
	t.Helper()
	installRoot := t.TempDir()
	unit := filepath.Join(installRoot, "tools", "units", "shapes.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(unit), 0o755))
	require.NoError(t, os.WriteFile(unit, []byte(`unit: builtin-shapes
types:
  - name: Row
    schema: {type: object, properties: {text: {type: string}}}
`), 0o644))
	corepath.SetInstallRoot(installRoot)
	t.Cleanup(func() { corepath.SetInstallRoot("") })
	return unit
}

func TestLoadClosureImportsAUnitFromTheAgentCoreLibrary(t *testing.T) {
	unit := fakeAgentCoreLibrary(t)
	root := writeUsednessClosureFixture(t, "selected")
	writeLoadFixture(t, root, "declarations.yaml", `unit: root
imports: [/opt/agent-core/tools/units/shapes.yaml]
tools:
  - name: selected
    binary: echo
    output:
      schema: {$type: builtin-shapes.Row}
`)

	closure, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.NoError(t, err, "a rooted type unit a tool references earns its import")
	require.Contains(t, closure.Files, unit, "the closure holds the mapped library file")
	require.Equal(t, "object", closure.Selected[0].Output.Schema["type"])

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
	require.Equal(t, "agent-core", roots["shapes.yaml"], "the dump marks the library file")
	require.Empty(t, roots["declarations.yaml"], "a profile's own file carries no root")
}

func TestLoadClosureReportsAnUnusedLibraryImport(t *testing.T) {
	unit := fakeAgentCoreLibrary(t)
	root := writeUsednessClosureFixture(t, "selected")
	writeLoadFixture(t, root, "declarations.yaml", `unit: root
imports: [/opt/agent-core/tools/units/shapes.yaml]
tools:
  - name: selected
    binary: echo
`)

	_, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.ErrorContains(t, err, "unused declaration imports")
	require.ErrorContains(t, err, unit, "a library import is held to usedness like any other")
}

func TestLoadClosureRejectsAnAbsoluteImportOutsideALibraryRoot(t *testing.T) {
	root := writeUsednessClosureFixture(t, "selected")
	elsewhere := writeLoadFixture(t, t.TempDir(), "shapes.yaml", "unit: shapes\ntypes: []\n")
	writeLoadFixture(t, root, "declarations.yaml", "unit: root\nimports: ["+elsewhere+"]\ntools:\n  - name: selected\n    binary: echo\n")

	_, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.ErrorContains(t, err, "imports absolute path")
	require.ErrorIs(t, err, corepath.ErrOutsideLibraryRoot)
}
