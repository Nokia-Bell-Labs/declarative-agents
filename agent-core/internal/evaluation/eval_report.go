// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"sort"
	"time"
)

// ModelStats aggregates convergence metrics across all runs of a model.
type ModelStats struct {
	Model         string
	Runs          int
	Successes     int
	SuccessRate   float64
	CleanRate     float64
	RecoveryRate  float64
	StuckRate     float64
	MeanIter      float64
	MeanTokensIn  float64
	MeanTokensOut float64
	MeanDuration  time.Duration
}

// ComputeModelStats builds model-level statistics from grouped results.
func ComputeModelStats(groups map[GroupKey][]EvalRunResult) []ModelStats {
	byModel := make(map[string][]EvalRunResult)
	for _, runs := range groups {
		for _, result := range runs {
			byModel[result.Model] = append(byModel[result.Model], result)
		}
	}
	stats := make([]ModelStats, 0, len(byModel))
	for model, runs := range byModel {
		stats = append(stats, computeModel(model, runs))
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].SuccessRate != stats[j].SuccessRate {
			return stats[i].SuccessRate > stats[j].SuccessRate
		}
		return stats[i].Model < stats[j].Model
	})
	return stats
}

func computeModel(model string, runs []EvalRunResult) ModelStats {
	stats := ModelStats{Model: model, Runs: len(runs)}
	var iterations, tokensIn, tokensOut []float64
	var durations []time.Duration
	var clean, converged, flat, regressing, runsWithFailures int
	for _, result := range runs {
		if result.TestsPassed {
			stats.Successes++
		}
		iterations = append(iterations, float64(result.Iterations))
		tokensIn = append(tokensIn, float64(result.TokensIn))
		tokensOut = append(tokensOut, float64(result.TokensOut))
		durations = append(durations, result.Duration)
		if result.Progression == nil {
			continue
		}
		switch result.Progression.Overall {
		case Clean:
			clean++
		case Converged:
			converged++
			runsWithFailures++
		case Improving:
			runsWithFailures++
		case Flat:
			flat++
			runsWithFailures++
		case Regressing:
			regressing++
			runsWithFailures++
		}
	}
	count := float64(len(runs))
	stats.SuccessRate = float64(stats.Successes) / count
	stats.CleanRate = float64(clean) / count
	stats.MeanIter = meanFloat(iterations)
	stats.MeanTokensIn = meanFloat(tokensIn)
	stats.MeanTokensOut = meanFloat(tokensOut)
	stats.MeanDuration = meanDur(durations)
	if runsWithFailures > 0 {
		stats.RecoveryRate = float64(converged) / float64(runsWithFailures)
		stats.StuckRate = float64(flat+regressing) / float64(runsWithFailures)
	}
	return stats
}

// SampleModelRow is a per-(sample, model) row with progression data.
type SampleModelRow struct {
	Sample       string
	Model        string
	Runs         int
	SuccessRate  float64
	MeanIter     float64
	MeanTokens   float64
	MeanDuration time.Duration
	Convergences map[Convergence]int
}

// ComputeDetailed builds per-(sample, model) rows.
func ComputeDetailed(groups map[GroupKey][]EvalRunResult) []SampleModelRow {
	var rows []SampleModelRow
	for key, runs := range groups {
		row := SampleModelRow{
			Sample: key.Sample, Model: key.Model, Runs: len(runs),
			Convergences: make(map[Convergence]int),
		}
		var successes int
		var iterations, tokens []float64
		var durations []time.Duration
		for _, result := range runs {
			if result.TestsPassed {
				successes++
			}
			iterations = append(iterations, float64(result.Iterations))
			tokens = append(tokens, float64(result.TokensIn+result.TokensOut))
			durations = append(durations, result.Duration)
			if result.Progression != nil {
				row.Convergences[result.Progression.Overall]++
			}
		}
		row.SuccessRate = float64(successes) / float64(len(runs))
		row.MeanIter = meanFloat(iterations)
		row.MeanTokens = meanFloat(tokens)
		row.MeanDuration = meanDur(durations)
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Sample != rows[j].Sample {
			return rows[i].Sample < rows[j].Sample
		}
		return rows[i].Model < rows[j].Model
	})
	return rows
}

func meanFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func meanDur(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	var sum time.Duration
	for _, value := range values {
		sum += value
	}
	return sum / time.Duration(len(values))
}
