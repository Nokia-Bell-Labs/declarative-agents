// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package worktree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/telemetry"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repo := filepath.Join(dir, "test-repo")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	gitCmd(t, repo, "init")
	gitCmd(t, repo, "config", "user.email", "test@test.com")
	gitCmd(t, repo, "config", "user.name", "test")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("# test\n"), 0o644))
	gitCmd(t, repo, "add", ".")
	gitCmd(t, repo, "commit", "-m", "initial")
	return repo
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
}

func checkedOutBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func TestCreateProducesWorktreeDirectory(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)

	wt, err := Create(context.Background(), tracing.NoopTracer{}, repo, "20260604-abc")
	require.NoError(t, err)
	t.Cleanup(func() { _ = wt.Remove(context.Background(), tracing.NoopTracer{}) })

	assert.DirExists(t, wt.Dir)
	assert.Equal(t, repo, wt.RepoDir)
	assert.Equal(t, "agent/20260604-abc", wt.Branch)
	assert.Equal(t, "agent/20260604-abc", checkedOutBranch(t, wt.Dir))
	assert.Equal(t, filepath.Dir(repo), filepath.Dir(wt.Dir),
		"worktree should be a sibling of the repo")
}

func TestBranchNameMatchesConvention(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)

	wt, err := Create(context.Background(), tracing.NoopTracer{}, repo, "run-42")
	require.NoError(t, err)
	t.Cleanup(func() { _ = wt.Remove(context.Background(), tracing.NoopTracer{}) })

	assert.Equal(t, "agent/run-42", wt.Branch)
}

func TestCreateFailsOnNonGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	notGit := filepath.Join(dir, "not-a-repo")
	require.NoError(t, os.MkdirAll(notGit, 0o755))

	_, err := Create(context.Background(), tracing.NoopTracer{}, notGit, "run-1")
	require.Error(t, err)

	var target *NotAGitRepoError
	require.True(t, errors.As(err, &target), "expected NotAGitRepoError, got %T", err)
	assert.Contains(t, target.Path, "not-a-repo")
}

func TestCreateFailsOnExistingBranch(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	gitCmd(t, repo, "branch", "agent/existing")

	_, err := Create(context.Background(), tracing.NoopTracer{}, repo, "existing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestRemoveDeletesWorktreeAndBranch(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)

	wt, err := Create(context.Background(), tracing.NoopTracer{}, repo, "to-remove")
	require.NoError(t, err)
	assert.DirExists(t, wt.Dir)

	require.NoError(t, wt.Remove(context.Background(), tracing.NoopTracer{}))

	assert.NoDirExists(t, wt.Dir)

	cmd := exec.Command("git", "branch", "--list", wt.Branch)
	cmd.Dir = repo
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(out)),
		"branch should be deleted after Remove")
}

func TestRemoveIsIdempotent(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)

	wt, err := Create(context.Background(), tracing.NoopTracer{}, repo, "idem")
	require.NoError(t, err)

	require.NoError(t, wt.Remove(context.Background(), tracing.NoopTracer{}), "first Remove")
	require.NoError(t, wt.Remove(context.Background(), tracing.NoopTracer{}), "second Remove")
}

func TestErrorsIncludeRepoPathAndStderr(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	notGit := filepath.Join(dir, "my-project")
	require.NoError(t, os.MkdirAll(notGit, 0o755))

	_, err := Create(context.Background(), tracing.NoopTracer{}, notGit, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), notGit, "error should contain repo path")

	repo := initTestRepo(t)
	gitCmd(t, repo, "branch", "agent/dup")
	_, err = Create(context.Background(), tracing.NoopTracer{}, repo, "dup")
	require.Error(t, err)
	assert.Contains(t, err.Error(), repo, "error should contain repo path")
	assert.Contains(t, err.Error(), "agent/dup")
}

func TestCreateAndRemoveEmitOTelSpans(t *testing.T) {
	tracer, exporter := telemetry.NewTestTracer(t, "test")

	repo := initTestRepo(t)

	wt, err := Create(context.Background(), tracer, repo, "otel-test")
	require.NoError(t, err)

	require.NoError(t, wt.Remove(context.Background(), tracer))

	spans := exporter.GetSpans()

	var createSpan, removeSpan *tracetest.SpanStub
	for i := range spans {
		switch spans[i].Name {
		case "worktree.create":
			createSpan = &spans[i]
		case "worktree.remove":
			removeSpan = &spans[i]
		}
	}

	require.NotNil(t, createSpan, "expected worktree.create span")
	require.NotNil(t, removeSpan, "expected worktree.remove span")

	assertSpanHasAttr(t, createSpan, "repo.path", repo)
	assertSpanHasAttr(t, createSpan, "worktree.branch", "agent/otel-test")
	assertSpanHasAttr(t, createSpan, "worktree.dir", wt.Dir)

	assertSpanHasAttr(t, removeSpan, "repo.path", repo)
	assertSpanHasAttr(t, removeSpan, "worktree.branch", "agent/otel-test")
	assertSpanHasAttr(t, removeSpan, "worktree.dir", wt.Dir)
}

func assertSpanHasAttr(t *testing.T, span *tracetest.SpanStub, key, want string) {
	t.Helper()
	for _, a := range span.Attributes {
		if string(a.Key) == key {
			assert.Equal(t, want, a.Value.AsString(),
				"span %s attr %s", span.Name, key)
			return
		}
	}
	t.Errorf("span %s missing attribute %s", span.Name, key)
}
