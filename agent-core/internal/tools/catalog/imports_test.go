// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToolImportsResolveTwoLevelsInPostorder(t *testing.T) {
	root := t.TempDir()
	leaf := writeToolImportFixture(t, root, "shared/leaf.yaml", toolUnit("leaf", "", "leaf-tool"))
	middle := writeToolImportFixture(
		t, root, "middle/middle.yaml",
		toolUnit("middle", "imports: [../shared/leaf.yaml]\n", "middle-tool"),
	)
	top := writeToolImportFixture(
		t, root, "top.yaml",
		toolUnit("top", "imports: [middle/middle.yaml]\n", "top-tool"),
	)
	visits := map[string]int{}
	resolver := newToolImportResolver(func(path string, _ []byte) error {
		visits[filepath.Clean(path)]++
		return nil
	})

	defs, err := resolver.loadRoots([]string{top})

	require.NoError(t, err)
	require.Equal(t, []string{"leaf-tool", "middle-tool", "top-tool"}, toolNames(defs))
	require.Equal(t, ToolSource{Unit: "leaf", Path: leaf}, defs[0].DeclarationSource())
	require.Equal(t, ToolSource{Unit: "middle", Path: middle}, defs[1].DeclarationSource())
	for _, path := range []string{leaf, middle, top} {
		require.Equal(t, 1, visits[path], path)
	}
}

func TestToolImportsReuseDiamondUnitOnce(t *testing.T) {
	root := t.TempDir()
	leaf := writeToolImportFixture(t, root, "leaf.yaml", toolUnit("leaf", "", "shared"))
	writeToolImportFixture(t, root, "left.yaml", toolUnit("left", "imports: [leaf.yaml]\n", "left"))
	writeToolImportFixture(t, root, "right.yaml", toolUnit("right", "imports: [leaf.yaml]\n", "right"))
	top := writeToolImportFixture(
		t, root, "top.yaml",
		toolUnit("top", "imports: [left.yaml, right.yaml]\n", "top"),
	)
	visits := map[string]int{}
	resolver := newToolImportResolver(func(path string, _ []byte) error {
		visits[filepath.Clean(path)]++
		return nil
	})

	defs, err := resolver.loadRoots([]string{top})

	require.NoError(t, err)
	require.Equal(t, []string{"shared", "left", "right", "top"}, toolNames(defs))
	require.Equal(t, 1, visits[leaf])
}

func TestToolImportsRejectSiblingDuplicateWithSources(t *testing.T) {
	root := t.TempDir()
	left := writeToolImportFixture(t, root, "left.yaml", toolUnit("left", "", "shared"))
	right := writeToolImportFixture(t, root, "right.yaml", toolUnit("right", "", "shared"))
	top := writeToolImportFixture(
		t, root, "top.yaml",
		toolUnit("top", "imports: [left.yaml, right.yaml]\n", "top"),
	)

	_, err := newToolImportResolver(nil).loadRoots([]string{top})

	require.ErrorContains(t, err, `duplicate imported tool "shared"`)
	require.ErrorContains(t, err, `unit "left"`)
	require.ErrorContains(t, err, left)
	require.ErrorContains(t, err, `unit "right"`)
	require.ErrorContains(t, err, right)
}

func TestToolImportsRejectCycleAndUnitCollision(t *testing.T) {
	t.Run("cycle", func(t *testing.T) {
		root := t.TempDir()
		first := writeToolImportFixture(t, root, "first.yaml", toolUnit("first", "imports: [second.yaml]\n"))
		second := writeToolImportFixture(t, root, "second.yaml", toolUnit("second", "imports: [first.yaml]\n"))

		_, err := newToolImportResolver(nil).loadRoots([]string{first})

		require.ErrorContains(t, err, "tool import cycle")
		require.ErrorContains(t, err, first+" -> "+second+" -> "+first)
	})

	t.Run("unit collision", func(t *testing.T) {
		root := t.TempDir()
		first := writeToolImportFixture(t, root, "first.yaml", toolUnit("duplicate", "", "first"))
		second := writeToolImportFixture(t, root, "second.yaml", toolUnit("duplicate", "", "second"))
		top := writeToolImportFixture(
			t, root, "top.yaml",
			toolUnit("top", "imports: [first.yaml, second.yaml]\n", "top"),
		)

		_, err := newToolImportResolver(nil).loadRoots([]string{top})

		require.ErrorContains(t, err, `duplicate tool unit "duplicate"`)
		require.ErrorContains(t, err, first)
		require.ErrorContains(t, err, second)
	})
}

