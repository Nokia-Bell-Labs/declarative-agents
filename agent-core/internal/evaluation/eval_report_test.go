// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeModelStatsAggregatesRuns(t *testing.T) {
	groups := map[GroupKey][]EvalRunResult{
		{Sample: "sample-a", Model: "model-a"}: {
			{
				Sample:      "sample-a",
				Model:       "model-a",
				TestsPassed: true,
				Iterations:  2,
				TokensIn:    10,
				TokensOut:   5,
				Duration:    2 * time.Second,
				Progression: &RunProgression{Overall: Clean},
			},
			{
				Sample:      "sample-a",
				Model:       "model-a",
				TestsPassed: false,
				Iterations:  4,
				TokensIn:    20,
				TokensOut:   15,
				Duration:    4 * time.Second,
				Progression: &RunProgression{Overall: Flat},
			},
		},
	}

	stats := ComputeModelStats(groups)

	require.Len(t, stats, 1)
	assert.Equal(t, "model-a", stats[0].Model)
	assert.Equal(t, 2, stats[0].Runs)
	assert.Equal(t, 1, stats[0].Successes)
	assert.Equal(t, 0.5, stats[0].SuccessRate)
	assert.Equal(t, 0.5, stats[0].CleanRate)
	assert.Equal(t, 1.0, stats[0].StuckRate)
	assert.Equal(t, 3.0, stats[0].MeanIter)
	assert.Equal(t, 15.0, stats[0].MeanTokensIn)
	assert.Equal(t, 10.0, stats[0].MeanTokensOut)
	assert.Equal(t, 3*time.Second, stats[0].MeanDuration)
}

func TestComputeDetailedAggregatesSampleModelRows(t *testing.T) {
	groups := map[GroupKey][]EvalRunResult{
		{Sample: "sample-a", Model: "model-a"}: {
			{
				Sample:      "sample-a",
				Model:       "model-a",
				TestsPassed: true,
				Iterations:  1,
				TokensIn:    5,
				TokensOut:   7,
				Duration:    time.Second,
				Progression: &RunProgression{Overall: Converged},
			},
		},
	}

	rows := ComputeDetailed(groups)

	require.Len(t, rows, 1)
	assert.Equal(t, "sample-a", rows[0].Sample)
	assert.Equal(t, "model-a", rows[0].Model)
	assert.Equal(t, 1, rows[0].Runs)
	assert.Equal(t, 1.0, rows[0].SuccessRate)
	assert.Equal(t, 1.0, rows[0].MeanIter)
	assert.Equal(t, 12.0, rows[0].MeanTokens)
	assert.Equal(t, time.Second, rows[0].MeanDuration)
	assert.Equal(t, 1, rows[0].Convergences[Converged])
}

func TestWriteCSVSuccessfulRoundTrip(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	writer := &trackingWriteCloser{Writer: &output}
	groups := csvTestGroups("progress\ncontinued")

	require.NoError(t, writeCSV(writer, groups))
	require.True(t, writer.closed)
	records, err := csv.NewReader(bytes.NewReader(output.Bytes())).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, "sample", records[0][0])
	assert.Equal(t, "sample-a", records[1][0])
	assert.Equal(t, "progress continued", records[1][11])
}

func TestWriteCSVPropagatesWriteFlushAndCloseFailures(t *testing.T) {
	t.Parallel()
	writeFailure := errors.New("injected write failure")
	closeFailure := errors.New("injected close failure")
	tests := []struct {
		name       string
		writer     *trackingWriteCloser
		groups     map[GroupKey][]EvalRunResult
		wantErrors []error
	}{
		{
			name: "buffered flush failure",
			writer: &trackingWriteCloser{
				Writer: errorWriter{err: writeFailure},
			},
			groups:     csvTestGroups("small"),
			wantErrors: []error{writeFailure},
		},
		{
			name: "direct large-row write failure",
			writer: &trackingWriteCloser{
				Writer: errorWriter{err: writeFailure},
			},
			groups:     csvTestGroups(strings.Repeat("x", 8192)),
			wantErrors: []error{writeFailure},
		},
		{
			name: "close failure",
			writer: &trackingWriteCloser{
				Writer:   io.Discard,
				closeErr: closeFailure,
			},
			groups:     csvTestGroups("small"),
			wantErrors: []error{closeFailure},
		},
		{
			name: "flush and close failures aggregate",
			writer: &trackingWriteCloser{
				Writer:   errorWriter{err: writeFailure},
				closeErr: closeFailure,
			},
			groups:     csvTestGroups("small"),
			wantErrors: []error{writeFailure, closeFailure},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := writeCSV(tt.writer, tt.groups)
			require.Error(t, err)
			for _, want := range tt.wantErrors {
				assert.ErrorIs(t, err, want)
			}
			assert.True(t, tt.writer.closed)
		})
	}
}

type trackingWriteCloser struct {
	io.Writer
	closeErr error
	closed   bool
}

func (w *trackingWriteCloser) Close() error {
	w.closed = true
	return w.closeErr
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func csvTestGroups(summary string) map[GroupKey][]EvalRunResult {
	return map[GroupKey][]EvalRunResult{
		{Sample: "sample-a", Model: "model-a"}: {{
			Sample:      "sample-a",
			Model:       "model-a",
			Repetition:  1,
			TestsPassed: true,
			Progression: &RunProgression{Overall: Converged, Summary: summary},
		}},
	}
}
