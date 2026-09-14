// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
		filepath.Join(root, "testdata", "integration", "units", "control-tools.yaml"),
		filepath.Join(root, "testdata", "integration", "units", "control-rest.yaml"),
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

func TestControlImportsMatchesResolvedControlProgram(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	previous := corepath.InstallRoot()
	corepath.SetInstallRoot(root)
	t.Cleanup(func() { corepath.SetInstallRoot(previous) })
	profiles := filepath.Join(root, "testdata", "integration", "profiles")

	control, err := LoadClosure(filepath.Join(profiles, "control", "profile.yaml"), Options{})
	require.NoError(t, err)
	imported, err := LoadClosure(filepath.Join(profiles, "control-imports", "profile.yaml"), Options{})
	require.NoError(t, err)

	require.Equal(t, normalizedResolvedDump(t, control), normalizedResolvedDump(t, imported))
	for _, path := range []string{
		filepath.Join(profiles, "control-imports", "declarations.yaml"),
		filepath.Join(profiles, "control-imports", "rest.yaml"),
		filepath.Join(root, "testdata", "integration", "units", "control-tools.yaml"),
		filepath.Join(root, "testdata", "integration", "units", "control-rest.yaml"),
		filepath.Join(root, "tools", "builtin", "lifecycle", "exit-agent.yaml"),
	} {
		require.Contains(t, imported.Files, canonicalPath(path))
	}
}

func normalizedResolvedDump(t *testing.T, closure *Closure) string {
	t.Helper()
	view := *closure
	view.Profile = catalog.AgentProfile{}
	view.Files = nil
	var output bytes.Buffer
	require.NoError(t, DumpConfig(&view, &output))
	return output.String()
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

func TestLoadClosureRejectsUnusedToolImportsDeterministically(t *testing.T) {
	root := writeUsednessClosureFixture(t, "selected")
	writeLoadFixture(t, root, "a.yaml", "unit: alpha\ntools:\n- {name: unused-a, binary: echo}\n")
	writeLoadFixture(t, root, "z.yaml", "unit: zulu\ntools:\n- {name: unused-z, binary: echo}\n")
	writeLoadFixture(t, root, "declarations.yaml", `unit: root
imports: [z.yaml, a.yaml]
tools:
  - {name: selected, binary: echo}
`)

	_, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.ErrorContains(t, err, "unused declaration imports")
	require.ErrorContains(t, err, `tool unit "alpha"`)
	require.ErrorContains(t, err, filepath.Join(root, "a.yaml"))
	require.ErrorContains(t, err, `tool unit "zulu"`)
	require.Less(t, strings.Index(err.Error(), `unit "alpha"`), strings.Index(err.Error(), `unit "zulu"`))
}

func TestLoadClosureAcceptsUsedToolImportAndDiamondReexports(t *testing.T) {
	root := writeUsednessClosureFixture(t, "shared")
	writeLoadFixture(t, root, "leaf.yaml", "unit: leaf\ntools:\n- {name: shared, binary: echo}\n")
	writeLoadFixture(t, root, "left.yaml", "unit: left\nimports: [leaf.yaml]\ntools: []\n")
	writeLoadFixture(t, root, "right.yaml", "unit: right\nimports: [leaf.yaml]\ntools: []\n")
	writeLoadFixture(t, root, "declarations.yaml", "unit: root\nimports: [left.yaml, right.yaml]\ntools: []\n")

	closure, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.NoError(t, err)
	require.Len(t, closure.Selected, 1)
	require.Equal(t, "leaf", closure.Selected[0].DeclarationSource().Unit)
}

func TestLoadClosureTreatsSelectedOverrideTargetAsUsed(t *testing.T) {
	root := writeUsednessClosureFixture(t, "shared")
	writeLoadFixture(t, root, "base.yaml", "unit: base\ntools:\n- {name: shared, binary: old}\n")
	writeLoadFixture(t, root, "declarations.yaml", `unit: root
imports: [base.yaml]
tools:
  - {name: shared, binary: new, override: true}
`)

	closure, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.NoError(t, err)
	require.Equal(t, "new", closure.Selected[0].Binary)
	require.Equal(t, "base", closure.Selected[0].OverrideTarget().Unit)
}

func TestLoadClosureRejectsUnusedRESTImport(t *testing.T) {
	root := writeUsednessClosureFixture(t, "selected")
	writeLoadFixture(t, root, "declarations.yaml", "tools:\n- {name: selected, binary: echo}\n")
	imported := writeLoadFixture(t, root, "rest-unused.yaml", `unit: unused-rest
rest:
  version: v1
  limits: {unused: {}}
`)
	writeLoadFixture(t, root, "rest.yaml", "unit: rest-root\nimports: [rest-unused.yaml]\nrest: {}\n")
	writeUsednessProfile(t, root, true)

	_, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.ErrorContains(t, err, `REST unit "unused-rest"`)
	require.ErrorContains(t, err, imported)
}

func TestLoadClosureAcceptsReferencedRESTImportsAndDependencies(t *testing.T) {
	root := writeUsednessClosureFixture(t, "launch")
	writeLoadFixture(t, root, "declarations.yaml", `tools:
  - name: launch
    type: builtin
    init: rest_server_launch
    config: {rest_ref: control}
`)
	writeLoadFixture(t, root, "limits.yaml", `unit: shared-limits
rest:
  version: v1
  limits:
    local:
      timeout: 30s
      max_request_bytes: 1024
      max_response_bytes: 1024
      redirect: {mode: none}
      network: {hosts: [127.0.0.1], ports: [0]}
`)
	writeLoadFixture(t, root, "server.yaml", `unit: control-server
rest:
  servers:
    control:
      address: 127.0.0.1:0
      limits_ref: local
      endpoints:
        exit:
          method: POST
          path: /exit
          binding: emit_signal
          signal: ExitRequested
`)
	writeLoadFixture(t, root, "rest.yaml", `unit: rest-root
imports: [server.yaml, limits.yaml]
rest: {}
`)
	writeUsednessProfile(t, root, true)

	closure, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.NoError(t, err)
	require.Contains(t, closure.Rest.Servers, "control")
	require.Contains(t, closure.Rest.Limits, "local")
}

func writeUsednessClosureFixture(t *testing.T, selected string) string {
	t.Helper()
	root := t.TempDir()
	writeLoadFixture(t, root, "machine.yaml", `name: usedness
initial_state: Idle
states: [Idle, {name: Done, run_status: succeeded}]
terminal_states: [Done]
signals: [Seed]
transitions: [{state: Idle, signal: Seed, next: Done}]
`)
	writeLoadFixture(t, root, "tools.yaml", "tools: ["+selected+"]\n")
	writeUsednessProfile(t, root, false)
	return root
}

func writeUsednessProfile(t *testing.T, root string, withREST bool) {
	t.Helper()
	rest := ""
	if withREST {
		rest = "rest_definitions: [rest.yaml]\n"
	}
	writeLoadFixture(t, root, "profile.yaml", `name: usedness
machine: machine.yaml
tools: [tools.yaml]
tool_declarations: [declarations.yaml]
`+rest)
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
