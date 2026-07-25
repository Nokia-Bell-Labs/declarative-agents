// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
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

func TestRunAgentCmdReportsResultArtifactWriteFailure(t *testing.T) {
	t.Parallel()

	pointDir := t.TempDir()
	resultPath := filepath.Join(pointDir, "missing", "result.json")
	pc := &PointContext{
		PointDir: pointDir, TracePath: filepath.Join(pointDir, "trace.ndjson"),
		ResultPath: resultPath, Harness: Harness{Binary: "echo"},
		ProfilePath: "agents/executor/profile.yaml", Timeout: 5 * time.Second,
	}

	result := (&runAgentCmd{pc: pc, ctx: context.Background()}).Execute()

	require.Equal(t, core.CommandError, result.Signal)
	require.Error(t, result.Err)
	require.True(t, errors.Is(result.Err, os.ErrNotExist))
	require.Contains(t, result.Output, resultPath)
	require.Equal(t, 0, pc.ExitCode)
	require.False(t, pc.TimedOut)
	require.Positive(t, pc.Duration)
	require.Equal(t, pc.Duration, result.Cost.Duration)
}

func TestRunAgentCmdPersistsNonzeroExitOutput(t *testing.T) {
	t.Parallel()

	pointDir := t.TempDir()
	script := filepath.Join(pointDir, "failing-agent")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nprintf 'child output'\nexit 7\n"), 0o755))
	resultPath := filepath.Join(pointDir, "result.json")
	pc := &PointContext{
		PointDir: pointDir, TracePath: filepath.Join(pointDir, "trace.ndjson"),
		ResultPath: resultPath, Harness: Harness{Binary: script},
		ProfilePath: "agents/executor/profile.yaml", Timeout: 5 * time.Second,
	}

	result := (&runAgentCmd{pc: pc, ctx: context.Background()}).Execute()

	require.Equal(t, SigHarnessFailed, result.Signal)
	require.NoError(t, result.Err)
	require.Equal(t, 7, pc.ExitCode)
	data, err := os.ReadFile(resultPath)
	require.NoError(t, err)
	require.Equal(t, "child output", string(data))
}
