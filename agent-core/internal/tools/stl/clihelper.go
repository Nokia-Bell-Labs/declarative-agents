// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RunCLI executes a CLI command in dir, capturing stdout and returning it.
// Stderr is captured and included in the error if the command fails.
// Pass an empty dir to inherit the working directory.
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

// RunGit executes a git command in dir.
func RunGit(ctx context.Context, dir string, args ...string) (string, error) {
	return RunCLI(ctx, dir, "git", args...)
}

// RunBd executes a bd (beads) command in dir.
func RunBd(ctx context.Context, dir string, args ...string) (string, error) {
	return RunCLI(ctx, dir, "bd", args...)
}

// VerifyGitDir checks that dir contains a .git entry (file or directory).
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
