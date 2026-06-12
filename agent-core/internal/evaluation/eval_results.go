// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// GroupKey identifies a (sample, model) combination for aggregating runs.
type GroupKey struct {
	Sample string
	Model  string
}

// String returns a stable key for sorting.
func (k GroupKey) String() string {
	return k.Sample + "/" + k.Model
}

// EvalRunResult holds all metrics and analysis for a single evaluation run.
type EvalRunResult struct {
	Sample      string
	Model       string
	Repetition  int
	TestsPassed bool
	ExitCode    int
	TimedOut    bool
	Iterations  int
	TokensIn    int
	TokensOut   int
	Duration    time.Duration
	Progression *RunProgression
}

// LoadMultiple loads and merges results from one or more session directories.
// Each directory should contain point subdirectories with meta.json files.
func LoadMultiple(dirs []string) (map[GroupKey][]EvalRunResult, error) {
	groups := make(map[GroupKey][]EvalRunResult)

	for _, dir := range dirs {
		results, err := loadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", dir, err)
		}
		for _, r := range results {
			key := GroupKey{Sample: r.Sample, Model: r.Model}
			groups[key] = append(groups[key], r)
		}
	}

	return groups, nil
}

func loadDir(dir string) ([]EvalRunResult, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var results []EvalRunResult
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pointDir := filepath.Join(dir, e.Name())
		metaPath := filepath.Join(pointDir, ArtifactMeta)

		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}

		var meta EvalMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}

		r := EvalRunResult{
			Sample:      meta.Sample,
			Model:       meta.Model,
			Repetition:  meta.Repetition,
			TestsPassed: meta.TestsPassed,
			ExitCode:    meta.ExitCode,
			TimedOut:    meta.TimedOut,
			Duration:    meta.Duration,
		}

		// Token counts may be stored in meta or derived from traces.
		r.TokensIn, r.TokensOut, r.Iterations = extractTokensFromTrace(pointDir)

		tracePath := filepath.Join(pointDir, ArtifactTrace)
		if spans, err := ReadTraceFile(tracePath); err == nil && len(spans) > 0 {
			snapshots := ExtractToolSnapshots(spans)
			prog := Classify(snapshots, meta.TestsPassed)
			r.Progression = &prog
			if r.Iterations == 0 {
				r.Iterations = countIterations(spans)
			}
		}

		results = append(results, r)
	}

	return results, nil
}

func extractTokensFromTrace(pointDir string) (tokensIn, tokensOut, iterations int) {
	tracePath := filepath.Join(pointDir, ArtifactTrace)
	spans, err := ReadTraceFile(tracePath)
	if err != nil {
		return 0, 0, 0
	}

	for _, s := range spans {
		if HasAttr(s, "gen_ai.usage.input_tokens") {
			tokensIn += IntAttr(s, "gen_ai.usage.input_tokens")
			tokensOut += IntAttr(s, "gen_ai.usage.output_tokens")
		}
	}

	return tokensIn, tokensOut, countIterations(spans)
}

func countIterations(spans []*Span) int {
	count := 0
	for _, s := range spans {
		if s.Name == "loop/iteration" || s.Name == "loop_iteration" {
			count++
		}
	}
	if count == 0 {
		count = len(ToolSpans(spans))
	}
	return count
}
