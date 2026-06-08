// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package execute invokes a child agent binary as a subprocess
// with OTel trace propagation.
package execute

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/yaml.v3"

	agentllm "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/stl"
	agenttel "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/telemetry"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
)

const (
	spanExecute    = "execute_task"
	defaultBinary  = "generator"
	defaultTimeout = 10 * time.Minute
)

// Config holds execution engine settings.
type Config struct {
	Binary    string        // Agent binary path. Default: "generator" (resolved from PATH).
	Model     string        // LLM model name passed via --model.
	OllamaURL string        // Ollama server URL passed via --ollama-url.
	Timeout   time.Duration // Per-invocation timeout. Default: 10 minutes.
	OTelDir   string        // Directory for temporary OTel log files.
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
	return defaultTimeout
}

// Result captures the outcome of an agent invocation.
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

// Success returns true when the agent exited with code 0.
func (r *Result) Success() bool { return r.ExitCode == 0 }

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
		attribute.String("generator.model", cfg.Model),
		attribute.String("generator.timeout", cfg.timeout().String()),
	)
	defer done()

	promptFile, err := writePromptFile(cfg.OTelDir, taskID, plan)
	if err != nil {
		child.RecordError(err)
		return nil, fmt.Errorf("execute %s: write prompt: %w", taskID, err)
	}
	defer os.Remove(promptFile)

	otelLogFile := filepath.Join(otelDir(cfg), fmt.Sprintf("generator-%s.otel.json", taskID))

	args := []string{
		"--prompt", promptFile,
		"--directory", worktreeDir,
		"--model", cfg.Model,
		"--otel-log-file", otelLogFile,
	}
	if cfg.OllamaURL != "" {
		args = append(args, "--ollama-url", cfg.OllamaURL)
	}

	sc := trace.SpanFromContext(child.Context()).SpanContext()
	if tp := agenttel.FormatTraceparent(sc); tp != "" {
		args = append(args, "--otel-parent-span", tp)
	}

	start := time.Now()
	tctx, cancel := context.WithTimeout(ctx, cfg.timeout())
	defer cancel()

	cmd := exec.CommandContext(tctx, cfg.binary(), args...)
	stl.ProcGroupCmd(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	elapsed := time.Since(start)

	result := &Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: elapsed,
	}

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			child.RecordError(runErr)
			return nil, fmt.Errorf("execute %s: %w", taskID, runErr)
		}
	}

	if result.ExitCode != 0 {
		child.SetAttributes(
			attribute.Int("exit_code", result.ExitCode),
			attribute.String("stderr", agentllm.Truncate(result.Stderr, 4096)),
		)
		child.RecordError(fmt.Errorf("generator exited %d", result.ExitCode))
	}

	return result, nil
}

func writePromptFile(dir, taskID string, plan any) (string, error) {
	data, err := yaml.Marshal(plan)
	if err != nil {
		return "", fmt.Errorf("marshal plan: %w", err)
	}
	d := dir
	if d == "" {
		d = os.TempDir()
	}
	f, err := os.CreateTemp(d, fmt.Sprintf("agent-prompt-%s-*.yaml", taskID))
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

func otelDir(cfg Config) string {
	if cfg.OTelDir != "" {
		return cfg.OTelDir
	}
	return os.TempDir()
}
