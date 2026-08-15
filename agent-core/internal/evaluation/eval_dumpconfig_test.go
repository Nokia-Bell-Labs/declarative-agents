// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package evaluation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestRecordAgentCommitMapsConfiguredRevParseResult(t *testing.T) {
	pc := &PointContext{}

	success := (&recordAgentCommitCmd{
		pc:    pc,
		prior: core.Result{Signal: core.ToolDone, Output: "abc123\n"},
	}).Execute()
	requireSignal(t, success, SigAgentCommitRecorded)
	require.Equal(t, "abc123", pc.AgentCommit)

	failure := (&recordAgentCommitCmd{
		pc:    pc,
		prior: core.Result{Signal: core.ToolFailed, Output: "not a repository"},
	}).Execute()
	requireSignal(t, failure, SigAgentCommitRecorded)
	require.Equal(t, "unknown", pc.AgentCommit)
}

func TestDumpConfigUsesRecordedCommitProvenance(t *testing.T) {
	pc := &PointContext{
		PointDir:    t.TempDir(),
		AgentCommit: "abc123",
		Harness:     Harness{Name: "executor", Binary: "agent"},
		Model:       "model",
		Sample:      Sample{Name: "sample"},
	}

	result := (&dumpConfigCmd{pc: pc}).Execute()

	requireSignal(t, result, SigConfigDumped)
	data, err := os.ReadFile(filepath.Join(pc.PointDir, ArtifactExperiment))
	require.NoError(t, err)
	require.Contains(t, string(data), "agent_commit: abc123")
}
