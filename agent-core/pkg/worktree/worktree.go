// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package worktree manages git worktrees for agent runs.
// Implements branch-level isolation via git worktree add/remove
// with deterministic branch naming.
package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/stl"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
)

const (
	branchPrefix = "agent/"
	spanCreate   = "worktree.create"
	spanRemove   = "worktree.remove"
)

// NotAGitRepoError indicates the target path lacks a .git directory.
type NotAGitRepoError struct {
	Path string
}

func (e *NotAGitRepoError) Error() string {
	return fmt.Sprintf("not a git repository: %s", e.Path)
}

// GitError wraps a failed git command with repo path and stderr.
type GitError struct {
	RepoDir string
	Args    []string
	Stderr  string
	Err     error
}

func (e *GitError) Error() string {
	op := strings.Join(e.Args, " ")
	return fmt.Sprintf("git %s in %s: %s", op, e.RepoDir, strings.TrimSpace(e.Stderr))
}

func (e *GitError) Unwrap() error { return e.Err }

// Worktree holds paths and branch name for a git worktree.
type Worktree struct {
	RepoDir string
	Branch  string
	Dir     string
}

// Create creates a git worktree with branch agent/<runID> as a sibling
// of repoDir. Returns the populated Worktree on success.
func Create(_ context.Context, tracer tracing.Tracer, repoDir, runID string) (*Worktree, error) {
	branch := branchPrefix + runID
	dir := worktreeDir(repoDir, runID)

	child, done := tracer.Push(spanCreate, spanAttrs(repoDir, branch, dir)...)
	defer done()

	if err := verifyGitRepo(repoDir); err != nil {
		child.RecordError(err)
		return nil, err
	}

	exists, err := branchExists(repoDir, branch)
	if err != nil {
		child.RecordError(err)
		return nil, err
	}
	if exists {
		err := fmt.Errorf("create worktree in %s: branch %s already exists", repoDir, branch)
		child.RecordError(err)
		return nil, err
	}

	if _, err := runGit(repoDir, "worktree", "add", "-b", branch, dir); err != nil {
		child.RecordError(err)
		return nil, fmt.Errorf("create worktree in %s: %w", repoDir, err)
	}

	return &Worktree{RepoDir: repoDir, Branch: branch, Dir: dir}, nil
}

// Remove deletes the worktree directory and its git metadata, then
// best-effort deletes the branch. Idempotent: succeeds if already removed.
func (w *Worktree) Remove(_ context.Context, tracer tracing.Tracer) error {
	child, done := tracer.Push(spanRemove, spanAttrs(w.RepoDir, w.Branch, w.Dir)...)
	defer done()

	if _, err := os.Stat(w.Dir); os.IsNotExist(err) {
		return nil
	}

	if _, err := runGit(w.RepoDir, "worktree", "remove", "--force", w.Dir); err != nil {
		child.RecordError(err)
		return fmt.Errorf("remove worktree in %s: %w", w.RepoDir, err)
	}

	// Best-effort branch delete; failure is non-fatal.
	_, _ = runGit(w.RepoDir, "branch", "-D", w.Branch)

	return nil
}

// --- helpers ---

func worktreeDir(repoDir, runID string) string {
	clean := filepath.Clean(repoDir)
	return clean + "-agent-" + runID
}

func verifyGitRepo(dir string) error {
	if err := stl.VerifyGitDir(dir); err != nil {
		if _, statErr := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(statErr) {
			return &NotAGitRepoError{Path: dir}
		}
		return err
	}
	return nil
}

func branchExists(repoDir, branch string) (bool, error) {
	out, err := runGit(repoDir, "branch", "--list", branch)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func runGit(repoDir string, args ...string) (string, error) {
	out, err := stl.RunGit(context.Background(), repoDir, args...)
	if err != nil {
		return "", &GitError{
			RepoDir: repoDir,
			Args:    args,
			Stderr:  err.Error(),
			Err:     err,
		}
	}
	return out, nil
}

func spanAttrs(repoDir, branch, dir string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("repo.path", repoDir),
		attribute.String("worktree.branch", branch),
		attribute.String("worktree.dir", dir),
	}
}
