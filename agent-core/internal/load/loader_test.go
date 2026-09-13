// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestLoadClosureLoadsControlProfileDeterministically(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	previous := corepath.InstallRoot()
	corepath.SetInstallRoot(root)
	t.Cleanup(func() { corepath.SetInstallRoot(previous) })

	profile := filepath.Join(root, "testdata", "integration", "profiles", "control", "profile.yaml")
	first, err := LoadClosure(profile, Options{})
	require.NoError(t, err)
	second, err := LoadClosure(profile, Options{})
	require.NoError(t, err)

	require.Equal(t, first.Files, second.Files)
	require.Equal(t, first.ToolUniverse, second.ToolUniverse)
	require.True(t, sort.StringsAreSorted(first.Files))
	require.Len(t, first.Selected, 3)
	require.Equal(t, "core-control", first.Profile.Name)
	require.Equal(t, "core-control", first.Machine.Name)
	require.NotEmpty(t, first.Rest.Servers)

	seen := make(map[string]bool, len(first.Files))
	for _, path := range first.Files {
		require.False(t, seen[path], "duplicate closure file %s", path)
		seen[path] = true
	}
	for _, path := range []string{
		profile,
		filepath.Join(filepath.Dir(profile), "machine.yaml"),
		filepath.Join(filepath.Dir(profile), "tools.yaml"),
		filepath.Join(filepath.Dir(profile), "declarations.yaml"),
		filepath.Join(filepath.Dir(profile), "rest.yaml"),
		filepath.Join(root, "tools", "builtin", "lifecycle", "all.yaml"),
		filepath.Join(root, "tools", "builtin", "lifecycle", "exit-agent.yaml"),
	} {
		require.True(t, seen[canonicalPath(path)], "closure is missing %s", path)
	}

	ollamaProfile := filepath.Join(root, "testdata", "integration", "profiles", "ollama-rest", "profile.yaml")
	ollama, err := LoadClosure(ollamaProfile, Options{})
	require.NoError(t, err)
	require.Contains(t, ollama.Files,
		canonicalPath(filepath.Join(filepath.Dir(ollamaProfile), "openapi.yaml")))
}

func TestClosureAssetsKeepDigestBoundToLoadedBytes(t *testing.T) {
	root := t.TempDir()
	writeLoadFixture(t, root, "machine.yaml", `name: snapshot
initial_state: Idle
states: [Idle, {name: Done, run_status: succeeded}]
terminal_states: [Done]
signals: [Seed]
transitions: [{state: Idle, signal: Seed, next: Done}]
`)
	writeLoadFixture(t, root, "tools.yaml", "tools: [noop]\n")
	declaration := writeLoadFixture(t, root, "declarations.yaml", "tools:\n- {name: noop, binary: \"true\"}\n")
	profilePath := writeLoadFixture(t, root, "profile.yaml", `name: snapshot
machine: machine.yaml
tools: [tools.yaml]
tool_declarations: [declarations.yaml]
`)
	closure, err := LoadClosure(profilePath, Options{})
	require.NoError(t, err)
	snapshot := catalog.BuildProgramRefFromAssets(closure.ProfilePath, closure.Assets)
	paths := catalog.ProgramPaths{
		Profile: closure.ProfilePath, Machine: closure.Profile.Machine,
		ToolSelections: closure.Profile.Tools, ToolDeclarations: closure.Profile.ToolDeclarations,
		ToolConfigDirs: closure.Profile.ToolConfigDirs, RESTDefinitions: closure.Profile.RestDefinitions,
		RESTConfigDirs: closure.Profile.RestConfigDirs,
	}
	current, err := catalog.BuildProgramRef(paths)
	require.NoError(t, err)
	require.Equal(t, current, snapshot)

	original, err := os.ReadFile(declaration)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.WriteFile(declaration, original, 0o644)) })
	require.NoError(t, os.WriteFile(declaration, append(original, []byte("\n# changed after load\n")...), 0o644))
	changed, err := catalog.BuildProgramRef(paths)
	require.NoError(t, err)
	require.NotEqual(t, changed, snapshot)
	require.Equal(t, snapshot, catalog.BuildProgramRefFromAssets(closure.ProfilePath, closure.Assets))
}

func TestLoadClosureReportsStrictFieldWithSourcePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		file     string
		contents string
		field    string
	}{
		{
			name: "profile", file: "profile.yaml", field: "profiel",
			contents: `name: strict
machine: machine.yaml
tools: [tools.yaml]
tool_declarations: [declarations.yaml]
profiel: typo
`,
		},
		{
			name: "machine", file: "machine.yaml", field: "initial_stat",
			contents: `name: strict
initial_state: Idle
initial_stat: Idle
states: [Idle, {name: Done, run_status: succeeded}]
terminal_states: [Done]
signals: [Seed]
transitions: [{state: Idle, signal: Seed, next: Done}]
`,
		},
		{
			name: "declaration", file: "declarations.yaml", field: "descrption",
			contents: "tools:\n- {name: noop, binary: \"true\", descrption: typo}\n",
		},
		{
			name: "selection", file: "tools.yaml", field: "toolz",
			contents: "tools: [noop]\ntoolz: [noop]\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := writeStrictClosureFixture(t)
			path := writeLoadFixture(t, root, test.file, test.contents)
			_, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})
			require.ErrorContains(t, err, path)
			require.ErrorContains(t, err, test.field)
		})
	}
}

func TestLoadClosureRejectsMultipleDocumentsAtEveryYAMLLoader(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"profile.yaml", "machine.yaml", "declarations.yaml", "tools.yaml"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := writeStrictClosureFixture(t)
			path := filepath.Join(root, name)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, append(data, []byte("---\n{}\n")...), 0o644))
			_, err = LoadClosure(filepath.Join(root, "profile.yaml"), Options{})
			require.ErrorContains(t, err, path)
			require.ErrorContains(t, err, "multiple YAML documents")
		})
	}
}

func writeStrictClosureFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeLoadFixture(t, root, "machine.yaml", `name: strict
initial_state: Idle
states: [Idle, {name: Done, run_status: succeeded}]
terminal_states: [Done]
signals: [Seed]
transitions: [{state: Idle, signal: Seed, next: Done}]
`)
	writeLoadFixture(t, root, "tools.yaml", "tools: [noop]\n")
	writeLoadFixture(t, root, "declarations.yaml", "tools:\n- {name: noop, binary: \"true\"}\n")
	writeLoadFixture(t, root, "profile.yaml", `name: strict
machine: machine.yaml
tools: [tools.yaml]
tool_declarations: [declarations.yaml]
`)
	return root
}

func writeLoadFixture(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}
