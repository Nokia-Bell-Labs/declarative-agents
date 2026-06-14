// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RunCLI is a compatibility wrapper for legacy callers that still execute
// simple CLIs through stl. Prefer a domain package or support package for new
// subprocess behavior.
func RunCLI(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		se := strings.TrimSpace(stderr.String())
		if se != "" {
			return "", fmt.Errorf("%s", se)
		}
		return "", err
	}
	return string(out), nil
}

// RunGit is a compatibility wrapper for legacy git callers.
func RunGit(ctx context.Context, dir string, args ...string) (string, error) {
	return RunCLI(ctx, dir, "git", args...)
}

// RunBd is a compatibility wrapper for legacy bd callers.
func RunBd(ctx context.Context, dir string, args ...string) (string, error) {
	return RunCLI(ctx, dir, "bd", args...)
}

// VerifyGitDir is a compatibility wrapper for legacy git workspace checks.
func VerifyGitDir(dir string) error {
	gitPath := dir + "/.git"
	if _, err := os.Stat(gitPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("not a git repository: %s", dir)
		}
		return fmt.Errorf("checking git repo %s: %v", dir, err)
	}
	return nil
}
