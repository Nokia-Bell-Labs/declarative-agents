// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCollectProfileDirsRecursesFamilies verifies that a directory holding
// machine.yaml is discovered as a profile, that a family directory without one
// has its immediate subdirectories scanned one level deeper, and that
// directories with no machine.yaml at either level are skipped. This guards the
// gap where profiles nested under a family (knowledge-manager/corpus-reader)
// silently escaped the spec corpus.
func TestCollectProfileDirsRecursesFamilies(t *testing.T) {
	root := t.TempDir()
	writeMachine := func(rel string) {
		dir := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "machine.yaml"), []byte("name: x\n"), 0o644))
	}
	// A top-level profile.
	writeMachine("top")
	// Two profiles nested one level under a family directory.
	writeMachine("family/leaf-a")
	writeMachine("family/leaf-b")
	// A family subdirectory without machine.yaml is not a profile.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "family", "assets"), 0o755))
	// A family whose subdirectories hold no machine.yaml contributes nothing.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "empty", "sub"), 0o755))

	dirs := collectProfileDirs(root)

	names := make([]string, 0, len(dirs))
	byName := make(map[string]string, len(dirs))
	for _, pd := range dirs {
		names = append(names, pd.Name)
		byName[pd.Name] = pd.Dir
	}
	sort.Strings(names)

	assert.Equal(t, []string{"leaf-a", "leaf-b", "top"}, names)
	assert.Equal(t, filepath.Join(root, "top"), byName["top"])
	assert.Equal(t, filepath.Join(root, "family", "leaf-a"), byName["leaf-a"])
	assert.Equal(t, filepath.Join(root, "family", "leaf-b"), byName["leaf-b"])
	assert.NotContains(t, names, "family")
	assert.NotContains(t, names, "empty")
	assert.NotContains(t, names, "assets")
}

// TestCollectProfileDirsMissingRootIsEmpty confirms a missing profiles root
// yields no profiles rather than an error, preserving the disabled-mode path.
func TestCollectProfileDirsMissingRootIsEmpty(t *testing.T) {
	assert.Empty(t, collectProfileDirs(filepath.Join(t.TempDir(), "does-not-exist")))
}
