// Copyright (c) 2026 Nokia. All rights reserved.

package bench

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/evaluation"
)

func TestSessionAPIsReturnEvaluationArtifacts(t *testing.T) {
	root := t.TempDir()
	pointID := evaluation.EvalPointID("sample1", "harness1", "model1", nil, 1)
	pointDir := filepath.Join(root, "suite1", "20260614T100000Z", pointID)
	require.NoError(t, os.MkdirAll(pointDir, 0o755))
	writeBenchTestArtifact(t, filepath.Join(pointDir, evaluation.ArtifactMeta), map[string]any{
		"harness":      "harness1",
		"model":        "model1",
		"sample":       "sample1",
		"repetition":   1,
		"exit_code":    0,
		"duration_ns":  time.Second,
		"tests_passed": true,
		"timed_out":    false,
	})
	trace := `{"Name":"execute_tool test","StartTime":"2026-01-01T00:00:00Z","EndTime":"2026-01-01T00:00:01Z","Attributes":[{"Key":"command.name","Value":{"Type":"STRING","Value":"test"}},{"Key":"command.signal","Value":{"Type":"STRING","Value":"ToolDone"}},{"Key":"tool.metrics.total","Value":{"Type":"INT64","Value":1}},{"Key":"tool.metrics.passed","Value":{"Type":"INT64","Value":1}},{"Key":"tool.metrics.failed","Value":{"Type":"INT64","Value":0}}],"Events":[]}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(pointDir, evaluation.ArtifactTrace), []byte(trace), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pointDir, evaluation.ArtifactExperiment), []byte("model: model1\n"), 0o644))

	handler := NewServer(ServerConfig{DataDir: root}, nil).Handler()

	assertAPIData(t, handler, "/api/v1/sessions", http.StatusOK, "suite1/20260614T100000Z")
	assertAPIData(t, handler, "/api/v1/sessions/suite1/20260614T100000Z", http.StatusOK, "modelStats")
	assertAPIData(t, handler, "/api/v1/sessions/suite1/20260614T100000Z/points", http.StatusOK, pointID)
	assertAPIData(t, handler, "/api/v1/sessions/suite1/20260614T100000Z/points/"+pointID, http.StatusOK, "snapshots")
	assertAPIData(t, handler, "/api/v1/sessions/suite1/20260614T100000Z/points/"+pointID+"/experiment", http.StatusOK, "model1")
}

func writeBenchTestArtifact(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func assertAPIData(t *testing.T, handler http.Handler, path string, status int, contains string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, status, rec.Code)
	require.Contains(t, rec.Body.String(), contains)
}

func TestActionEndpointAppliesSinglePendingActionBackpressure(t *testing.T) {
	actionCh := make(chan UserAction, 1)
	handler := NewServer(ServerConfig{}, actionCh).Handler()

	rec := postAction(t, handler, `{"type":"launch_eval"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "accepted")

	rec = postAction(t, handler, `{"type":"shutdown"}`)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "no active serve_ui listener")
}

func postAction(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/actions", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
