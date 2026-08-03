// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/filesystem"
)

// resultWriteParams decodes the write parameters run_agent emits for the
// following write word.
func resultWriteParams(t *testing.T, output string) (path, content string) {
	t.Helper()
	var decoded struct {
		Parameters struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		} `json:"parameters"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &decoded))
	return decoded.Parameters.Path, decoded.Parameters.Content
}

func TestRunAgentCmdUsesSharedExecuteConfigArgs(t *testing.T) {
	t.Parallel()

	pointDir := t.TempDir()
	tracePath := filepath.Join(pointDir, "trace.ndjson")
	resultPath := filepath.Join(pointDir, "result.json")
	pc := &PointContext{
		PointID: "point-1", PointDir: pointDir, TracePath: tracePath,
		ResultPath: resultPath, Harness: Harness{Binary: "echo"},
		ProfilePath: "agents/executor/profile.yaml", CoreRoot: "/checkout/agent-core",
		Timeout: 5 * time.Second,
	}

	result := (&runAgentCmd{pc: pc, ctx: context.Background()}).Execute()

	require.Equal(t, SigHarnessFinished, result.Signal)
	// run_agent emits write parameters instead of writing the artifact itself
	// (GH-1378): path is the workspace-relative result.json and content is the
	// child agent's stdout.
	path, content := resultWriteParams(t, result.Output)
	require.Equal(t, ArtifactResult, path)
	require.Contains(t, content, "--profile agents/executor/profile.yaml")
	require.Contains(t, content, "--core-root /checkout/agent-core")
	require.Contains(t, content, "--directory "+pointDir)
	require.Contains(t, content, "--otel-log-file "+tracePath)
}

func TestRunAgentCmdEmitsNonzeroExitOutputParameters(t *testing.T) {
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
	require.Positive(t, pc.Duration)
	require.Equal(t, pc.Duration, result.Cost.Duration)
	path, content := resultWriteParams(t, result.Output)
	require.Equal(t, ArtifactResult, path)
	require.Equal(t, "child output", content)
}

func TestRunAgentCmdEmptyOutputWritesEmptyArtifact(t *testing.T) {
	t.Parallel()

	pointDir := t.TempDir()
	script := filepath.Join(pointDir, "silent-agent")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	pc := &PointContext{
		PointDir: pointDir, TracePath: filepath.Join(pointDir, "trace.ndjson"),
		ResultPath: filepath.Join(pointDir, ArtifactResult), Harness: Harness{Binary: script},
		ProfilePath: "agents/executor/profile.yaml", Timeout: 5 * time.Second,
	}

	runResult := (&runAgentCmd{pc: pc, ctx: context.Background()}).Execute()
	require.Equal(t, SigHarnessFinished, runResult.Signal, runResult.Output)
	writeResult := (&filesystem.WriteBuilder{Root: pointDir}).Build(runResult).Execute()

	require.Equal(t, core.ToolDone, writeResult.Signal, writeResult.Output)
	data, err := os.ReadFile(pc.ResultPath)
	require.NoError(t, err)
	require.Empty(t, data)
}
