// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestRunOracleCheckPassesAndFailsWithDomainSignals(t *testing.T) {
	passPC := pointResultFixture(t, "func TestPass(t *testing.T) {}\n")
	res := (&runOracleCheckCmd{pc: passPC}).Execute()
	requireSignal(t, res, SigOracleCheckPassed)
	require.True(t, passPC.TestsPassed)
	require.Contains(t, passPC.TestOutput, "ok")

	failPC := pointResultFixture(t, "func TestFail(t *testing.T) { t.Fatal(\"boom\") }\n")
	res = (&runOracleCheckCmd{pc: failPC}).Execute()
	require.Equal(t, SigOracleCheckFailed, res.Signal)
	require.NoError(t, res.Err)
	require.False(t, failPC.TestsPassed)
	require.Contains(t, failPC.TestOutput, "boom")
}

func TestCollectTraceTokensHandlesMissingTrace(t *testing.T) {
	pc := pointResultFixture(t, "func TestPass(t *testing.T) {}\n")
	pc.TracePath = filepath.Join(pc.PointDir, "missing-trace.ndjson")

	res := (&collectTraceTokensCmd{pc: pc}).Execute()

	requireSignal(t, res, SigTraceTokensCollected)
	require.Zero(t, pc.Tokens)
	require.Contains(t, res.Output, "trace file not found")
}

func TestCollectTraceTokensParsesTraceUsage(t *testing.T) {
	pc := pointResultFixture(t, "func TestPass(t *testing.T) {}\n")
	require.NoError(t, os.WriteFile(pc.TracePath, sampleNDJSON, 0o644))

	res := (&collectTraceTokensCmd{pc: pc}).Execute()

	requireSignal(t, res, SigTraceTokensCollected)
	require.Equal(t, 150, pc.Tokens)
	require.Equal(t, 150, res.Cost.TokensIn)
}

func TestCollectTraceTokensUndoRestoresPointContext(t *testing.T) {
	pc := pointResultFixture(t, "func TestPass(t *testing.T) {}\n")
	pc.PointID = "point-1"
	pc.Tokens = 7
	require.NoError(t, os.WriteFile(pc.TracePath, sampleNDJSON, 0o644))

	cmd := &collectTraceTokensCmd{pc: pc}
	requireSignal(t, cmd.Execute(), SigTraceTokensCollected)
	require.Equal(t, 150, pc.Tokens)

	undo := cmd.Undo(core.Result{})
	requireSignal(t, undo, core.ToolDone)
	require.Equal(t, 7, pc.Tokens)

	memento, err := cmd.UndoMemento()
	require.NoError(t, err)
	require.Equal(t, core.UndoMementoReversible, memento.Kind)
	require.NoError(t, core.ValidateUndoMemento(memento))
	require.Contains(t, string(memento.Payload), `"point_id"`)
}

func TestCheckAgentVersionReportsMismatchWarning(t *testing.T) {
	var stderr bytes.Buffer
	pc := pointResultFixture(t, "func TestPass(t *testing.T) {}\n")
	pc.Harness.Version = "v9.9.9"
	pc.Stderr = &stderr
	require.NoError(t, os.WriteFile(pc.TracePath, sampleNDJSON, 0o644))

	res := (&checkAgentVersionCmd{pc: pc}).Execute()

	requireSignal(t, res, SigAgentVersionMismatch)
	require.True(t, pc.VersionMismatch)
	require.Equal(t, "v0.1.0", pc.TraceVersion)
	require.Contains(t, res.Output, "version mismatch")
	require.Contains(t, stderr.String(), "WARN: version mismatch")
}

func TestSummarizePointResultsUsesCollectedState(t *testing.T) {
	pc := pointResultFixture(t, "func TestPass(t *testing.T) {}\n")
	pc.TestsPassed = true
	pc.TestOutput = "ok example"
	pc.Tokens = 42

	res := (&summarizePointResultsCmd{pc: pc}).Execute()

	requireSignal(t, res, SigResultsCollected)
	require.Contains(t, res.Output, "tests_passed=true")
	require.Contains(t, res.Output, "tokens=42")
	require.Equal(t, 42, res.Cost.TokensIn)
}

func pointResultFixture(t *testing.T, testBody string) *PointContext {
	t.Helper()
	pointDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(pointDir, "go.mod"), []byte("module example.com/point\n\ngo 1.22\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pointDir, "main.go"), []byte("package main\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pointDir, "main_test.go"), []byte("package main\n\nimport \"testing\"\n\n"+testBody), 0o644))

	return &PointContext{
		PointDir:  pointDir,
		TracePath: filepath.Join(pointDir, ArtifactTrace),
		Harness:   Harness{Name: "harness", Version: "v0.1.0"},
	}
}
