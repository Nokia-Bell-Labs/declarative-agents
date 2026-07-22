// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"encoding/json"
	"errors"
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
	var loadErrors []error
	seen := make(map[string]string)

	for _, dir := range dirs {
		results, err := loadDir(dir)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Errorf("load %s: %w", dir, err))
		}
		for _, r := range results {
			identity := fmt.Sprintf("%s/%s/%d", r.Sample, r.Model, r.Repetition)
			if previous, duplicate := seen[identity]; duplicate {
				loadErrors = append(loadErrors, fmt.Errorf("duplicate evaluation point %s in %s and %s", identity, previous, dir))
				continue
			}
			seen[identity] = dir
			key := GroupKey{Sample: r.Sample, Model: r.Model}
			groups[key] = append(groups[key], r)
		}
	}

	return groups, errors.Join(loadErrors...)
}

func loadDir(dir string) ([]EvalRunResult, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var results []EvalRunResult
	var loadErrors []error
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pointDir := filepath.Join(dir, e.Name())
		metaPath := filepath.Join(pointDir, ArtifactMeta)

		data, err := os.ReadFile(metaPath)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Errorf("%s: read %s: %w", e.Name(), ArtifactMeta, err))
			continue
		}

		var meta EvalMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			loadErrors = append(loadErrors, fmt.Errorf("%s: parse %s: %w", e.Name(), ArtifactMeta, err))
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
		r.TokensIn, r.TokensOut, r.Iterations, err = extractTokensFromTrace(pointDir)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Errorf("%s: read %s: %w", e.Name(), ArtifactTrace, err))
		}

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

	return results, errors.Join(loadErrors...)
}

func extractTokensFromTrace(pointDir string) (tokensIn, tokensOut, iterations int, err error) {
	tracePath := filepath.Join(pointDir, ArtifactTrace)
	spans, err := ReadTraceFile(tracePath)
	if err != nil {
		return 0, 0, 0, err
	}
	if len(spans) == 0 {
		return 0, 0, 0, fmt.Errorf("trace contains no spans")
	}

	for _, s := range spans {
		if HasAttr(s, "gen_ai.usage.input_tokens") {
			tokensIn += IntAttr(s, "gen_ai.usage.input_tokens")
			tokensOut += IntAttr(s, "gen_ai.usage.output_tokens")
		}
	}

	return tokensIn, tokensOut, countIterations(spans), nil
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
