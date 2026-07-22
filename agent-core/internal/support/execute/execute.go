// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package execute invokes a child agent binary as a subprocess
// with OTel trace propagation.
package execute

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"gopkg.in/yaml.v3"

	agentllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/subprocess"
)

const (
	spanExecute   = "execute_task"
	defaultBinary = "agent"

	// TaskFilePath is the relative path under the worktree where the
	// task plan YAML is written before spawning a child agent.
	TaskFilePath = "doc/task.yaml"
)

// Config holds execution engine settings.
type Config struct {
	Binary      string        // Agent binary path. Default: "agent" (resolved from PATH).
	Profile     string        // --profile flag for the child agent.
	Directory   string        // --directory flag for the child workspace.
	Request     string        // --request flag for runtime input.
	Output      string        // --output flag for runtime artifacts.
	OTelLogFile string        // --otel-log-file flag for child trace capture.
	Timeout     time.Duration // Per-invocation timeout. Default: 10 minutes.
	OTelDir     string        // Directory for temporary OTel log files.
	Env         []string      // Additional KEY=VALUE vars for the child, appended to the parent environment.
}

func (c *Config) binary() string {
	if c.Binary != "" {
		return c.Binary
	}
	return defaultBinary
}

func (c *Config) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 10 * time.Minute
}

// BuildArgs constructs the CLI argument list from the config fields.
func (c *Config) BuildArgs() []string {
	var args []string
	if c.Profile != "" {
		args = append(args, "--profile", c.Profile)
	}
	args = appendFlag(args, "--directory", c.Directory)
	args = appendFlag(args, "--request", c.Request)
	args = appendFlag(args, "--output", c.Output)
	args = appendFlag(args, "--otel-log-file", c.OTelLogFile)
	return args
}

// Result captures the outcome of an agent invocation.
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
	TimedOut bool
	Err      error
}

// Success returns true when the agent exited with code 0.
func (r *Result) Success() bool { return r.ExitCode == 0 && r.Err == nil }

// Execute invokes the agent binary with the given plan written to a
// temporary YAML file. The worktreeDir is passed via --directory and the
// taskID is used for span attributes and temp file naming.
//
// The caller's context carries the OTel span; Execute formats it as a
// W3C traceparent and passes it via --otel-parent-span.
func Execute(ctx context.Context, tracer tracing.Tracer, cfg Config, taskID, worktreeDir string, plan any) (*Result, error) {
	child, done := tracer.Push(spanExecute,
		attribute.String("task.id", taskID),
		attribute.String("generator.binary", cfg.binary()),
		attribute.String("generator.timeout", cfg.timeout().String()),
	)
	defer done()

	taskFile := filepath.Join(worktreeDir, TaskFilePath)
	if err := writeTaskFile(taskFile, plan); err != nil {
		child.RecordError(err)
		return nil, fmt.Errorf("execute %s: write task file: %w", taskID, err)
	}

	otelLogFile := filepath.Join(otelDir(cfg), fmt.Sprintf("agent-%s.otel.json", taskID))

	cfg.Directory = worktreeDir
	cfg.OTelLogFile = otelLogFile
	result := RunAgent(child.Context(), cfg)

	if result.Err != nil {
		child.RecordError(result.Err)
		return nil, fmt.Errorf("execute %s: %w", taskID, result.Err)
	}

	if result.ExitCode != 0 {
		child.SetAttributes(
			attribute.Int("exit_code", result.ExitCode),
			attribute.String("stderr", agentllm.Truncate(result.Stderr, 4096)),
		)
		child.RecordError(fmt.Errorf("agent exited %d", result.ExitCode))
	}

	return result, nil
}

// RunAgent invokes the agent binary with base args from cfg plus any
// extra args. Unlike Execute, it does not write a task file. Suitable
// for launch_eval, self_invoke, and other child agent invocations.
func RunAgent(ctx context.Context, cfg Config, extraArgs ...string) *Result {
	r := subprocess.Run(ctx, cfg.subprocessSpec(extraArgs...))
	return &Result{
		Stdout:   r.Stdout,
		Stderr:   r.Stderr,
		ExitCode: r.ExitCode,
		Duration: r.Duration,
		TimedOut: r.TimedOut,
		Err:      r.Err,
	}
}

func appendFlag(args []string, name, value string) []string {
	if value == "" {
		return args
	}
	return append(args, name, value)
}

func (c Config) subprocessSpec(extraArgs ...string) subprocess.Spec {
	args := c.BuildArgs()
	args = append(args, extraArgs...)
	return subprocess.Spec{
		Binary:        c.binary(),
		Args:          args,
		Env:           c.Env,
		Timeout:       c.timeout(),
		PropagateOTel: true,
	}
}

func writeTaskFile(path string, plan any) error {
	data, err := yaml.Marshal(plan)
	if err != nil {
		return fmt.Errorf("marshal plan: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func otelDir(cfg Config) string {
	if cfg.OTelDir != "" {
		return cfg.OTelDir
	}
	return os.TempDir()
}
