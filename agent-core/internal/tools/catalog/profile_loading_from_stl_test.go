// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadProfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
name: generator
machine: machine.yaml
tools:
  - tools.yaml
tool_declarations:
  - ../../tools/builtin.yaml
  - llm/default.yaml
`), 0o644))

	p, err := LoadProfile(path)
	require.NoError(t, err)
	assert.Equal(t, "generator", p.Name)
	assert.Equal(t, filepath.Join(dir, "machine.yaml"), p.Machine)
	assert.Equal(t, []string{filepath.Join(dir, "tools.yaml")}, p.Tools)
	assert.Equal(t, []string{
		filepath.Join(dir, "../../tools/builtin.yaml"),
		filepath.Join(dir, "llm/default.yaml"),
	}, p.ToolDeclarations)
}

func TestLoadProfile_AbsolutePaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
name: test
machine: /absolute/machine.yaml
tools:
  - /absolute/tools.yaml
`), 0o644))

	p, err := LoadProfile(path)
	require.NoError(t, err)
	assert.Equal(t, "/absolute/machine.yaml", p.Machine)
	assert.Equal(t, []string{"/absolute/tools.yaml"}, p.Tools)
}

func TestLoadProfile_MissingMachine(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
name: bad
tools:
  - tools.yaml
`), 0o644))

	_, err := LoadProfile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "machine is required")
}

func TestLoadProfile_MissingTools(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
name: bad
machine: machine.yaml
`), 0o644))

	_, err := LoadProfile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tools entry is required")
}

func TestLoadProfile_Directory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
name: test
machine: machine.yaml
tools:
  - tools.yaml
directory: ../workspace
`), 0o644))

	p, err := LoadProfile(path)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "../workspace"), p.Directory)
}
