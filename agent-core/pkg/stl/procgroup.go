// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"bytes"
	"context"
	"os/exec"
	"syscall"
	"time"
)

const defaultWaitDelay = 3 * time.Second

// ProcGroupCmd sets up cmd to run in its own process group so that the
// entire tree is killed on context cancellation rather than just the
// lead process. WaitDelay defaults to 3 seconds.
func ProcGroupCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = defaultWaitDelay
}

// RunResult captures stdout, stderr, exit code, and elapsed time from
// a process-group-managed subprocess invocation.
type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
	Err      error
}

// Success returns true when the subprocess exited with code 0.
func (r *RunResult) Success() bool { return r.ExitCode == 0 && r.Err == nil }

// RunProcGroup creates a command with process-group management, runs it
// within the given timeout, and returns a RunResult. The command inherits
// the provided context for cancellation.
func RunProcGroup(ctx context.Context, timeout time.Duration, dir string, name string, args ...string) *RunResult {
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(tctx, name, args...)
	cmd.Dir = dir
	ProcGroupCmd(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)

	result := &RunResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: elapsed,
	}

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
			result.Err = runErr
		}
	}

	return result
}