func TestToolImportsApplyExplicitOverrideInImportedPosition(t *testing.T) {
	root := t.TempDir()
	base := writeToolImportFixture(t, root, "base.yaml", `unit: base
tools:
  - {name: before, binary: before}
  - {name: shared, binary: old}
  - {name: after, binary: after}
`)
	top := writeToolImportFixture(t, root, "top.yaml", `unit: top
imports: [base.yaml]
tools:
  - {name: shared, binary: new, override: true}
  - {name: local, binary: local}
`)

	defs, err := newToolImportResolver(nil).loadRoots([]string{top})

	require.NoError(t, err)
	require.Equal(t, []string{"before", "shared", "after", "local"}, toolNames(defs))
	require.Equal(t, "new", defs[1].Binary)
	require.Equal(t, ToolSource{Unit: "top", Path: top}, defs[1].DeclarationSource())
	require.Equal(t, ToolSource{Unit: "base", Path: base}, defs[1].OverrideTarget())
}

func TestToolImportsRejectInvalidOverrideFormsAndLocalDuplicates(t *testing.T) {
	tests := []struct {
		name string
		top  string
		want string
	}{
		{
			name: "collision without override",
			top:  toolUnit("top", "imports: [base.yaml]\n", "shared"),
			want: "collides with imported",
		},
		{
			name: "override without target",
			top: `unit: top
imports: [base.yaml]
tools:
  - {name: missing, binary: echo, override: true}
`,
			want: "overrides no imported target",
		},
		{
			name: "duplicate local",
			top: `unit: top
imports: [base.yaml]
tools:
  - {name: local, binary: one}
  - {name: local, binary: two}
`,
			want: "duplicate local tool",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeToolImportFixture(t, root, "base.yaml", toolUnit("base", "", "shared"))
			top := writeToolImportFixture(t, root, "top.yaml", test.top)

			_, err := newToolImportResolver(nil).loadRoots([]string{top})

			require.ErrorContains(t, err, test.want)
			require.ErrorContains(t, err, top)
		})
	}
}

func TestToolImportsRejectInvalidSyntaxAndContracts(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "missing unit", body: "imports: [child.yaml]\ntools: []\n", want: "must declare unit"},
		{name: "invalid unit", body: "unit: Invalid_Name\ntools: []\n", want: `invalid unit "Invalid_Name"`},
		{name: "unknown override spelling", body: "unit: top\ntools:\n- {name: tool, binary: echo, overide: true}\n", want: `unknown field "overide"`},
		{name: "mixed content", body: "unit: top\nrest: {}\ntools: []\n", want: `field rest not found`},
		{name: "missing tools", body: "unit: top\n", want: "top-level tools field is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeToolImportFixture(t, root, "child.yaml", toolUnit("child", "", "child"))
			top := writeToolImportFixture(t, root, "top.yaml", test.body)

			_, err := newToolImportResolver(nil).loadRoots([]string{top})

			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestToolImportsRejectAbsolutePath(t *testing.T) {
	t.Run("absolute import", func(t *testing.T) {
		root := t.TempDir()
		child := writeToolImportFixture(t, root, "child.yaml", toolUnit("child", "", "child"))
		top := writeToolImportFixture(
			t, root, "top.yaml",
			fmt.Sprintf("unit: top\nimports: [%q]\ntools: []\n", child),
		)

		_, err := newToolImportResolver(nil).loadRoots([]string{top})

		require.ErrorContains(t, err, "imports absolute path")
	})

}

// The audit and runtime load policies still diverge on environment expansion:
// the audit reads declarations as authored, the runtime expands them.
func TestToolLoadOptionsKeepAuditDivergencesInSingleLoader(t *testing.T) {
	root := t.TempDir()
	declaration := writeToolImportFixture(t, root, "tools.yaml", `tools:
  - {name: configured, binary: "${AUDIT_BINARY}"}
`)
	t.Setenv("AUDIT_BINARY", "expanded")

	audit, err := LoadToolDeclarationsWithOptions(
		[]string{declaration},
		LoadOptions{ExpandEnv: false},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, "${AUDIT_BINARY}", audit[0].Binary)

	runtime, err := LoadToolDefs(declaration)
	require.NoError(t, err)
	require.Equal(t, "expanded", runtime[0].Binary)
}

func toolUnit(unit, fields string, tools ...string) string {
	var output strings.Builder
	fmt.Fprintf(&output, "unit: %s\n%s", unit, fields)
	output.WriteString("tools:\n")
	for _, tool := range tools {
		fmt.Fprintf(&output, "  - {name: %s, binary: echo}\n", tool)
	}
	return output.String()
}

func toolNames(defs []ToolDef) []string {
	names := make([]string, len(defs))
	for index, def := range defs {
		names[index] = def.Name
	}
	return names
}

func writeToolImportFixture(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Clean(filepath.Join(root, name))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
