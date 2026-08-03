// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEvaluationArtifactQueriesPreserveBenchData(t *testing.T) {
	root := t.TempDir()
	pointID := EvalPointID("sample1", "harness1", "model1", nil, 1)
	pointDir := filepath.Join(root, "suite1", "20260614T100000Z", pointID)
	require.NoError(t, os.MkdirAll(pointDir, 0o755))
	writeArtifactQueryJSON(t, filepath.Join(pointDir, ArtifactMeta), EvalMeta{
		Harness: "harness1", Model: "model1", Sample: "sample1", Repetition: 1,
		ExitCode: 0, Duration: time.Second, TestsPassed: true,
	})
	trace := `{"Name":"execute_tool test","StartTime":"2026-01-01T00:00:00Z","EndTime":"2026-01-01T00:00:01Z","Attributes":[{"Key":"command.name","Value":{"Type":"STRING","Value":"test"}},{"Key":"command.signal","Value":{"Type":"STRING","Value":"ToolDone"}},{"Key":"tool.metrics.total","Value":{"Type":"INT64","Value":1}},{"Key":"tool.metrics.passed","Value":{"Type":"INT64","Value":1}},{"Key":"tool.metrics.failed","Value":{"Type":"INT64","Value":0}}],"Events":[]}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(pointDir, ArtifactTrace), []byte(trace), 0o644))

	sessions, err := ListEvaluationSessions(root)
	require.NoError(t, err)
	require.Equal(t, []EvaluationSessionSummary{{
		ID: "suite1/20260614T100000Z", Name: "suite1", Timestamp: "20260614T100000Z",
		PointCount: 1, PassCount: 1,
	}}, sessions)

	detail, err := AnalyzeEvaluationSession(root, "suite1", "20260614T100000Z")
	require.NoError(t, err)
	require.Equal(t, 1, detail.TotalPoints)
	require.Equal(t, 1, detail.TotalPassed)
	require.Equal(t, "model1", detail.ModelStats[0].Model)

	points, err := ListEvaluationPoints(root, "suite1", "20260614T100000Z")
	require.NoError(t, err)
	require.Len(t, points, 1)
	require.Equal(t, pointID, points[0].PointID)
	require.True(t, points[0].TestsPassed)

	traceData, err := ReadEvaluationTrace(root, "suite1", "20260614T100000Z", pointID)
	require.NoError(t, err)
	require.Equal(t, pointID, traceData.PointID)
	require.Len(t, traceData.Spans, 1)
	require.Len(t, traceData.Snapshots, 1)
}

func TestEvaluationArtifactQueriesRejectTraversal(t *testing.T) {
	_, err := AnalyzeEvaluationSession(t.TempDir(), "..", "timestamp")
	require.ErrorContains(t, err, "denied evaluation session path")
	_, err = ReadEvaluationTrace(t.TempDir(), "suite", "timestamp", "../point")
	require.Error(t, err)
}

// TestEvaluationArtifactQueriesRejectSymlinkEscape is the GH-1358 confinement
// guard: an in-tree symlink at the suite, timestamp, point, or trace-file level
// that resolves outside the results root must be denied with no bytes returned.
// safeEvaluationComponent is only a lexical check; os.Stat/os.ReadFile follow
// symlinks, so the read path must verify the resolved path stays under the root.
func TestEvaluationArtifactQueriesRejectSymlinkEscape(t *testing.T) {
	const ts = "20260101T000000Z"
	const point = "pt1"

	writeTrace := func(t *testing.T, dir string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ArtifactTrace), []byte("{}\n"), 0o644))
	}

	t.Run("suite symlink escapes root", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		writeTrace(t, filepath.Join(outside, ts, point))
		if err := os.Symlink(outside, filepath.Join(root, "suitelink")); err != nil {
			t.Skipf("symlinks unsupported: %v", err)
		}

		trace, err := ReadEvaluationTrace(root, "suitelink", ts, point)
		require.ErrorContains(t, err, "denied evaluation")
		require.Empty(t, trace.Spans)
		require.Empty(t, trace.PointID)

		_, err = AnalyzeEvaluationSession(root, "suitelink", ts)
		require.ErrorContains(t, err, "denied evaluation")
	})

	t.Run("timestamp symlink escapes root", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "suite"), 0o755))
		writeTrace(t, filepath.Join(outside, point))
		if err := os.Symlink(outside, filepath.Join(root, "suite", "tslink")); err != nil {
			t.Skipf("symlinks unsupported: %v", err)
		}

		trace, err := ReadEvaluationTrace(root, "suite", "tslink", point)
		require.ErrorContains(t, err, "denied evaluation")
		require.Empty(t, trace.Spans)

		_, err = ListEvaluationPoints(root, "suite", "tslink")
		require.ErrorContains(t, err, "denied evaluation")
	})

	t.Run("point symlink escapes root", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		session := filepath.Join(root, "suite", ts)
		require.NoError(t, os.MkdirAll(session, 0o755))
		writeTrace(t, filepath.Join(outside, "secret"))
		if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(session, "ptlink")); err != nil {
			t.Skipf("symlinks unsupported: %v", err)
		}

		trace, err := ReadEvaluationTrace(root, "suite", ts, "ptlink")
		require.ErrorContains(t, err, "denied evaluation")
		require.Empty(t, trace.Spans)
	})

	t.Run("trace file symlink escapes point dir", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		pointDir := filepath.Join(root, "suite", ts, point)
		require.NoError(t, os.MkdirAll(pointDir, 0o755))
		secret := filepath.Join(outside, "secret.jsonl")
		require.NoError(t, os.WriteFile(secret, []byte("{}\n"), 0o644))
		if err := os.Symlink(secret, filepath.Join(pointDir, ArtifactTrace)); err != nil {
			t.Skipf("symlinks unsupported: %v", err)
		}

		trace, err := ReadEvaluationTrace(root, "suite", ts, point)
		require.ErrorContains(t, err, "denied evaluation")
		require.Empty(t, trace.Spans)
	})
}

func TestListEvaluationSessionsMissingRootIsEmpty(t *testing.T) {
	sessions, err := ListEvaluationSessions(filepath.Join(t.TempDir(), "missing"))
	require.NoError(t, err)
	require.Empty(t, sessions)
}

func writeArtifactQueryJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}
