// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunAgentCmdUsesSharedExecuteConfigArgs(t *testing.T) {
	t.Parallel()

	pointDir := t.TempDir()
	tracePath := filepath.Join(pointDir, "trace.ndjson")
	resultPath := filepath.Join(pointDir, "result.json")
	pc := &PointContext{
		PointID: "point-1", PointDir: pointDir, TracePath: tracePath,
		ResultPath: resultPath, Harness: Harness{Binary: "echo"},
		ProfilePath: "agents/executor/profile.yaml", Timeout: 5 * time.Second,
	}

	result := (&runAgentCmd{pc: pc, ctx: context.Background()}).Execute()

	require.Equal(t, SigHarnessFinished, result.Signal)
	require.Contains(t, result.Output, "--profile agents/executor/profile.yaml")
	require.Contains(t, result.Output, "--directory "+pointDir)
	require.Contains(t, result.Output, "--otel-log-file "+tracePath)
	data, err := os.ReadFile(resultPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "--profile agents/executor/profile.yaml")
}
