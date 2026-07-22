// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDirReportsMalformedAndPartialSessions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setup       func(*testing.T, string)
		wantResults int
		wantErr     string
	}{
		{name: "complete", setup: writeCompletePoint, wantResults: 1},
		{name: "missing metadata", setup: func(t *testing.T, point string) {
			require.NoError(t, os.WriteFile(filepath.Join(point, ArtifactTrace), sampleNDJSON, 0o600))
		}, wantErr: ArtifactMeta},
		{name: "malformed metadata", setup: func(t *testing.T, point string) {
			require.NoError(t, os.WriteFile(filepath.Join(point, ArtifactMeta), []byte(`{"sample":`), 0o600))
		}, wantErr: "parse " + ArtifactMeta},
		{name: "metadata path unreadable", setup: func(t *testing.T, point string) {
			require.NoError(t, os.Mkdir(filepath.Join(point, ArtifactMeta), 0o700))
		}, wantErr: "read " + ArtifactMeta},
		{name: "partial malformed trace", setup: func(t *testing.T, point string) {
			writePointMeta(t, point)
			require.NoError(t, os.WriteFile(filepath.Join(point, ArtifactTrace), []byte(`{broken`), 0o600))
		}, wantResults: 1, wantErr: "read " + ArtifactTrace},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			point := filepath.Join(root, "point-1")
			require.NoError(t, os.Mkdir(point, 0o700))
			tt.setup(t, point)
			results, err := loadDir(root)
			require.Len(t, results, tt.wantResults)
			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Equal(t, 100, results[0].TokensIn)
				assert.Equal(t, 50, results[0].TokensOut)
				assert.Equal(t, 1, results[0].Iterations)
				return
			}
			require.ErrorContains(t, err, "point-1")
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestLoadMultipleReportsDuplicatesAndReturnsPartialGroups(t *testing.T) {
	t.Parallel()
	first, second := t.TempDir(), t.TempDir()
	for _, root := range []string{first, second} {
		point := filepath.Join(root, "point")
		require.NoError(t, os.Mkdir(point, 0o700))
		writeCompletePoint(t, point)
	}

	groups, err := LoadMultiple([]string{first, second})
	require.ErrorContains(t, err, "duplicate evaluation point sample-a/model-a/1")
	require.Len(t, groups[GroupKey{Sample: "sample-a", Model: "model-a"}], 1)
}

func TestCountIterationsUsesLoopSpansBeforeToolFallback(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 2, countIterations([]*Span{{Name: "loop/iteration"}, {Name: "loop_iteration"}, {Name: "execute_tool"}}))
	spans, err := ParseNDJSON(sampleNDJSON)
	require.NoError(t, err)
	assert.Equal(t, 1, countIterations(spans))
	assert.Zero(t, countIterations(nil))
}

func writeCompletePoint(t *testing.T, point string) {
	t.Helper()
	writePointMeta(t, point)
	require.NoError(t, os.WriteFile(filepath.Join(point, ArtifactTrace), sampleNDJSON, 0o600))
}

func writePointMeta(t *testing.T, point string) {
	t.Helper()
	data, err := json.Marshal(EvalMeta{
		Sample: "sample-a", Model: "model-a", Repetition: 1, TestsPassed: true,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(point, ArtifactMeta), data, 0o600))
}
