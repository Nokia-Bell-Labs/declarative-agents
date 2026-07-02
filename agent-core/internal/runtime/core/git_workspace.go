// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitWorkspace uses git commits as opaque workspace refs.
//
// It is intentionally opt-in: callers must construct and pass it through
// LoopParams or tool builders. The configured directory must be the git
// repository top-level so restore cannot accidentally reset a larger parent
// repository than the caller intended.
type GitWorkspace struct {
	Dir string
}

// NewGitWorkspace validates dir and returns a Workspace backed by the git repo
// rooted at that directory.
func NewGitWorkspace(dir string) (*GitWorkspace, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("git workspace %q: resolve path: %w", dir, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("git workspace %q: resolve symlinks: %w", dir, err)
	}
	if err := verifyGitWorkspaceRoot(context.Background(), resolved); err != nil {
		return nil, err
	}
	return &GitWorkspace{Dir: resolved}, nil
}

// Checkpoint stages all workspace changes and creates an allow-empty commit.
// The returned ref is the new HEAD commit SHA.
func (g *GitWorkspace) Checkpoint(ctx context.Context, label string) (string, error) {
	if err := g.verify(ctx); err != nil {
		return "", err
	}
	message := "checkpoint"
	if label != "" {
		message += ": " + label
	}
	if _, err := g.runGit(ctx, "add", "-A"); err != nil {
		return "", err
	}
	if _, err := g.runGit(ctx, "-c", "user.name=agent-core", "-c", "user.email=agent-core@example.invalid", "commit", "--allow-empty", "-m", message); err != nil {
		return "", err
	}
	return g.CurrentRef(ctx)
}

// Restore resets the configured git workspace to ref.
func (g *GitWorkspace) Restore(ctx context.Context, ref string) error {
	if ref == "" {
		return fmt.Errorf("git workspace %q: restore requires non-empty ref", g.Dir)
	}
	if err := g.verify(ctx); err != nil {
		return err
	}
	_, err := g.runGit(ctx, "reset", "--hard", ref)
	return err
}

// CurrentRef returns the current HEAD commit SHA.
func (g *GitWorkspace) CurrentRef(ctx context.Context) (string, error) {
	if err := g.verify(ctx); err != nil {
		return "", err
	}
	out, err := g.runGit(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (g *GitWorkspace) verify(ctx context.Context) error {
	if g == nil || g.Dir == "" {
		return fmt.Errorf("git workspace: missing directory")
	}
	return verifyGitWorkspaceRoot(ctx, g.Dir)
}

func verifyGitWorkspaceRoot(ctx context.Context, dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("git workspace %q: stat: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("git workspace %q: not a directory", dir)
	}
	top, err := runGitInDir(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	top = strings.TrimSpace(top)
	absTop, err := filepath.Abs(top)
	if err != nil {
		return fmt.Errorf("git workspace %q: resolve git top-level %q: %w", dir, top, err)
	}
	resolvedTop, err := filepath.EvalSymlinks(absTop)
	if err != nil {
		return fmt.Errorf("git workspace %q: resolve git top-level symlinks %q: %w", dir, absTop, err)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("git workspace %q: resolve path: %w", dir, err)
	}
	resolvedDir, err := filepath.EvalSymlinks(absDir)
	if err != nil {
		return fmt.Errorf("git workspace %q: resolve symlinks: %w", dir, err)
	}
	if filepath.Clean(resolvedTop) != filepath.Clean(resolvedDir) {
		return fmt.Errorf("git workspace %q: expected git top-level %q", dir, resolvedTop)
	}
	return nil
}

func (g *GitWorkspace) runGit(ctx context.Context, args ...string) (string, error) {
	return runGitInDir(ctx, g.Dir, args...)
}

func runGitInDir(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s in %s: %s: %w", strings.Join(args, " "), dir, strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

var _ Workspace = (*GitWorkspace)(nil)
