// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/execute"
)

func TestSelfInvokeBuilder_Build(t *testing.T) {
	t.Parallel()
	builder := &SelfInvokeBuilder{
		Config: execute.Config{
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
		Config: execute.Config{
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
		Config: execute.Config{
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

func TestSelfInvokeBuilder_Execute_BinaryNotFound(t *testing.T) {
	t.Parallel()
	builder := &SelfInvokeBuilder{
		Config: execute.Config{
			Binary:  "/nonexistent/agent",
			Timeout: 5 * time.Second,
		},
		Ctx: context.Background(),
	}

	cmd := builder.Build(core.Result{
		Output: `{"parameters":{"run_id":"exec-err"}}`,
	})
	result := cmd.Execute()

	assert.Equal(t, core.ToolFailed, result.Signal)
}

func TestSelfInvokeBuilder_ExtraArgs(t *testing.T) {
	t.Parallel()
	builder := &SelfInvokeBuilder{
		Config: execute.Config{
			Binary:  "echo",
			Machine: "m.yaml",
			Timeout: 5 * time.Second,
		},
		ExtraArgs: []string{"--directory", "/workspace"},
		Ctx:       context.Background(),
	}

	cmd := builder.Build(core.Result{
		Output: `{"parameters":{"run_id":"extra-test"}}`,
	})
	result := cmd.Execute()

	assert.Equal(t, core.ToolDone, result.Signal)
	assert.Contains(t, result.Output, "--directory")
	assert.Contains(t, result.Output, "/workspace")
}

func TestSelfInvokeToolSpec(t *testing.T) {
	t.Parallel()
	spec := SelfInvokeToolSpec()

	assert.Equal(t, "self_invoke", spec.Name)
	assert.Equal(t, core.Internal, spec.Visibility)
}
