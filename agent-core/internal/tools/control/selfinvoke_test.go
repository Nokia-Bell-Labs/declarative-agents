// Copyright (c) 2026 Nokia. All rights reserved.

package control

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/execute"
)

func TestSelfInvokeUsesSharedExecuteConfigArgs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	builder := &SelfInvokeBuilder{
		Config: execute.Config{
			Binary: "echo", Profile: "agents/executor/profile.yaml",
			CoreRoot: "/checkout/agent-core", Directory: "/workspace",
			OTelDir: dir, Timeout: 5 * time.Second,
		},
		Ctx: context.Background(),
	}

	result := builder.Build(core.Result{Output: `{"parameters":{"run_id":"child-1"}}`}).Execute()

	require.Equal(t, core.ToolDone, result.Signal)
	require.Contains(t, result.Output, "--profile agents/executor/profile.yaml")
	require.Contains(t, result.Output, "--core-root /checkout/agent-core")
	require.Contains(t, result.Output, "--directory /workspace")
	require.Contains(t, result.Output, "--otel-log-file "+dir+"/child-child-1.otel.json")
}

func TestSelfInvokeResolvesRequestAndOutputFromCommandState(t *testing.T) {
	builder := &SelfInvokeBuilder{
		Config:      execute.Config{Binary: "echo", Profile: "agents/critic/profile.yaml"},
		RequestFrom: "$from(action).suite",
		OutputFrom:  "$from(action).output_dir",
		Ctx:         context.Background(),
	}
	cmd := builder.Build(core.Result{})
	aware := cmd.(core.CommandStateAware)
	aware.SetCommandState(core.NewCommandStateView(core.Execution{{
		CommandName: "await_action",
		Label:       "action",
		Result: core.ResultDigest{
			Output:           `{"suite":"suites/basic.yaml","output_dir":"eval-results"}`,
			RedactionVersion: core.OutputRedactionVersion1,
			RedactionStatus:  core.OutputRedactionApplied,
		},
	}}))

	result := cmd.Execute()

	require.Equal(t, core.ToolDone, result.Signal)
	require.Contains(t, result.Output, "--request suites/basic.yaml")
	require.Contains(t, result.Output, "--output eval-results")
}
