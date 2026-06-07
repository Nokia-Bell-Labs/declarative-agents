// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		require.NoError(t, cmd.Run(), "git %v", args)
	}
	// Create initial commit so HEAD exists
	f := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(f, []byte("# test\n"), 0o644))
	for _, args := range [][]string{
		{"add", "-A"},
		{"commit", "-m", "initial"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		require.NoError(t, cmd.Run(), "git %v", args)
	}
	return dir
}

func TestCommit_Success(t *testing.T) {
	dir := initGitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hello"), 0o644))

	cmd := &commitCmd{root: dir, message: "add new file"}
	res := cmd.Execute()

	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Contains(t, res.Output, "committed")
}

func TestCommit_NothingToCommit(t *testing.T) {
	dir := initGitRepo(t)

	cmd := &commitCmd{root: dir, message: "empty"}
	res := cmd.Execute()

	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Contains(t, res.Output, "nothing to commit")
}

func TestCommit_NotGitRepo(t *testing.T) {
	dir := t.TempDir()

	cmd := &commitCmd{root: dir, message: "fail"}
	res := cmd.Execute()

	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "not a git repository")
}

func TestWorkspaceStatus_Clean(t *testing.T) {
	dir := initGitRepo(t)

	cmd := &workspaceStatusCmd{root: dir}
	res := cmd.Execute()

	assert.Equal(t, core.ToolDone, res.Signal)
	assert.True(t, strings.HasPrefix(res.Output, "clean"))
}

func TestWorkspaceStatus_Dirty(t *testing.T) {
	dir := initGitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("x"), 0o644))

	cmd := &workspaceStatusCmd{root: dir}
	res := cmd.Execute()

	assert.Equal(t, core.ToolDone, res.Signal)
	assert.True(t, strings.HasPrefix(res.Output, "dirty"))
	assert.Contains(t, res.Output, "dirty.txt")
}

func TestWorktreeAdd_Success(t *testing.T) {
	dir := initGitRepo(t)

	cmd := &worktreeAddCmd{repoDir: dir, id: "task-1", prefix: "work/"}
	res := cmd.Execute()

	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Contains(t, res.Output, "worktree:")
	assert.Contains(t, res.Output, "branch: work/task-1")

	wtDir := dir + "-task-1"
	_, err := os.Stat(wtDir)
	assert.NoError(t, err, "worktree directory should exist")

	// Cleanup
	cleanup := exec.Command("git", "worktree", "remove", "--force", wtDir)
	cleanup.Dir = dir
	_ = cleanup.Run()
	delBranch := exec.Command("git", "branch", "-D", "work/task-1")
	delBranch.Dir = dir
	_ = delBranch.Run()
}

func TestWorktreeAdd_BranchExists(t *testing.T) {
	dir := initGitRepo(t)

	cmd := exec.Command("git", "branch", "work/dup")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	add := &worktreeAddCmd{repoDir: dir, id: "dup", prefix: "work/"}
	res := add.Execute()

	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "already exists")
}

func TestWorktreeRemove_Success(t *testing.T) {
	dir := initGitRepo(t)

	branch := "work/rm-test"
	wtDir := dir + "-rm-test"
	cmd := exec.Command("git", "worktree", "add", "-b", branch, wtDir)
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	rm := &worktreeRemoveCmd{repoDir: dir, dir: wtDir, branch: branch}
	res := rm.Execute()

	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Contains(t, res.Output, "removed")

	_, err := os.Stat(wtDir)
	assert.True(t, os.IsNotExist(err), "worktree dir should be gone")
}

func TestWorktreeRemove_AlreadyRemoved(t *testing.T) {
	dir := initGitRepo(t)

	rm := &worktreeRemoveCmd{repoDir: dir, dir: "/nonexistent/path", branch: "work/gone"}
	res := rm.Execute()

	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Contains(t, res.Output, "already removed")
}

func TestCommitBuilder_MissingMessage(t *testing.T) {
	b := &CommitBuilder{Root: "/tmp"}
	cmd := b.Build(core.Result{Output: `{"parameters":{}}`})
	assert.Equal(t, "commit", cmd.Name())
	res := cmd.Execute()
	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "message")
}

func TestWorktreeAddBuilder_MissingID(t *testing.T) {
	b := &WorktreeAddBuilder{Root: "/tmp"}
	cmd := b.Build(core.Result{Output: `{"parameters":{}}`})
	assert.Equal(t, "worktree_add", cmd.Name())
	res := cmd.Execute()
	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "id")
}
