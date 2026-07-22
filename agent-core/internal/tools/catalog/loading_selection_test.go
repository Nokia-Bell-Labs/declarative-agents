// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadToolSelection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := dir + "/tools.yaml"
	writeFile(t, path, `tools:
  - read
  - write
  - build
`)
	names, err := LoadToolSelection(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"read", "write", "build"}, names)
}

func TestLoadToolSelections(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	writeFile(t, dir+"/a.yaml", "tools:\n  - read\n  - write\n")
	writeFile(t, dir+"/b.yaml", "tools:\n  - build\n  - write\n")

	names, err := LoadToolSelections([]string{dir + "/a.yaml", dir + "/b.yaml"})
	require.NoError(t, err)
	assert.Equal(t, []string{"read", "write", "build"}, names)
}

func TestLoadToolSelections_Single(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir+"/tools.yaml", "tools:\n  - read\n  - write\n")

	names, err := LoadToolSelections([]string{dir + "/tools.yaml"})
	require.NoError(t, err)
	assert.Equal(t, []string{"read", "write"}, names)
}

func TestLoadToolDeclarationsFromDirs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "read.yaml"), `tools:
  - name: read
    type: builtin
    init: file_read
    description: Read a file
`)
	writeFile(t, filepath.Join(dir, "write.yaml"), `tools:
  - name: write
    type: builtin
    init: file_write
    description: Write a file
`)
	writeFile(t, filepath.Join(dir, "not-yaml.txt"), "ignored")
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o755))

	defs, err := LoadToolDeclarationsFromDirs([]string{dir})
	require.NoError(t, err)
	require.Len(t, defs, 2)
	assert.Equal(t, "read", defs[0].Name)
	assert.Equal(t, "write", defs[1].Name)
}

func TestLoadToolDeclarationsFromDirs_MissingDir(t *testing.T) {
	t.Parallel()

	_, err := LoadToolDeclarationsFromDirs([]string{"/nonexistent/dir"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scan tool config dir")
}

func TestSelectTools(t *testing.T) {
	t.Parallel()

	decls := []ToolDef{
		{Name: "read", Type: "builtin", Init: "file_read", Description: "Read files"},
		{Name: "write", Type: "builtin", Init: "file_write", Description: "Write files"},
		{Name: "build", Type: "exec", Binary: "go", Args: []string{"build"}, Description: "Go build"},
	}

	t.Run("valid selection", func(t *testing.T) {
		t.Parallel()

		result, err := SelectTools(decls, []string{"read", "build"})
		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, "read", result[0].Name)
		assert.Equal(t, "build", result[1].Name)
		assert.Equal(t, "builtin", result[0].Type)
		assert.Equal(t, "exec", result[1].Type)
	})

	t.Run("undeclared tool", func(t *testing.T) {
		t.Parallel()

		_, err := SelectTools(decls, []string{"read", "missing"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing")
		assert.Contains(t, err.Error(), "not declared")
	})

	t.Run("empty selection", func(t *testing.T) {
		t.Parallel()

		result, err := SelectTools(decls, []string{})
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestLoadToolDeclarations_Merge(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir+"/a.yaml", `tools:
  - name: foo
    type: exec
    binary: echo
    args: [a]
    description: "Foo A"
`)
	writeFile(t, dir+"/b.yaml", `tools:
  - name: foo
    type: exec
    binary: echo
    args: [b]
    description: "Foo B"
  - name: bar
    type: exec
    binary: echo
    args: [bar]
    description: "Bar"
`)
	defs, err := LoadToolDeclarations([]string{dir + "/a.yaml", dir + "/b.yaml"})
	require.NoError(t, err)
	require.Len(t, defs, 2)
	assert.Equal(t, "foo", defs[0].Name)
	assert.Equal(t, "Foo B", defs[0].Description)
	assert.Equal(t, "bar", defs[1].Name)
}
