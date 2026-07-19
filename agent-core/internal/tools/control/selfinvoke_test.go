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
			Directory: "/workspace", OTelDir: dir, Timeout: 5 * time.Second,
		},
		Ctx: context.Background(),
	}

	result := builder.Build(core.Result{Output: `{"parameters":{"run_id":"child-1"}}`}).Execute()

	require.Equal(t, core.ToolDone, result.Signal)
	require.Contains(t, result.Output, "--profile agents/executor/profile.yaml")
	require.Contains(t, result.Output, "--directory /workspace")
	require.Contains(t, result.Output, "--otel-log-file "+dir+"/child-child-1.otel.json")
}
