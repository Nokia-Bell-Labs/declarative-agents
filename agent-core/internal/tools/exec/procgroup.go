// Copyright (c) 2026 Nokia. All rights reserved.

package exec

import (
	"bytes"
	"context"
	osexec "os/exec"
	"syscall"
	"time"
)

const defaultWaitDelay = 3 * time.Second

// ProcGroupCmd configures cmd to run in its own process group.
func ProcGroupCmd(cmd *osexec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = defaultWaitDelay
}

// RunResult captures stdout, stderr, exit code, and elapsed time.
type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
	Err      error
}

// Success returns true when the subprocess exited with code 0.
func (r *RunResult) Success() bool { return r.ExitCode == 0 && r.Err == nil }

// RunProcGroup runs a process-group-managed subprocess within timeout.
func RunProcGroup(ctx context.Context, timeout time.Duration, dir string, name string, args ...string) *RunResult {
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := osexec.CommandContext(tctx, name, args...)
	cmd.Dir = dir
	ProcGroupCmd(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	return runProcGroupResult(stdout.String(), stderr.String(), time.Since(start), runErr)
}

func runProcGroupResult(stdout, stderr string, elapsed time.Duration, runErr error) *RunResult {
	result := &RunResult{Stdout: stdout, Stderr: stderr, Duration: elapsed}
	if runErr == nil {
		return result
	}
	if exitErr, ok := runErr.(*osexec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		return result
	}
	result.ExitCode = -1
	result.Err = runErr
	return result
}
