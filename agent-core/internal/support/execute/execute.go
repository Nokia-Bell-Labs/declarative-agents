// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package execute invokes a child agent binary as a subprocess
// with OTel trace propagation.
package execute

import (
	"context"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/subprocess"
)

const defaultBinary = "agent"

// Config holds execution engine settings.
type Config struct {
	Binary          string        // Agent binary path. Default: "agent" (resolved from PATH).
	Profile         string        // --profile flag for the child agent.
	CoreRoot        string        // --core-root development install mapping for the child agent.
	Directory       string        // --directory flag for the child workspace.
	Request         string        // --request flag for runtime input.
	Output          string        // --output flag for runtime artifacts.
	OTelLogFile     string        // --otel-log-file flag for child trace capture.
	OTelServiceName string        // --otel-service-name identity for child spans.
	OTLPEndpoint    string        // --otel-otlp-endpoint destination for child spans.
	Timeout         time.Duration // Per-invocation timeout. Default: 10 minutes.
	OTelDir         string        // Directory for temporary OTel log files.
	Env             []string      // Additional KEY=VALUE vars for the child, appended to the parent environment.
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
	args = appendFlag(args, "--core-root", c.CoreRoot)
	args = appendFlag(args, "--directory", c.Directory)
	args = appendFlag(args, "--request", c.Request)
	args = appendFlag(args, "--output", c.Output)
	args = appendFlag(args, "--otel-log-file", c.OTelLogFile)
	args = appendFlag(args, "--otel-service-name", c.OTelServiceName)
	args = appendFlag(args, "--otel-otlp-endpoint", c.OTLPEndpoint)
	return args
}

// Result captures the outcome of an agent invocation.
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
	Started  bool
	TimedOut bool
	Err      error
}

// Success returns true when the agent exited with code 0.
func (r *Result) Success() bool { return r.ExitCode == 0 && r.Err == nil }

// RunAgent invokes the agent binary with base args from cfg plus any extra args.
// File materialization and workflow sequencing remain separate machine words.
func RunAgent(ctx context.Context, cfg Config, extraArgs ...string) *Result {
	r := subprocess.Run(ctx, cfg.subprocessSpec(extraArgs...))
	return &Result{
		Stdout:   r.Stdout,
		Stderr:   r.Stderr,
		ExitCode: r.ExitCode,
		Duration: r.Duration,
		Started:  r.Started,
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
