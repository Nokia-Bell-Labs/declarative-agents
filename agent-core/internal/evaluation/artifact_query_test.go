// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package evaluation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
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

// seedEvaluationArtifacts writes one complete evaluation point (meta + trace)
// under a fresh results root and returns the root plus its coordinates.
func seedEvaluationArtifacts(t *testing.T) (root, suite, timestamp, pointID string) {
	t.Helper()
	root = t.TempDir()
	suite, timestamp = "suite1", "20260614T100000Z"
	pointID = EvalPointID("sample1", "harness1", "model1", nil, 1)
	pointDir := filepath.Join(root, suite, timestamp, pointID)
	require.NoError(t, os.MkdirAll(pointDir, 0o755))
	writeArtifactQueryJSON(t, filepath.Join(pointDir, ArtifactMeta), EvalMeta{
		Harness: "harness1", Model: "model1", Sample: "sample1", Repetition: 1,
		ExitCode: 0, Duration: time.Second, TestsPassed: true,
	})
	trace := `{"Name":"execute_tool test","StartTime":"2026-01-01T00:00:00Z","EndTime":"2026-01-01T00:00:01Z","Attributes":[{"Key":"command.name","Value":{"Type":"STRING","Value":"test"}},{"Key":"command.signal","Value":{"Type":"STRING","Value":"ToolDone"}}],"Events":[]}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(pointDir, ArtifactTrace), []byte(trace), 0o644))
	return root, suite, timestamp, pointID
}

func artifactParams(t *testing.T, kv map[string]string) string {
	t.Helper()
	data, err := json.Marshal(kv)
	require.NoError(t, err)
	return string(data)
}

// TestEvaluationArtifactCommandBoundary exercises the registered command
// boundary -- Build(res).Execute() -- for every operation, so the formal suite
// evidence observes parameter decoding, the output envelope, and signal mapping
// rather than only the lower-layer query helpers (GH-1358).
func TestEvaluationArtifactCommandBoundary(t *testing.T) {
	root, suite, timestamp, pointID := seedEvaluationArtifacts(t)

	cases := []struct {
		name       string
		operation  string
		params     string
		wantSignal core.Signal
		wantSubs   []string
	}{
		{
			name:       "list sessions",
			operation:  "list_evaluation_sessions",
			wantSignal: SignalEvaluationDataReady,
			wantSubs:   []string{`"data"`, suite, timestamp},
		},
		{
			name:       "analyze session",
			operation:  "analyze_evaluation_session",
			params:     artifactParams(t, map[string]string{"suite": suite, "timestamp": timestamp}),
			wantSignal: SignalEvaluationDataReady,
			wantSubs:   []string{`"modelStats"`, "model1"},
		},
		{
			name:       "list points",
			operation:  "list_evaluation_points",
			params:     artifactParams(t, map[string]string{"suite": suite, "timestamp": timestamp}),
			wantSignal: SignalEvaluationDataReady,
			wantSubs:   []string{pointID, `"testsPassed":true`},
		},
		{
			name:       "read trace",
			operation:  "read_evaluation_trace",
			params:     artifactParams(t, map[string]string{"suite": suite, "timestamp": timestamp, "point_id": pointID}),
			wantSignal: SignalEvaluationDataReady,
			wantSubs:   []string{`"spans"`, pointID},
		},
		{
			name:       "unknown operation",
			operation:  "delete_everything",
			wantSignal: core.CommandError,
			wantSubs:   []string{"unknown evaluation artifact operation"},
		},
		{
			name:       "denied traversal",
			operation:  "analyze_evaluation_session",
			params:     artifactParams(t, map[string]string{"suite": "..", "timestamp": timestamp}),
			wantSignal: SignalEvaluationDenied,
			wantSubs:   []string{"denied evaluation"},
		},
		{
			name:       "malformed parameters denied",
			operation:  "analyze_evaluation_session",
			params:     "not-json",
			wantSignal: SignalEvaluationDenied,
			wantSubs:   []string{"denied evaluation"},
		},
		{
			name:       "missing point",
			operation:  "read_evaluation_trace",
			params:     artifactParams(t, map[string]string{"suite": suite, "timestamp": timestamp, "point_id": "absent"}),
			wantSignal: SignalEvaluationMissing,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			builder := &EvaluationArtifactBuilder{Name: "evaluation_artifact", Operation: tc.operation, DataDir: root}
			cmd := builder.Build(core.Result{Output: tc.params})
			require.Equal(t, "evaluation_artifact", cmd.Name())

			result := cmd.Execute()
			require.Equal(t, tc.wantSignal, result.Signal, result.Output)
			for _, sub := range tc.wantSubs {
				require.Contains(t, result.Output, sub)
			}
			if tc.wantSignal == SignalEvaluationDataReady {
				var env map[string]json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(result.Output), &env))
				require.Contains(t, env, "data", "success envelope must carry a data field")
			}
		})
	}
}

// TestEvaluationArtifactBuilderDecodesNestedParameters proves the command reads
// parameters wrapped under a "parameters" key, the shape declared tools emit.
func TestEvaluationArtifactBuilderDecodesNestedParameters(t *testing.T) {
	t.Parallel()
	root, suite, timestamp, pointID := seedEvaluationArtifacts(t)
	payload := `{"parameters":{"suite":"` + suite + `","timestamp":"` + timestamp + `","point_id":"` + pointID + `"}}`

	builder := &EvaluationArtifactBuilder{Name: "evaluation_artifact", Operation: "read_evaluation_trace", DataDir: root}
	result := builder.Build(core.Result{Output: payload}).Execute()

	require.Equal(t, SignalEvaluationDataReady, result.Signal, result.Output)
	require.Contains(t, result.Output, pointID)
}

func TestEvaluationArtifactCommandUndoIsNoop(t *testing.T) {
	t.Parallel()
	builder := &EvaluationArtifactBuilder{Name: "evaluation_artifact", Operation: "list_evaluation_sessions", DataDir: t.TempDir()}
	undo := builder.Build(core.Result{}).Undo(core.Result{})
	require.Equal(t, core.ToolDone, undo.Signal)
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
