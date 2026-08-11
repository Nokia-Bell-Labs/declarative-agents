// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestBuiltinBundleIncludesSingleFilesystemWordContracts(t *testing.T) {
	t.Parallel()
	path := builtinBundlePath(t)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var raw ToolDefsFile
	require.NoError(t, yaml.Unmarshal(data, &raw))
	for _, def := range raw.Tools {
		require.NotContains(t, []string{"read", "write", "edit", "find"}, def.Name,
			"filesystem words belong in per-word includes, not inline duplicates")
	}
	for _, include := range []string{
		"builtin/read.yaml", "builtin/write.yaml", "builtin/edit.yaml", "builtin/find.yaml",
	} {
		require.Contains(t, raw.Includes, include)
	}

	defs, err := LoadToolDeclarations([]string{path})
	require.NoError(t, err)
	read := toolDefByName(t, defs, "read")
	require.NotEmpty(t, read.Metrics.Instruments)
	require.NotEmpty(t, read.Problem)
	require.NotEmpty(t, read.Goals)
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
