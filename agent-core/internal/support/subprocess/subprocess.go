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
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry"
)

// DefaultWaitDelay bounds how long a killed process may hold its I/O pipes open
// before the runtime abandons the wait.
const DefaultWaitDelay = 3 * time.Second

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

	Stdin            string // written to the child's stdin, then closed; empty means no stdin
	CombinedOutput   bool   // merge stdout and stderr into Result.Stdout in write order
	NoDefaultTimeout bool   // run under ctx (and Timeout when positive) without the 10-minute default

	PropagateOTel bool // append --otel-parent-span from ctx
}

// RunCLIOutput runs a CLI and returns stdout, using stderr as the error text
// when the command fails.
func RunCLIOutput(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	SetProcGroup(cmd)

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

// Run executes a subprocess with process-group management. It is the single
// spawn primitive: exec tools, child agent invocations, and CLI-bound words all
// route through it rather than reimplementing capture, timeout, and proc-group
// handling (GH-447, GH-1393).
func Run(ctx context.Context, spec Spec) *Result {
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

	childCtx, cancel := spec.withTimeout(ctx)
	defer cancel()

	cmd := exec.CommandContext(childCtx, spec.Binary, args...)
	cmd.Dir = spec.Dir
	SetProcGroup(cmd)

	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}

	var stdout, stderr bytes.Buffer
	if spec.CombinedOutput {
		cmd.Stdout, cmd.Stderr = &stdout, &stdout
	} else {
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
	}

	start := time.Now()
	err := runCmd(cmd, spec.Stdin)
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

// withTimeout derives the child context. A spec may opt out of the 10-minute
// default (exec tools rely on the dispatch context for cancellation instead),
// while still honoring a positive Timeout as an upper bound.
func (s Spec) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.NoDefaultTimeout {
		if s.Timeout > 0 {
			return context.WithTimeout(ctx, s.Timeout)
		}
		return context.WithCancel(ctx)
	}
	timeout := s.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}
	return context.WithTimeout(ctx, timeout)
}

// runCmd runs cmd, streaming stdin from a goroutine when the spec provides it so
// a child that never drains its input cannot wedge the writer.
func runCmd(cmd *exec.Cmd, stdin string) error {
	if stdin == "" {
		return cmd.Run()
	}
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = pipe.Close()
		return err
	}
	written := make(chan struct{})
	go func() {
		_, _ = io.WriteString(pipe, stdin)
		_ = pipe.Close()
		close(written)
	}()
	waitErr := cmd.Wait()
	_ = pipe.Close()
	<-written
	return waitErr
}

// EnvVar formats an environment variable assignment.
func EnvVar(key, value string) string {
	return fmt.Sprintf("%s=%s", key, value)
}

// EnvVarInt formats an integer environment variable assignment.
func EnvVarInt(key string, value int) string {
	return fmt.Sprintf("%s=%d", key, value)
}

// SetProcGroup configures cmd to run in its own process group so cancellation
// kills the whole group rather than only the leader (srd013 R4.2).
func SetProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = DefaultWaitDelay
}
