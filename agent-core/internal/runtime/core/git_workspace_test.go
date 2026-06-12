// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGitWorkspaceCheckpointAndRestore(t *testing.T) {
	ctx := context.Background()
	repo := initGitWorkspaceRepo(t)
	ws, err := NewGitWorkspace(repo)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.txt"), []byte("v1\n"), 0o644))
	ref1, err := ws.Checkpoint(ctx, "first")
	require.NoError(t, err)
	require.NotEmpty(t, ref1)

	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.txt"), []byte("v2\n"), 0o644))
	ref2, err := ws.Checkpoint(ctx, "second")
	require.NoError(t, err)
	require.NotEmpty(t, ref2)
	require.NotEqual(t, ref1, ref2)

	require.NoError(t, ws.Restore(ctx, ref1))
	data, err := os.ReadFile(filepath.Join(repo, "file.txt"))
	require.NoError(t, err)
	require.Equal(t, "v1\n", string(data))

	current, err := ws.CurrentRef(ctx)
	require.NoError(t, err)
	require.Equal(t, ref1, current)
}

func TestGitWorkspaceCheckpointAllowEmpty(t *testing.T) {
	ctx := context.Background()
	repo := initGitWorkspaceRepo(t)
	ws, err := NewGitWorkspace(repo)
	require.NoError(t, err)

	ref, err := ws.Checkpoint(ctx, "empty")

	require.NoError(t, err)
	require.NotEmpty(t, ref)
	log := gitWorkspaceCmd(t, repo, "log", "--format=%s", "-1")
	require.Equal(t, "checkpoint: empty", strings.TrimSpace(log))
}

func TestGitWorkspaceCurrentRefReturnsHead(t *testing.T) {
	ctx := context.Background()
	repo := initGitWorkspaceRepo(t)
	ws, err := NewGitWorkspace(repo)
	require.NoError(t, err)

	got, err := ws.CurrentRef(ctx)

	require.NoError(t, err)
	want := strings.TrimSpace(gitWorkspaceCmd(t, repo, "rev-parse", "HEAD"))
	require.Equal(t, want, got)
}

func TestGitWorkspaceErrorsOutsideGitRepo(t *testing.T) {
	dir := t.TempDir()

	_, err := NewGitWorkspace(dir)

	require.Error(t, err)
	require.Contains(t, err.Error(), "git rev-parse --show-toplevel")
	require.Contains(t, err.Error(), dir)
}

func TestGitWorkspaceRefusesSubdirectoryRoot(t *testing.T) {
	repo := initGitWorkspaceRepo(t)
	subdir := filepath.Join(repo, "subdir")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	_, err := NewGitWorkspace(subdir)

	require.Error(t, err)
	require.Contains(t, err.Error(), "expected git top-level")
	require.Contains(t, err.Error(), subdir)
}

func TestGitWorkspaceRestoreErrorsMentionCommandAndPath(t *testing.T) {
	repo := initGitWorkspaceRepo(t)
	ws, err := NewGitWorkspace(repo)
	require.NoError(t, err)

	err = ws.Restore(context.Background(), "missing-ref")

	require.Error(t, err)
	require.Contains(t, err.Error(), "git reset --hard missing-ref")
	require.Contains(t, err.Error(), repo)
}

func initGitWorkspaceRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	gitWorkspaceCmd(t, repo, "init")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("initial\n"), 0o644))
	gitWorkspaceCmd(t, repo, "add", "-A")
	gitWorkspaceCmd(t, repo, "-c", "user.name=agent-core", "-c", "user.email=agent-core@example.invalid", "commit", "-m", "initial")
	return repo
}

func gitWorkspaceCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v in %s failed: %s", args, dir, string(out))
	return string(out)
}
