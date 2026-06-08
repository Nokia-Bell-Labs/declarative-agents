// Package subprocess provides a unified interface for invoking the agent
// binary (or other binaries) as a child process with OTel propagation,
// timeout handling, environment variables, and process group management.
//
// All three call sites (stl.SelfInvoke, pkg/execute, eval/clitool) share
// this foundation.
package subprocess

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/trace"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/telemetry"
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
	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	result := &Result{
		Stdout:   string(output),
		Duration: elapsed,
	}

	if err != nil {
		if childCtx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
			result.ExitCode = -1
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
