// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resolveDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	r, err := filepath.EvalSymlinks(d)
	require.NoError(t, err)
	return r
}

func TestValidatePath_InsideWorkspace(t *testing.T) {
	root := resolveDir(t)
	f := filepath.Join(root, "hello.txt")
	os.WriteFile(f, []byte("hi"), 0o644)

	resolved, err := ValidatePath(root, "hello.txt")
	require.NoError(t, err)
	assert.Equal(t, f, resolved)
}

func TestValidatePath_AbsolutePath(t *testing.T) {
	root := resolveDir(t)
	f := filepath.Join(root, "abs.txt")
	os.WriteFile(f, []byte("hi"), 0o644)

	resolved, err := ValidatePath(root, f)
	require.NoError(t, err)
	assert.Equal(t, f, resolved)
}

func TestValidatePath_OutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	_, err := ValidatePath(root, "/etc/passwd")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside the workspace")
}

func TestValidatePath_TraversalAttempt(t *testing.T) {
	root := t.TempDir()
	_, err := ValidatePath(root, "../../../etc/passwd")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside the workspace")
}

func TestValidatePath_NotFound(t *testing.T) {
	root := t.TempDir()
	_, err := ValidatePath(root, "nonexistent.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestValidatePath_NewFileInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	_, err := ValidatePath(root, "newdir/newfile.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NotContains(t, err.Error(), "outside")
}

func TestRelPath(t *testing.T) {
	assert.Equal(t, "foo/bar.txt", RelPath("/workspace", "/workspace/foo/bar.txt"))
	assert.Equal(t, "file.go", RelPath("/workspace", "/workspace/file.go"))
}
