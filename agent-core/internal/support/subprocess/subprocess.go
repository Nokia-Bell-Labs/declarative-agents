// Copyright (c) 2026 Nokia. All rights reserved.

// Package subprocess provides a unified interface for invoking the agent
// binary (or other binaries) as a child process with OTel propagation,
// timeout handling, environment variables, and process group management.
//
// All child agent invocations (execute.RunAgent, execute.Execute) and exec
// tools share this foundation.
package subprocess

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry"
)

const defaultWaitDelay = 3 * time.Second

// Result captures the outcome of a subprocess invocation.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
	TimedOut bool
	Err      error
}

func (r *Result) Success() bool { return r.ExitCode == 0 && r.Err == nil }

// Spec describes how to run a subprocess.
type Spec struct {
	Binary  string
	Args    []string
	Dir     string
	Env     []string // additional env vars (appended to os.Environ)
	Timeout time.Duration

	PropagateOTel bool // append --otel-parent-span from ctx
}

// RunCLIOutput runs a CLI and returns stdout, using stderr as the error text
// when the command fails.
func RunCLIOutput(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	setProcGroup(cmd)

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

// Run executes a subprocess with process-group management.
func Run(ctx context.Context, spec Spec) *Result {
	timeout := spec.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}

	args := spec.Args
	if spec.PropagateOTel {
		sc := trace.SpanFromContext(ctx).SpanContext()
		if sc.IsValid() {
			tp := telemetry.FormatTraceparent(sc)
			if tp != "" {
				args = append(args, "--otel-parent-span", tp)
			}
		}
	}

	childCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(childCtx, spec.Binary, args...)
	cmd.Dir = spec.Dir
	setProcGroup(cmd)

	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}

	start := time.Now()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	elapsed := time.Since(start)

	result := &Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: elapsed,
	}

	if err != nil {
		if childCtx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
			result.ExitCode = -1
		} else if childCtx.Err() != nil {
			result.ExitCode = -1
			result.Err = childCtx.Err()
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
			result.Err = err
		}
	}

	return result
}

// EnvVar formats an environment variable assignment.
func EnvVar(key, value string) string {
	return fmt.Sprintf("%s=%s", key, value)
}

// EnvVarInt formats an integer environment variable assignment.
func EnvVarInt(key string, value int) string {
	return fmt.Sprintf("%s=%d", key, value)
}

func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = defaultWaitDelay
}
