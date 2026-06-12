// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/support/execute"
)

func TestSelfInvokeBuilder_Build(t *testing.T) {
	t.Parallel()
	builder := &SelfInvokeBuilder{
		Config: execute.Config{
			Binary:  "echo",
			Profile: "agents/generator/profile.yaml",
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
			Profile: "agents/generator/profile.yaml",
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

func TestSelfInvokeUndoMementoCapturesChildRunMetadata(t *testing.T) {
	t.Parallel()
	cmd := (&SelfInvokeBuilder{
		Config: execute.Config{
			Profile: "agents/generator/profile.yaml",
			Machine: "machine.yaml",
			Tools:   "tools.yaml",
			OTelDir: t.TempDir(),
		},
		Ctx: context.Background(),
	}).Build(core.Result{Output: `{"parameters":{"run_id":"child-1"}}`})

	provider, ok := cmd.(core.UndoMementoProvider)
	require.True(t, ok)
	memento, err := provider.UndoMemento()
	require.NoError(t, err)
	require.NoError(t, core.ValidateUndoMemento(memento))

	var payload BoundaryCompensationPayload
	require.NoError(t, json.Unmarshal(memento.Payload, &payload))
	require.Equal(t, "child_agent_workspace_restore", payload.BoundaryCompensation.Strategy)
	require.Equal(t, "child-1", payload.BoundaryCompensation.ChildRunID)
	require.Equal(t, "agents/generator/profile.yaml", payload.BoundaryCompensation.ChildProfile)
	require.Equal(t, "machine.yaml", payload.BoundaryCompensation.ChildMachine)
}

func TestSelfInvokeToolSpec(t *testing.T) {
	t.Parallel()
	spec := SelfInvokeToolSpec()

	assert.Equal(t, "self_invoke", spec.Name)
	assert.Equal(t, core.Internal, spec.Visibility)
}
