// Copyright (c) 2026 Nokia. All rights reserved.

package control

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/support/execute"
)

func TestSelfInvokeUsesSharedExecuteConfigArgs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	builder := &SelfInvokeBuilder{
		Config: execute.Config{
			Binary: "echo", Profile: "agents/generator/profile.yaml",
			Directory: "/workspace", OTelDir: dir, Timeout: 5 * time.Second,
		},
		Ctx: context.Background(),
	}

	result := builder.Build(core.Result{Output: `{"parameters":{"run_id":"child-1"}}`}).Execute()

	require.Equal(t, core.ToolDone, result.Signal)
	require.Contains(t, result.Output, "--profile agents/generator/profile.yaml")
	require.Contains(t, result.Output, "--directory /workspace")
	require.Contains(t, result.Output, "--otel-log-file "+dir+"/child-child-1.otel.json")
}
