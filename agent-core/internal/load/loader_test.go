// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
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
