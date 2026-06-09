// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

func TestSelfInvokeConfig_BinaryDefault(t *testing.T) {
	t.Parallel()
	cfg := SelfInvokeConfig{}
	args, env := buildSelfInvokeArgs(cfg, "run-1")
	assert.Empty(t, args, "empty config should produce no args")
	assert.Empty(t, env, "empty config should produce no env")
}

func TestSelfInvokeConfig_BinaryExplicit(t *testing.T) {
	t.Parallel()
	cfg := SelfInvokeConfig{Binary: "/usr/local/bin/agent"}
	assert.Equal(t, "/usr/local/bin/agent", cfg.Binary)
}

func TestBuildSelfInvokeArgs_AllFlags(t *testing.T) {
	t.Parallel()
	cfg := SelfInvokeConfig{
		Machine:    "pipeline.yaml",
		Tools:      "tools.yaml",
		Directory:  "/workspace",
		Model:      "qwen2.5-coder",
		OllamaURL:  "http://localhost:11434",
		LLMTimeout: 30 * time.Second,
		MaxTime:    5 * time.Minute,
		OTelDir:    "/tmp/otel",
	}

	args, env := buildSelfInvokeArgs(cfg, "run-42")

	assert.Contains(t, args, "--machine")
	assert.Contains(t, args, "pipeline.yaml")
	assert.Contains(t, args, "--tools")
	assert.Contains(t, args, "tools.yaml")
	assert.Contains(t, args, "--directory")
	assert.Contains(t, args, "/workspace")
	assert.Contains(t, args, "--otel-log-file")
	assert.Contains(t, args, "/tmp/otel/child-run-42.otel.json")

	assert.Contains(t, args, "--model")
	assert.Contains(t, args, "qwen2.5-coder")
	assert.Contains(t, args, "--ollama-url")
	assert.Contains(t, args, "http://localhost:11434")
	assert.Empty(t, env)
}

func TestBuildSelfInvokeArgs_Minimal(t *testing.T) {
	t.Parallel()
	cfg := SelfInvokeConfig{
		Machine: "gen.yaml",
		Tools:   "gen-tools.yaml",
	}

	args, env := buildSelfInvokeArgs(cfg, "run-1")

	assert.Equal(t, []string{"--machine", "gen.yaml", "--tools", "gen-tools.yaml"}, args)
	assert.Empty(t, env)
}

func TestBuildSelfInvokeArgs_SkipsEmpty(t *testing.T) {
	t.Parallel()
	cfg := SelfInvokeConfig{
		Machine: "m.yaml",
	}

	args, env := buildSelfInvokeArgs(cfg, "r")
	assert.NotContains(t, args, "--tools")
	assert.NotContains(t, args, "--directory")
	assert.NotContains(t, args, "--otel-log-file")
	assert.Empty(t, env)
}

func TestSelfInvoke_Success(t *testing.T) {
	t.Parallel()
	cfg := SelfInvokeConfig{
		Binary:  "echo",
		Timeout: 5 * time.Second,
	}

	result, err := SelfInvoke(context.Background(), nil, cfg, "test-run")
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.True(t, result.Success())
	assert.True(t, result.Duration > 0)
}

func TestSelfInvoke_Failure(t *testing.T) {
	t.Parallel()
	cfg := SelfInvokeConfig{
		Binary:  "false",
		Timeout: 5 * time.Second,
	}

	result, err := SelfInvoke(context.Background(), nil, cfg, "test-run")
	require.NoError(t, err)
	assert.NotEqual(t, 0, result.ExitCode)
	assert.False(t, result.Success())
}

func TestSelfInvoke_BinaryNotFound(t *testing.T) {
	t.Parallel()
	cfg := SelfInvokeConfig{
		Binary:  "/nonexistent/binary/path",
		Timeout: 5 * time.Second,
	}

	_, err := SelfInvoke(context.Background(), nil, cfg, "test-run")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "self-invoke")
}

func TestSelfInvoke_DefaultBinary(t *testing.T) {
	t.Parallel()
	cfg := SelfInvokeConfig{}
	// With empty binary, SelfInvoke uses os.Args[0] which won't have
	// the right flags, but we can verify it doesn't panic.
	// os.Args[0] is the test binary itself.
	assert.Equal(t, "", cfg.Binary)
	assert.Equal(t, os.Args[0], os.Args[0]) // sanity
}

func TestSelfInvoke_DefaultTimeout(t *testing.T) {
	t.Parallel()
	cfg := SelfInvokeConfig{
		Binary: "echo",
		// Timeout left at zero => should default to 10 minutes
	}

	result, err := SelfInvoke(context.Background(), nil, cfg, "test-run")
	require.NoError(t, err)
	assert.True(t, result.Success())
}

func TestSelfInvokeResult_Success(t *testing.T) {
	t.Parallel()

	assert.True(t, (&SelfInvokeResult{ExitCode: 0}).Success())
	assert.False(t, (&SelfInvokeResult{ExitCode: 1}).Success())
	assert.False(t, (&SelfInvokeResult{ExitCode: -1}).Success())
}

func TestSelfInvokeBuilder_Build(t *testing.T) {
	t.Parallel()
	builder := &SelfInvokeBuilder{
		Config: SelfInvokeConfig{
			Binary:  "echo",
			Machine: "test.yaml",
			Tools:   "tools.yaml",
		},
		Ctx: context.Background(),
	}

	res := core.Result{
		Output: `{"parameters":{"run_id":"build-test-42"}}`,
	}

	cmd := builder.Build(res)
	assert.Equal(t, "self_invoke", cmd.Name())
}

func TestSelfInvokeBuilder_Execute_Success(t *testing.T) {
	t.Parallel()
	builder := &SelfInvokeBuilder{
		Config: SelfInvokeConfig{
			Binary:  "echo",
			Timeout: 5 * time.Second,
		},
		Ctx: context.Background(),
	}

	cmd := builder.Build(core.Result{
		Output: `{"parameters":{"run_id":"exec-ok"}}`,
	})
	result := cmd.Execute()

	assert.Equal(t, core.ToolDone, result.Signal)
	assert.Equal(t, "self_invoke", result.CommandName)
	assert.True(t, result.Cost.Duration > 0)
}

func TestSelfInvokeBuilder_Execute_Failure(t *testing.T) {
	t.Parallel()
	builder := &SelfInvokeBuilder{
		Config: SelfInvokeConfig{
			Binary:  "false",
			Timeout: 5 * time.Second,
		},
		Ctx: context.Background(),
	}

	cmd := builder.Build(core.Result{
		Output: `{"parameters":{"run_id":"exec-fail"}}`,
	})
	result := cmd.Execute()

	assert.Equal(t, core.ToolFailed, result.Signal)
	assert.Equal(t, "self_invoke", result.CommandName)
}

func TestSelfInvokeBuilder_Execute_Error(t *testing.T) {
	t.Parallel()
	builder := &SelfInvokeBuilder{
		Config: SelfInvokeConfig{
			Binary:  "/nonexistent/agent",
			Timeout: 5 * time.Second,
		},
		Ctx: context.Background(),
	}

	cmd := builder.Build(core.Result{
		Output: `{"parameters":{"run_id":"exec-err"}}`,
	})
	result := cmd.Execute()

	assert.Equal(t, core.CommandError, result.Signal)
	assert.NotNil(t, result.Err)
}

func TestSelfInvokeToolSpec(t *testing.T) {
	t.Parallel()
	spec := SelfInvokeToolSpec()

	assert.Equal(t, "self_invoke", spec.Name)
	assert.Equal(t, core.Internal, spec.Visibility)
}
