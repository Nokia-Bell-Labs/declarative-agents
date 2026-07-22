// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package execute

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	agentllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	agenttel "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

func TestFormatTraceparent(t *testing.T) {
	t.Run("valid span context", func(t *testing.T) {
		traceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
		spanID, _ := trace.SpanIDFromHex("b7ad6b7169203331")
		sc := trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    traceID,
			SpanID:     spanID,
			TraceFlags: trace.FlagsSampled,
			Remote:     true,
		})
		got := agenttel.FormatTraceparent(sc)
		assert.Equal(t, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", got)
	})

	t.Run("invalid span context", func(t *testing.T) {
		sc := trace.SpanContext{}
		got := agenttel.FormatTraceparent(sc)
		assert.Equal(t, "", got)
	})
}

func TestWriteTaskFile(t *testing.T) {
	dir := t.TempDir()
	plan := map[string]string{"title": "test plan"}
	path := filepath.Join(dir, "doc", "task.yaml")

	err := writeTaskFile(path, plan)
	require.NoError(t, err)

	assert.FileExists(t, path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "title: test plan")
}

func TestWriteTaskFile_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "doc", "task.yaml")

	err := writeTaskFile(path, map[string]string{"key": "value"})
	require.NoError(t, err)
	assert.FileExists(t, path)
}

func TestTruncateViaAgentCore(t *testing.T) {
	assert.Equal(t, "abc", agentllm.Truncate("abc", 10))
	assert.Equal(t, "", agentllm.Truncate("", 5))
	result := agentllm.Truncate("abcdef", 2)
	assert.Equal(t, "ab", result[:2])
	assert.Contains(t, result, "...")
}

func TestExecute_BinaryNotFound(t *testing.T) {
	cfg := Config{
		Binary:  "nonexistent-generator-binary-xyz",
		Timeout: 5 * time.Second,
		OTelDir: t.TempDir(),
	}

	result, err := Execute(context.Background(), tracing.NoopTracer{}, cfg, "task-1", t.TempDir(), map[string]string{"title": "test"})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "execute task-1")
}

func TestExecute_ScriptExitNonZero(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-gen.sh")
	err := os.WriteFile(script, []byte("#!/bin/sh\necho 'stdout-output'\necho 'stderr-output' >&2\nexit 1\n"), 0o755)
	require.NoError(t, err)

	cfg := Config{
		Binary:  script,
		Timeout: 5 * time.Second,
		OTelDir: dir,
	}

	result, err := Execute(context.Background(), tracing.NoopTracer{}, cfg, "fail-task", dir, map[string]string{"title": "fail"})
	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode)
	assert.False(t, result.Success())
	assert.Contains(t, result.Stdout, "stdout-output")
	assert.Contains(t, result.Stderr, "stderr-output")
	assert.NotContains(t, result.Stdout, "stderr-output")
	assert.True(t, result.Duration > 0)
}

func TestExecute_ScriptExitZero(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ok-gen.sh")
	err := os.WriteFile(script, []byte("#!/bin/sh\necho 'done'\n"), 0o755)
	require.NoError(t, err)

	cfg := Config{
		Binary:  script,
		Timeout: 5 * time.Second,
		OTelDir: dir,
	}

	result, err := Execute(context.Background(), tracing.NoopTracer{}, cfg, "ok-task", dir, map[string]string{"title": "ok"})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.True(t, result.Success())
	assert.Contains(t, result.Stdout, "done")
}

func TestExecute_Timeout(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "slow-gen.sh")
	err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755)
	require.NoError(t, err)

	cfg := Config{
		Binary:  script,
		Timeout: 500 * time.Millisecond,
		OTelDir: dir,
	}

	result, err := Execute(context.Background(), tracing.NoopTracer{}, cfg, "slow-task", dir, map[string]string{"title": "slow"})
	require.NoError(t, err)
	assert.NotEqual(t, 0, result.ExitCode)
	assert.False(t, result.Success())
}

func TestExecute_TaskFileWritten(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "check-gen.sh")
	err := os.WriteFile(script, []byte("#!/bin/sh\necho ok\n"), 0o755)
	require.NoError(t, err)

	worktree := t.TempDir()
	cfg := Config{
		Binary:  script,
		Timeout: 5 * time.Second,
		OTelDir: dir,
	}

	_, err = Execute(context.Background(), tracing.NoopTracer{}, cfg, "cleanup-task", worktree, map[string]string{"title": "cleanup"})
	require.NoError(t, err)

	taskFile := filepath.Join(worktree, "doc", "task.yaml")
	assert.FileExists(t, taskFile)
}

func TestConfigDefaults(t *testing.T) {
	c := Config{}
	assert.Equal(t, "agent", c.binary())
	assert.Equal(t, 10*time.Minute, c.timeout())

	c2 := Config{Binary: "my-gen", Timeout: 3 * time.Minute}
	assert.Equal(t, "my-gen", c2.binary())
	assert.Equal(t, 3*time.Minute, c2.timeout())
}

func TestResultSuccess(t *testing.T) {
	assert.True(t, (&Result{ExitCode: 0}).Success())
	assert.False(t, (&Result{ExitCode: 1}).Success())
	assert.False(t, (&Result{ExitCode: -1}).Success())
}

func TestBuildArgs_ProfileOnly(t *testing.T) {
	cfg := Config{
		Profile: "agents/executor/profile.yaml",
	}

	args := cfg.BuildArgs()
	assert.Equal(t, []string{
		"--profile", "agents/executor/profile.yaml",
	}, args)
}

func TestBuildArgs_ChildRuntimeData(t *testing.T) {
	cfg := Config{
		Profile:     "agents/executor/profile.yaml",
		Directory:   "/workspace",
		Request:     "suite.yaml",
		Output:      "eval-results",
		OTelLogFile: "child.otel.json",
	}

	args := cfg.BuildArgs()

	assert.Equal(t, []string{
		"--profile", "agents/executor/profile.yaml",
		"--directory", "/workspace",
		"--request", "suite.yaml",
		"--output", "eval-results",
		"--otel-log-file", "child.otel.json",
	}, args)
}

func TestBuildArgs_Empty(t *testing.T) {
	cfg := Config{}
	args := cfg.BuildArgs()
	assert.Empty(t, args)
}

func TestRunAgent_Success(t *testing.T) {
	result := RunAgent(context.Background(), Config{
		Binary:  "echo",
		Timeout: 5 * time.Second,
	}, "hello")

	assert.Equal(t, 0, result.ExitCode)
	assert.True(t, result.Success())
	assert.Contains(t, result.Stdout, "hello")
}

func TestRunAgent_ExtraArgs(t *testing.T) {
	result := RunAgent(context.Background(), Config{
		Binary:  "echo",
		Profile: "agents/executor/profile.yaml",
		Timeout: 5 * time.Second,
	}, "--directory", "/workspace")

	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "--profile")
	assert.Contains(t, result.Stdout, "agents/executor/profile.yaml")
	assert.Contains(t, result.Stdout, "--directory")
	assert.Contains(t, result.Stdout, "/workspace")
}

func TestRunAgent_Failure(t *testing.T) {
	result := RunAgent(context.Background(), Config{
		Binary:  "false",
		Timeout: 5 * time.Second,
	})

	assert.False(t, result.Success())
	assert.NotEqual(t, 0, result.ExitCode)
}
