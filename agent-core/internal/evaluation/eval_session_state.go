// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// Session-level signals for the evaluator session machine.
const (
	SigSuiteConfigParsed      core.Signal = "SuiteConfigParsed"
	SigSuiteSamplesDiscovered core.Signal = "SuiteSamplesDiscovered"
	SigEvalGridExpanded       core.Signal = "EvalGridExpanded"
	SigEvalSessionInitialized core.Signal = "EvalSessionInitialized"
	SigSuiteLoaded            core.Signal = "SuiteLoaded"
	SigPointReady             core.Signal = "PointReady"
	SigPointDone              core.Signal = "PointDone"
	SigAllPointsDone          core.Signal = "AllPointsDone"
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

	Suite        SuiteConfig
	SessionDir   string
	PointMachine string
	Result       SessionResult
	Stderr       io.Writer

	// Grid iteration state
	gridPoints []GridPoint
	reps       int
	timeout    time.Duration
	ollamaURL  string
	llmTimeout time.Duration

	// Iterator indices into the (harness, model, gridpoint, sample, rep) space (legacy)
	// or (profile, gridpoint, sample, rep) space (profile mode)
	hIdx, mIdx, gIdx, sIdx, rIdx int
	pIdx                         int
	started                      bool
	exhausted                    bool

	start time.Time
}

// InitSession prepares the session for iteration. Must be called after Suite is
// populated, samples are discovered, and the grid has been expanded.
func (s *EvalSessionState) InitSession(outputDir string, reps int, timeout time.Duration, ollamaURL string, llmTimeout time.Duration) error {
	s.SessionDir = filepath.Join(outputDir, s.Suite.Name, time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(s.SessionDir, 0o755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	if len(s.gridPoints) == 0 {
		s.ExpandGrid()
	}

	s.reps = reps
	if s.reps < 1 {
		s.reps = 1
	}
	s.timeout = timeout
	if s.timeout == 0 {
		s.timeout = 10 * time.Minute
	}
	s.ollamaURL = ollamaURL
	s.llmTimeout = llmTimeout

	if s.Stderr == nil {
		s.Stderr = os.Stderr
	}

	s.start = time.Now()
	return nil
}

// ExpandGrid materializes the suite's grid into iteration points.
func (s *EvalSessionState) ExpandGrid() {
	s.gridPoints = expandGrid(s.Suite.Grid)
	if len(s.gridPoints) == 0 {
		s.gridPoints = []GridPoint{{}}
	}
}

// NextPoint advances the iterator to the next grid point. Returns the
// PointContext and true if a point is available, or nil and false if
// the grid is exhausted.
func (s *EvalSessionState) NextPoint() (*PointContext, bool) {
	if s.exhausted {
		return nil, false
	}

	if len(s.Suite.Profiles) > 0 {
		return s.nextPointProfile()
	}
	return s.nextPointLegacy()
}

func (s *EvalSessionState) nextPointProfile() (*PointContext, bool) {
	if !s.started {
		s.started = true
		s.pIdx, s.gIdx, s.sIdx, s.rIdx = 0, 0, 0, 0
	} else {
		s.rIdx++
		if s.rIdx >= s.reps {
			s.rIdx = 0
			s.sIdx++
		}
		if s.sIdx >= len(s.Suite.Samples) {
			s.sIdx = 0
			s.gIdx++
		}
		if s.gIdx >= len(s.gridPoints) {
			s.gIdx = 0
			s.pIdx++
		}
		if s.pIdx >= len(s.Suite.Profiles) {
			s.exhausted = true
			return nil, false
		}
	}

	sp := s.Suite.Profiles[s.pIdx]
	gp := s.gridPoints[s.gIdx]
	sample := s.Suite.Samples[s.sIdx]

	pointID := EvalPointID(sample.Name, sp.Name, sp.Model, gp, s.rIdx)

	pc := &PointContext{
		SessionDir:  s.SessionDir,
		PointID:     pointID,
		Sample:      sample,
		Harness:     Harness{Name: sp.Name, Binary: sp.Binary},
		Model:       sp.Model,
		ProfilePath: sp.Path,
		GridPoint:   gp,
		Rep:         s.rIdx,
		Timeout:     s.timeout,
		LLMTimeout:  s.llmTimeout,
		OllamaURL:   s.ollamaURL,
		Stderr:      s.Stderr,
	}

	return pc, true
}

func (s *EvalSessionState) nextPointLegacy() (*PointContext, bool) {
	if !s.started {
		s.started = true
		s.hIdx, s.mIdx, s.gIdx, s.sIdx, s.rIdx = 0, 0, 0, 0, 0
	} else {
		s.rIdx++
		if s.rIdx >= s.reps {
			s.rIdx = 0
			s.sIdx++
		}
		if s.sIdx >= len(s.Suite.Samples) {
			s.sIdx = 0
			s.gIdx++
		}
		if s.gIdx >= len(s.gridPoints) {
			s.gIdx = 0
			s.mIdx++
		}
		if s.mIdx >= len(s.Suite.Models) {
			s.mIdx = 0
			s.hIdx++
		}
		if s.hIdx >= len(s.Suite.Harnesses) {
			s.exhausted = true
			return nil, false
		}
	}

	harness := s.Suite.Harnesses[s.hIdx]
	model := s.Suite.Models[s.mIdx]
	gp := s.gridPoints[s.gIdx]
	sample := s.Suite.Samples[s.sIdx]

	pointID := EvalPointID(sample.Name, harness.Name, model, gp, s.rIdx)

	pc := &PointContext{
		SessionDir: s.SessionDir,
		PointID:    pointID,
		Sample:     sample,
		Harness:    harness,
		Model:      model,
		GridPoint:  gp,
		Rep:        s.rIdx,
		Timeout:    s.timeout,
		LLMTimeout: s.llmTimeout,
		OllamaURL:  s.ollamaURL,
		Stderr:     s.Stderr,
	}

	return pc, true
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
