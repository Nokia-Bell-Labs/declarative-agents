// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestEvaluatorReceiptRestoresFreshCommandAfterCheckpointRoundTrip(t *testing.T) {
	t.Parallel()

	stderr := &bytes.Buffer{}
	pc := &PointContext{AgentCommit: "prior-commit", Stderr: stderr}
	state := &EvalState{PC: pc}
	builder := &RecordAgentCommitBuilder{ES: state}

	result := builder.Build(core.Result{Signal: core.ToolDone, Output: "replacement-commit\n"}).Execute()
	require.Equal(t, SigAgentCommitRecorded, result.Signal)
	require.Equal(t, "replacement-commit", pc.AgentCommit)
	require.NotEmpty(t, result.Receipt)

	checkpoint := &core.InMemoryCheckpoint{}
	require.NoError(t, checkpoint.Save(core.Position{}, core.Execution{{
		CommandName: "record_agent_commit",
		Receipt:     result.Receipt,
	}}))
	_, execution, err := checkpoint.Load()
	require.NoError(t, err)
	require.Len(t, execution, 1)

	pc.AgentCommit = "later-commit"
	undoResult := builder.BuildReverser().Undo(core.Result{Receipt: execution[0].Receipt})

	require.Equal(t, core.ToolDone, undoResult.Signal, undoResult.Output)
	require.Equal(t, "prior-commit", pc.AgentCommit)
	require.Same(t, stderr, pc.Stderr)
}

func TestEvaluatorReceiptRemovesOwnedArtifactWithFreshCommand(t *testing.T) {
	t.Parallel()

	pointDir := t.TempDir()
	pc := &PointContext{PointDir: pointDir, Sample: Sample{Name: "sample"}}
	builder := &DumpConfigBuilder{ES: &EvalState{PC: pc}}

	result := builder.Build(core.Result{}).Execute()
	require.Equal(t, SigConfigDumped, result.Signal, result.Output)
	artifact := filepath.Join(pointDir, ArtifactExperiment)
	_, err := os.Stat(artifact)
	require.NoError(t, err)

	undoResult := builder.BuildReverser().Undo(core.Result{Receipt: result.Receipt})

	require.Equal(t, core.ToolDone, undoResult.Signal, undoResult.Output)
	_, err = os.Stat(artifact)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestRunAgentReceiptRestoresPointAndSurfacesChildCompensation(t *testing.T) {
	t.Parallel()

	pointDir := t.TempDir()
	script := filepath.Join(pointDir, "agent")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nprintf 'agent output'\n"), 0o755))
	resultPath := filepath.Join(pointDir, "result.json")
	pc := &PointContext{
		PointDir: pointDir, TracePath: filepath.Join(pointDir, "trace.ndjson"),
		ResultPath: resultPath, ProfilePath: "agents/executor/profile.yaml",
		Harness: Harness{Binary: script}, Timeout: 5 * time.Second,
	}
	builder := &RunAgentBuilder{ES: &EvalState{PC: pc, Ctx: context.Background()}}

	result := builder.Build(core.Result{}).Execute()
	require.Equal(t, SigHarnessFinished, result.Signal, result.Output)
	require.NotEmpty(t, result.Receipt)
	require.FileExists(t, resultPath)
	var receipt evaluatorReceipt
	require.NoError(t, json.Unmarshal([]byte(result.Receipt), &receipt))
	metadata, ok := receipt.BoundaryMetadata.(map[string]any)
	require.True(t, ok)
	require.Equal(t, script, metadata["binary"])
	require.Equal(t, resultPath, metadata["result_path"])

	pc.ExitCode = 99
	undoResult := builder.BuildReverser().Undo(core.Result{Receipt: result.Receipt})

	require.Equal(t, core.CommandError, undoResult.Signal)
	require.Contains(t, undoResult.Output, "boundary compensation required")
	require.Zero(t, pc.ExitCode)
	_, err := os.Stat(resultPath)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestRunPointReceiptRestoresSessionAndSurfacesNestedCompensation(t *testing.T) {
	t.Parallel()

	session := &EvalSessionState{SessionDir: "prior-session"}
	builder := &RunPointBuilder{ES: session}
	result := builder.Build(core.Result{}).Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.NotEmpty(t, result.Receipt)
	var receipt evaluatorReceipt
	require.NoError(t, json.Unmarshal([]byte(result.Receipt), &receipt))
	require.NotNil(t, receipt.BoundaryMetadata)

	session.SessionDir = "replacement-session"
	undoResult := builder.BuildReverser().Undo(core.Result{Receipt: result.Receipt})

	require.Equal(t, core.CommandError, undoResult.Signal)
	require.Contains(t, undoResult.Output, "nested point machine history")
	require.Equal(t, "prior-session", session.SessionDir)
}

func TestEvaluatorMutationBuildersImplementReverser(t *testing.T) {
	t.Parallel()

	builders := []core.Builder{
		&ParseSuiteConfigBuilder{},
		&DiscoverSuiteSamplesBuilder{},
		&ExpandEvalGridBuilder{},
		&InitEvalSessionBuilder{},
		&NextPointBuilder{},
		&ReportSessionBuilder{},
		&RunPointBuilder{},
		&CreatePointDirBuilder{},
		&DumpConfigBuilder{},
		&RunAgentBuilder{},
		&RecordAgentCommitBuilder{},
		&RecordOracleResultBuilder{},
		&CollectTraceTokensBuilder{},
		&CheckAgentVersionBuilder{},
		&RecordPointFailureBuilder{},
		&CollectMetricsBuilder{},
	}
	for _, builder := range builders {
		_, ok := builder.(core.Reverser)
		require.Truef(t, ok, "%T must implement core.Reverser", builder)
	}
}
