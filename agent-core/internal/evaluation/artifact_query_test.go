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
