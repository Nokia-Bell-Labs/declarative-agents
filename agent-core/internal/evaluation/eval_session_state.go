// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package evaluation

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// Session-level signals for the evaluator session machine.
const (
	SigSuiteConfigParsed      core.Signal = "SuiteConfigParsed"
	SigSuiteSamplesDiscovered core.Signal = "SuiteSamplesDiscovered"
	SigEvalGridExpanded       core.Signal = "EvalGridExpanded"
	SigEvalSessionInitialized core.Signal = "EvalSessionInitialized"
	SigSuiteLoaded            core.Signal = "SuiteLoaded"
	SigEvalPointsMaterialized core.Signal = "EvalPointsMaterialized"
	SigPointDone              core.Signal = "PointDone"
	SigPointsCompleted        core.Signal = "PointsCompleted"
	SigSessionReported        core.Signal = "SessionReported"
)

// EvalSessionState holds session-level state for the evaluator session
// machine. It extends EvalState (which holds the per-point PC) with
// suite configuration, grid iteration, and result accumulation.
type EvalSessionState struct {
	EvalState

	// Configured from CLI flags / tool YAML config
	SuitePath string
	OutputDir string
	Reps      int
	Timeout   time.Duration
	OllamaURL string
	// Declared defaults apply only when neither runtime nor suite input selects
	// a value. They are populated from ToolDef config, not Go literals.
	DefaultOutputDir string
	DefaultReps      int
	DefaultTimeout   time.Duration
	SampleLayout     SampleLayout
	// ChildAgentBinary overrides the harness binary the evaluator launches for
	// each suite profile. Empty means the default "agent" (resolved from PATH).
	ChildAgentBinary string
	CoreRoot         string

	Suite        SuiteConfig
	SessionDir   string
	PointMachine string
	PointSpec    *core.MachineSpec
	Result       SessionResult
	Stderr       io.Writer
	Tracer       tracing.Tracer

	// Grid iteration state
	gridPoints []GridPoint
	reps       int
	timeout    time.Duration
	ollamaURL  string
	llmTimeout time.Duration

	start time.Time
}

// InitSession prepares the session for iteration. Must be called after Suite is
// populated, samples are discovered, and the grid has been expanded.
func (s *EvalSessionState) InitSession(outputDir string, reps int, timeout time.Duration, ollamaURL string, llmTimeout time.Duration) error {
	if len(s.gridPoints) == 0 {
		return fmt.Errorf("initialize session requires expand_eval_grid")
	}
	if outputDir == "" {
		return fmt.Errorf("initialize session requires output_dir")
	}
	if reps < 1 {
		return fmt.Errorf("initialize session requires reps of at least 1")
	}
	if err := validateEvaluatorPointTimeout(timeout); err != nil {
		return fmt.Errorf("initialize session: %w", err)
	}
	if _, err := s.aggregateSessionDuration(reps, timeout); err != nil {
		return fmt.Errorf("initialize session: %w", err)
	}
	s.SessionDir = filepath.Join(outputDir, s.Suite.Name, time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(s.SessionDir, 0o755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	s.reps = reps
	s.timeout = timeout
	s.ollamaURL = ollamaURL
	s.llmTimeout = llmTimeout

	if s.Stderr == nil {
		s.Stderr = os.Stderr
	}

	s.start = time.Now()
	return nil
}

func (s *EvalSessionState) aggregateSessionDuration(reps int, timeout time.Duration) (time.Duration, error) {
	dimensions := []struct {
		name  string
		value int
	}{
		{name: "profiles", value: len(s.Suite.Profiles)},
		{name: "grid points", value: len(s.gridPoints)},
		{name: "samples", value: len(s.Suite.Samples)},
		{name: "repetitions", value: reps},
	}

	pointCount := int64(1)
	for _, dimension := range dimensions {
		if dimension.value < 1 {
			return 0, fmt.Errorf("aggregate duration requires at least one %s", dimension.name)
		}
		value := int64(dimension.value)
		if pointCount > math.MaxInt64/value {
			return 0, fmt.Errorf("aggregate duration overflows while multiplying %s", dimension.name)
		}
		pointCount *= value
	}

	timeoutNanos := int64(timeout)
	if pointCount > math.MaxInt64/timeoutNanos {
		return 0, fmt.Errorf("aggregate duration overflows for %d points at %s", pointCount, timeout)
	}
	aggregate := time.Duration(pointCount * timeoutNanos)
	if aggregate > maxEvaluatorSessionDuration {
		return 0, fmt.Errorf("aggregate duration %s exceeds maximum %s", aggregate, maxEvaluatorSessionDuration)
	}
	return aggregate, nil
}

// ExpandGrid materializes the suite's grid into iteration points.
func (s *EvalSessionState) ExpandGrid() {
	s.gridPoints = expandGrid(s.Suite.Grid)
	if len(s.gridPoints) == 0 {
		s.gridPoints = []GridPoint{{}}
	}
}

// RecordPoint records a completed point's results into the session accumulator.
func (s *EvalSessionState) RecordPoint(pc *PointContext) {
	pr := PointResult{
		PointID:     pc.PointID,
		Sample:      pc.Sample.Name,
		Harness:     pc.Harness.Name,
		Model:       pc.Model,
		TestsPassed: pc.TestsPassed,
		TimedOut:    pc.TimedOut,
		ExitCode:    pc.ExitCode,
		Tokens:      pc.Tokens,
		Duration:    pc.Duration,
	}

	s.Result.Points = append(s.Result.Points, pr)
	s.Result.TotalPoints++
	if pc.TestsPassed {
		s.Result.Passed++
	} else if pc.TimedOut {
		s.Result.TimedOut++
	} else {
		s.Result.Failed++
	}
}

// FinalizeSession sets the total duration on the result.
func (s *EvalSessionState) FinalizeSession() {
	s.Result.Duration = time.Since(s.start)
}
