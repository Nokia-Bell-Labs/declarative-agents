// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
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
