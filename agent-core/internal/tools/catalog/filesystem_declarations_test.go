// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestBuiltinCompatibilityBundleIncludesFullLeafBundle(t *testing.T) {
	t.Parallel()
	path := builtinBundlePath(t)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var raw ToolDefsFile
	require.NoError(t, yaml.Unmarshal(data, &raw))
	require.Empty(t, raw.Tools, "compatibility bundle must not inline tool contracts")
	require.Equal(t, []string{"builtin/all.yaml"}, raw.Includes)

	defs, err := LoadToolDeclarations([]string{path})
	require.NoError(t, err)
	read := toolDefByName(t, defs, "read")
	require.NotEmpty(t, read.Metrics.Instruments)
	require.NotEmpty(t, read.Problem)
	require.NotEmpty(t, read.Goals)
}

func TestExecCompatibilityBundleIncludesFullLeafBundle(t *testing.T) {
	t.Parallel()
	path := filepath.Join(filepath.Dir(builtinBundlePath(t)), "exec.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var raw ToolDefsFile
	require.NoError(t, yaml.Unmarshal(data, &raw))
	require.Empty(t, raw.Tools, "compatibility bundle must not inline tool contracts")
	require.Equal(t, []string{"exec/all.yaml"}, raw.Includes)

	defs, err := LoadToolDeclarations([]string{path})
	require.NoError(t, err)
	commit := toolDefByName(t, defs, "commit")
	require.NotEmpty(t, commit.Emits)
	require.NotEmpty(t, commit.Output.Schema)
	require.NotEmpty(t, commit.Errors)
}

func TestSharedToolDeclarationsHaveOneSource(t *testing.T) {
	t.Parallel()
	root := filepath.Dir(builtinBundlePath(t))
	owners := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var raw ToolDefsFile
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, def := range raw.Tools {
			if previous, exists := owners[def.Name]; exists {
				t.Errorf("tool %q is declared in both %s and %s", def.Name, previous, relative)
				continue
			}
			owners[def.Name] = relative
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, owners)
}

func builtinBundlePath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "tools", "builtin.yaml")
}

func toolDefByName(t *testing.T, defs []ToolDef, name string) ToolDef {
	t.Helper()
	for _, def := range defs {
		if def.Name == name {
			return def
		}
	}
	t.Fatalf("tool %q not found", name)
	return ToolDef{}
}
