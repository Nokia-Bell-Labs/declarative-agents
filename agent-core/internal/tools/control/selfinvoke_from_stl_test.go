// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package control

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/execute"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

func TestSelfInvokeBuilder_Build(t *testing.T) {
	t.Parallel()
	builder := &SelfInvokeBuilder{
		ToolName: "invoke_executor",
		Config: execute.Config{
			Binary:  "echo",
			Profile: "agents/executor/profile.yaml",
		},
		Ctx: context.Background(),
	}

	res := core.Result{
		Output: `{"parameters":{"run_id":"build-test-42"}}`,
	}

	cmd := builder.Build(res)
	assert.Equal(t, "invoke_executor", cmd.Name())
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
	assert.NotEmpty(t, result.Receipt, "a child that started and exited unsuccessfully remains compensatable")
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
	assert.Empty(t, result.Receipt, "a child that never started must not claim a boundary receipt")
}

func TestSelfInvokeBuilder_ExtraArgs(t *testing.T) {
	t.Parallel()
	builder := &SelfInvokeBuilder{
		Config: execute.Config{
			Binary:  "echo",
			Profile: "agents/executor/profile.yaml",
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

func TestSelfInvokeAliasNamesPreStartErrors(t *testing.T) {
	t.Parallel()
	builder := &SelfInvokeBuilder{
		ToolName:    "launch_evaluator",
		Config:      execute.Config{Binary: "true", Profile: "agents/critic/profile.yaml"},
		RequestFrom: "$from(missing).suite",
		Ctx:         context.Background(),
	}

	result := builder.Build(core.Result{}).Execute()

	assert.Equal(t, core.CommandError, result.Signal)
	assert.Equal(t, "launch_evaluator", result.CommandName)
	assert.ErrorContains(t, result.Err, "launch_evaluator:")
	assert.Empty(t, result.Receipt)
}

func TestSelfInvokeUndoMatchesCompensatableReceipt(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	traceDir := t.TempDir()
	builder := &SelfInvokeBuilder{
		ToolName: "invoke_executor",
		Config: execute.Config{
			Binary: "true", Profile: "child/profile.yaml", OTelDir: traceDir,
		},
		WorkspacePath: workspace,
		Ctx:           context.Background(),
	}
	cmd := builder.Build(core.Result{Output: `{"parameters":{"run_id":"child-42"}}`})
	result := cmd.Execute()
	assert.Equal(t, core.ToolDone, result.Signal)
	assert.Equal(t, "invoke_executor", result.CommandName)
	assert.NotEmpty(t, result.Receipt)
	compensation, ok, err := undo.DecodeBoundaryReceipt(result.Receipt)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "child_agent_workspace_restore", compensation.Strategy)
	assert.Equal(t, []string{"child_workspace_ref", "child_trace"}, compensation.Requires)
	assert.Equal(t, "invoke_executor", compensation.Data["command_name"])
	assert.Equal(t, "child/profile.yaml", compensation.Data["child_profile"])
	assert.Equal(t, "child-42", compensation.Data["child_run_id"])
	assert.Equal(t, workspace, compensation.Data["child_workspace_path"])
	assert.Equal(t, []interface{}{traceDir + "/child-child-42.otel.json"}, compensation.Data["artifact_paths"])

	fresh := builder.BuildReverser()
	assert.Equal(t, "invoke_executor", fresh.Name())
	undone := fresh.Undo(result)
	assert.Equal(t, core.CompensationRequired, undone.Signal)
	assert.Equal(t, "invoke_executor", undone.CommandName)
	assert.NoError(t, undone.Err)
	pending, ok, err := undo.DecodeBoundaryReceipt(undone.Output)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, compensation, pending)
}

func TestSelfInvokeReverserRejectsInvalidBoundaryReceipts(t *testing.T) {
	t.Parallel()
	builder := &SelfInvokeBuilder{
		ToolName:      "invoke_executor",
		Config:        execute.Config{Profile: "child/profile.yaml"},
		WorkspacePath: "/workspace",
	}
	valid := undo.BoundaryCompensationPayload{BoundaryCompensation: undo.BoundaryCompensation{
		Strategy: selfInvokeUndoStrategy,
		Requires: []string{childWorkspaceRequirement, childTraceRequirement},
		Data: map[string]interface{}{
			"command_name":         "invoke_executor",
			"child_profile":        "child/profile.yaml",
			"child_run_id":         "child-42",
			"child_workspace_path": "/workspace",
			"artifact_paths":       []string{"/traces/child-42.json"},
		},
	}}
	tests := []struct {
		name    string
		receipt string
		want    string
	}{
		{name: "missing", want: "missing compensation data"},
		{name: "malformed", receipt: "{", want: "decode child boundary receipt"},
		{
			name: "wrong strategy",
			receipt: undo.EncodeBoundaryReceipt(undo.BoundaryCompensationPayload{
				BoundaryCompensation: undo.BoundaryCompensation{
					Strategy: "nested_machine_rollback",
				},
			}),
			want: "does not match",
		},
		{
			name: "command mismatch",
			receipt: selfInvokeReceiptWith(t, valid, func(compensation *undo.BoundaryCompensation) {
				compensation.Data["command_name"] = "launch_evaluator"
			}),
			want: "does not match reverser",
		},
		{
			name: "profile mismatch",
			receipt: selfInvokeReceiptWith(t, valid, func(compensation *undo.BoundaryCompensation) {
				compensation.Data["child_profile"] = "other/profile.yaml"
			}),
			want: "does not match configured profile",
		},
		{
			name: "workspace mismatch",
			receipt: selfInvokeReceiptWith(t, valid, func(compensation *undo.BoundaryCompensation) {
				compensation.Data["child_workspace_path"] = "/other"
			}),
			want: "does not match configured workspace path",
		},
		{
			name: "missing requirement",
			receipt: selfInvokeReceiptWith(t, valid, func(compensation *undo.BoundaryCompensation) {
				compensation.Requires = []string{childWorkspaceRequirement}
			}),
			want: "missing compensation requirement",
		},
		{
			name: "malformed artifact paths",
			receipt: selfInvokeReceiptWith(t, valid, func(compensation *undo.BoundaryCompensation) {
				compensation.Data["artifact_paths"] = "trace.json"
			}),
			want: "artifact_paths has type",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildReverser().Undo(core.Result{
				CommandName: "invoke_executor",
				Receipt:     tt.receipt,
			})
			assert.Equal(t, core.CommandError, result.Signal)
			assert.Equal(t, "invoke_executor", result.CommandName)
			assert.ErrorContains(t, result.Err, tt.want)
		})
	}
}

func selfInvokeReceiptWith(
	t *testing.T,
	payload undo.BoundaryCompensationPayload,
	mutate func(*undo.BoundaryCompensation),
) string {
	t.Helper()
	compensation := payload.BoundaryCompensation
	compensation.Requires = append([]string(nil), compensation.Requires...)
	compensation.Data = make(map[string]interface{}, len(payload.BoundaryCompensation.Data))
	for key, value := range payload.BoundaryCompensation.Data {
		compensation.Data[key] = value
	}
	mutate(&compensation)
	receipt := undo.EncodeBoundaryReceipt(undo.BoundaryCompensationPayload{
		BoundaryCompensation: compensation,
	})
	assert.NotEmpty(t, receipt)
	return receipt
}
