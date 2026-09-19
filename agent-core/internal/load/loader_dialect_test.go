// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

// A chat dialect named in a tool's config is a closure file reached through
// the providers root (srd058 R1.1, R1.3, R2.3). The root registry is
// process-scoped, so these tests do not run in parallel.

func providerFixtureDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "providers", "fixture"))
	require.NoError(t, err)
	return dir
}

func writeDialectProfile(t *testing.T, providers string) string {
	t.Helper()
	root := writeUsednessClosureFixture(t, "ask")
	writeLoadFixture(t, root, "declarations.yaml", `tools:
  - name: ask
    type: builtin
    init: invoke_llm
    config:
      dialect: /opt/providers/chat-dialect.yaml
      model: m1
`)
	writeLoadFixture(t, root, "profile.yaml", "name: usedness\nmachine: machine.yaml\ntools: [tools.yaml]\n"+
		"tool_declarations: [declarations.yaml]\nlibraries: {providers: "+providers+"}\n")
	return filepath.Join(root, "profile.yaml")
}

func TestDumpRecordsDialectRoot(t *testing.T) {
	t.Cleanup(func() { corepath.SetLibraryRoots(nil); corepath.SetLibraryOverrides(nil) })
	fixture := providerFixtureDir(t)
	dialect := filepath.Join(fixture, "chat-dialect.yaml")

	closure, err := LoadClosure(writeDialectProfile(t, fixture), Options{})

	require.NoError(t, err)
	require.Contains(t, closure.Files, dialect, "the dialect is a closure file")
	require.Contains(t, closure.Assets, dialect, "its bytes are part of the program identity")
	require.Len(t, closure.Selected, 1)
	require.Equal(t, dialect, closure.Selected[0].Config["dialect"],
		"the tool carries the resolved path, so building it needs no root registry")

	var dump bytes.Buffer
	require.NoError(t, DumpConfig(closure, &dump))
	var decoded struct {
		Files []dumpFile `yaml:"files"`
	}
	require.NoError(t, yaml.Unmarshal(dump.Bytes(), &decoded))
	roots := map[string]string{}
	for _, file := range decoded.Files {
		roots[file.Path] = file.Root
	}
	require.Equal(t, "providers", roots[dialect], "the dump marks the dialect with its root")
}

func TestProviderLibraryMissingDialectNamesPathAndDirectory(t *testing.T) {
	t.Cleanup(func() { corepath.SetLibraryRoots(nil); corepath.SetLibraryOverrides(nil) })
	empty := t.TempDir()
	corepath.SetLibraryOverrides(map[string]string{"providers": empty})

	_, err := LoadClosure(writeDialectProfile(t, providerFixtureDir(t)), Options{})

	require.ErrorContains(t, err, "/opt/providers/chat-dialect.yaml", "the path as written")
	require.ErrorContains(t, err, filepath.Join(empty, "chat-dialect.yaml"), "where it resolved")
	require.ErrorContains(t, err, `library root "providers" is bound to `+empty, "the bound directory")
}

func TestDialectPathResolvesRelativeToItsDeclaration(t *testing.T) {
	t.Cleanup(func() { corepath.SetLibraryRoots(nil) })
	root := writeUsednessClosureFixture(t, "ask")
	writeLoadFixture(t, root, "chat-dialect.yaml", "unit: local\n")
	writeLoadFixture(t, root, "declarations.yaml",
		"tools:\n  - name: ask\n    type: builtin\n    init: invoke_llm\n    config: {dialect: chat-dialect.yaml}\n")

	closure, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.NoError(t, err)
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, "chat-dialect.yaml"))
	require.NoError(t, err)
	got, err := filepath.EvalSymlinks(closure.Selected[0].Config["dialect"].(string))
	require.NoError(t, err)
	require.Equal(t, resolved, got)
}
