// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
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
		for _, r := range runs {
			byModel[r.Model] = append(byModel[r.Model], r)
		}
	}

	var stats []ModelStats
	for model, runs := range byModel {
		ms := computeModel(model, runs)
		stats = append(stats, ms)
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
	ms := ModelStats{Model: model, Runs: len(runs)}

	var iters, tokIn, tokOut []float64
	var durations []time.Duration
	var clean, converged, flat, regressing int
	runsWithFailures := 0

	for _, r := range runs {
		if r.TestsPassed {
			ms.Successes++
		}
		iters = append(iters, float64(r.Iterations))
		tokIn = append(tokIn, float64(r.TokensIn))
		tokOut = append(tokOut, float64(r.TokensOut))
		durations = append(durations, r.Duration)

		if r.Progression != nil {
			switch r.Progression.Overall {
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
	}

	n := float64(len(runs))
	ms.SuccessRate = float64(ms.Successes) / n
	ms.CleanRate = float64(clean) / n
	ms.MeanIter = meanFloat(iters)
	ms.MeanTokensIn = meanFloat(tokIn)
	ms.MeanTokensOut = meanFloat(tokOut)
	ms.MeanDuration = meanDur(durations)

	if runsWithFailures > 0 {
		ms.RecoveryRate = float64(converged) / float64(runsWithFailures)
		ms.StuckRate = float64(flat+regressing) / float64(runsWithFailures)
	}

	return ms
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
			Sample:       key.Sample,
			Model:        key.Model,
			Runs:         len(runs),
			Convergences: make(map[Convergence]int),
		}

		successes := 0
		var iters, tokens []float64
		var durs []time.Duration

		for _, r := range runs {
			if r.TestsPassed {
				successes++
			}
			iters = append(iters, float64(r.Iterations))
			tokens = append(tokens, float64(r.TokensIn+r.TokensOut))
			durs = append(durs, r.Duration)

			if r.Progression != nil {
				row.Convergences[r.Progression.Overall]++
			}
		}

		row.SuccessRate = float64(successes) / float64(len(runs))
		row.MeanIter = meanFloat(iters)
		row.MeanTokens = meanFloat(tokens)
		row.MeanDuration = meanDur(durs)
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

// PrintModelTable writes a model-level summary table.
func PrintModelTable(w io.Writer, stats []ModelStats) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "MODEL\tRUNS\tSUCCESS\tCLEAN\tRECOVERY\tSTUCK\tMEAN ITER\tMEAN TOK\tMEAN DUR\n")
	_, _ = fmt.Fprintf(tw, "-----\t----\t-------\t-----\t--------\t-----\t---------\t--------\t--------\n")

	for _, s := range stats {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%.0f%%\t%.0f%%\t%.0f%%\t%.0f%%\t%.1f\t%.0f\t%s\n",
			s.Model,
			s.Runs,
			s.SuccessRate*100,
			s.CleanRate*100,
			s.RecoveryRate*100,
			s.StuckRate*100,
			s.MeanIter,
			s.MeanTokensIn+s.MeanTokensOut,
			s.MeanDuration.Truncate(time.Second),
		)
	}
	_ = tw.Flush()

	totalRuns := 0
	totalSuccess := 0
	for _, s := range stats {
		totalRuns += s.Runs
		totalSuccess += s.Successes
	}
	overallRate := 0.0
	if totalRuns > 0 {
		overallRate = float64(totalSuccess) / float64(totalRuns) * 100
	}
	_, _ = fmt.Fprintf(w, "\n%d model rows, %d total runs, %.0f%% overall success\n",
		len(stats), totalRuns, overallRate)
}

// PrintDetailedTable writes per-(sample, model) rows with convergence counts.
func PrintDetailedTable(w io.Writer, rows []SampleModelRow) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "SAMPLE\tMODEL\tRUNS\tSUCCESS\tCLEAN\tCONVERGED\tIMPROVING\tFLAT\tREGRESS\tMEAN ITER\tMEAN DUR\n")
	_, _ = fmt.Fprintf(tw, "------\t-----\t----\t-------\t-----\t---------\t---------\t----\t-------\t---------\t--------\n")

	for _, r := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%.0f%%\t%d\t%d\t%d\t%d\t%d\t%.1f\t%s\n",
			r.Sample,
			r.Model,
			r.Runs,
			r.SuccessRate*100,
			r.Convergences[Clean],
			r.Convergences[Converged],
			r.Convergences[Improving],
			r.Convergences[Flat],
			r.Convergences[Regressing],
			r.MeanIter,
			r.MeanDuration.Truncate(time.Second),
		)
	}
	_ = tw.Flush()
}

// PrintProgression writes per-run tool progression timelines.
func PrintProgression(w io.Writer, groups map[GroupKey][]EvalRunResult) {
	type entry struct {
		key  GroupKey
		runs []EvalRunResult
	}
	var entries []entry
	for k, v := range groups {
		entries = append(entries, entry{k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].key.Sample != entries[j].key.Sample {
			return entries[i].key.Sample < entries[j].key.Sample
		}
		return entries[i].key.Model < entries[j].key.Model
	})

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "SAMPLE\tMODEL\tREP\tRESULT\tCONVERGENCE\tPROGRESSION\n")
	_, _ = fmt.Fprintf(tw, "------\t-----\t---\t------\t-----------\t-----------\n")

	for _, e := range entries {
		for _, r := range e.runs {
			result := "FAIL"
			if r.TestsPassed {
				result = "PASS"
			}
			conv := string(NoData)
			prog := "-"
			if r.Progression != nil {
				conv = string(r.Progression.Overall)
				prog = r.Progression.Summary
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\n",
				e.key.Sample, e.key.Model, r.Repetition, result, conv, prog)
		}
	}
	_ = tw.Flush()
}

// WriteCSV writes detailed per-run data as CSV.
func WriteCSV(path string, groups map[GroupKey][]EvalRunResult) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create CSV: %w", err)
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"sample", "model", "repetition",
		"tests_passed", "exit_code", "timed_out",
		"iterations", "tokens_in", "tokens_out",
		"duration_s", "convergence", "progression",
	}
	if err := w.Write(header); err != nil {
		return err
	}

	type entry struct {
		key  GroupKey
		runs []EvalRunResult
	}
	var entries []entry
	for k, v := range groups {
		entries = append(entries, entry{k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].key.String() < entries[j].key.String()
	})

	for _, e := range entries {
		for _, r := range e.runs {
			conv := ""
			prog := ""
			if r.Progression != nil {
				conv = string(r.Progression.Overall)
				prog = r.Progression.Summary
			}
			prog = strings.ReplaceAll(prog, "\n", " ")

			row := []string{
				r.Sample, r.Model,
				fmt.Sprintf("%d", r.Repetition),
				fmt.Sprintf("%t", r.TestsPassed),
				fmt.Sprintf("%d", r.ExitCode),
				fmt.Sprintf("%t", r.TimedOut),
				fmt.Sprintf("%d", r.Iterations),
				fmt.Sprintf("%d", r.TokensIn),
				fmt.Sprintf("%d", r.TokensOut),
				fmt.Sprintf("%.1f", r.Duration.Seconds()),
				conv, prog,
			}
			if err := w.Write(row); err != nil {
				return err
			}
		}
	}

	return nil
}

func meanFloat(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func meanDur(vals []time.Duration) time.Duration {
	if len(vals) == 0 {
		return 0
	}
	var sum time.Duration
	for _, v := range vals {
		sum += v
	}
	return sum / time.Duration(len(vals))
}
