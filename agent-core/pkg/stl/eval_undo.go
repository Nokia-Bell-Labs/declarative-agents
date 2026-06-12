// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/spec"
)

type evalSessionSnapshot struct {
	suite        SuiteConfig
	sessionDir   string
	pointMachine string
	result       SessionResult
	pc           *PointContext
	gridPoints   []GridPoint
	reps         int
	timeout      int64
	ollamaURL    string
	llmTimeout   int64
	hIdx         int
	mIdx         int
	gIdx         int
	sIdx         int
	rIdx         int
	pIdx         int
	started      bool
	exhausted    bool
	startUnixNS  int64
}

func snapshotEvalSession(es *EvalSessionState) evalSessionSnapshot {
	snap := evalSessionSnapshot{
		suite:        cloneSuiteConfig(es.Suite),
		sessionDir:   es.SessionDir,
		pointMachine: es.PointMachine,
		result:       cloneSessionResult(es.Result),
		pc:           clonePointContext(es.PC),
		gridPoints:   cloneGridPoints(es.gridPoints),
		reps:         es.reps,
		timeout:      int64(es.timeout),
		ollamaURL:    es.ollamaURL,
		llmTimeout:   int64(es.llmTimeout),
		hIdx:         es.hIdx,
		mIdx:         es.mIdx,
		gIdx:         es.gIdx,
		sIdx:         es.sIdx,
		rIdx:         es.rIdx,
		pIdx:         es.pIdx,
		started:      es.started,
		exhausted:    es.exhausted,
	}
	if !es.start.IsZero() {
		snap.startUnixNS = es.start.UnixNano()
	}
	return snap
}

func (s evalSessionSnapshot) restore(es *EvalSessionState) {
	es.Suite = cloneSuiteConfig(s.suite)
	es.SessionDir = s.sessionDir
	es.PointMachine = s.pointMachine
	es.Result = cloneSessionResult(s.result)
	es.PC = clonePointContext(s.pc)
	es.gridPoints = cloneGridPoints(s.gridPoints)
	es.reps = s.reps
	es.timeout = time.Duration(s.timeout)
	es.ollamaURL = s.ollamaURL
	es.llmTimeout = time.Duration(s.llmTimeout)
	es.hIdx, es.mIdx, es.gIdx, es.sIdx, es.rIdx = s.hIdx, s.mIdx, s.gIdx, s.sIdx, s.rIdx
	es.pIdx = s.pIdx
	es.started = s.started
	es.exhausted = s.exhausted
	if s.startUnixNS == 0 {
		es.start = time.Time{}
	} else {
		es.start = time.Unix(0, s.startUnixNS)
	}
}

func undoEvalSessionSnapshot(commandName string, es *EvalSessionState, snap evalSessionSnapshot, ok bool) core.Result {
	if !ok {
		err := fmt.Errorf("undo %s: no evaluator session snapshot recorded", commandName)
		return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
	}
	snap.restore(es)
	return core.Result{Signal: core.ToolDone, CommandName: commandName, Output: "undo: restored evaluator session state"}
}

func evalSessionMemento(commandName string, snap evalSessionSnapshot, ok bool) (core.UndoMemento, error) {
	if !ok {
		return core.UndoMemento{}, fmt.Errorf("%w: no evaluator session snapshot recorded for %s", core.ErrUndoMementoMissing, commandName)
	}
	return core.NewUndoMemento(commandName, core.UndoMementoReversible, struct {
		DomainState struct {
			SuiteName   string `json:"suite_name,omitempty"`
			SessionDir  string `json:"session_dir,omitempty"`
			TotalPoints int    `json:"total_points"`
			GridPoints  int    `json:"grid_points"`
			Started     bool   `json:"started"`
			Exhausted   bool   `json:"exhausted"`
		} `json:"domain_state"`
	}{
		DomainState: struct {
			SuiteName   string `json:"suite_name,omitempty"`
			SessionDir  string `json:"session_dir,omitempty"`
			TotalPoints int    `json:"total_points"`
			GridPoints  int    `json:"grid_points"`
			Started     bool   `json:"started"`
			Exhausted   bool   `json:"exhausted"`
		}{
			SuiteName:   snap.suite.Name,
			SessionDir:  snap.sessionDir,
			TotalPoints: snap.result.TotalPoints,
			GridPoints:  len(snap.gridPoints),
			Started:     snap.started,
			Exhausted:   snap.exhausted,
		},
	})
}

type pointContextSnapshot struct {
	point *PointContext
}

func snapshotPointContext(pc *PointContext) pointContextSnapshot {
	return pointContextSnapshot{point: clonePointContext(pc)}
}

func undoPointContextSnapshot(commandName string, pc *PointContext, snap pointContextSnapshot, ok bool) core.Result {
	if !ok || snap.point == nil {
		err := fmt.Errorf("undo %s: no point context snapshot recorded", commandName)
		return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
	}
	*pc = *clonePointContext(snap.point)
	return core.Result{Signal: core.ToolDone, CommandName: commandName, Output: "undo: restored point context"}
}

func pointContextMemento(commandName string, snap pointContextSnapshot, ok bool) (core.UndoMemento, error) {
	if !ok || snap.point == nil {
		return core.UndoMemento{}, fmt.Errorf("%w: no point context snapshot recorded for %s", core.ErrUndoMementoMissing, commandName)
	}
	return core.NewUndoMemento(commandName, core.UndoMementoReversible, struct {
		DomainState struct {
			PointID         string `json:"point_id,omitempty"`
			PointDir        string `json:"point_dir,omitempty"`
			Tokens          int    `json:"tokens"`
			TestsPassed     bool   `json:"tests_passed"`
			VersionMismatch bool   `json:"version_mismatch"`
		} `json:"domain_state"`
	}{
		DomainState: struct {
			PointID         string `json:"point_id,omitempty"`
			PointDir        string `json:"point_dir,omitempty"`
			Tokens          int    `json:"tokens"`
			TestsPassed     bool   `json:"tests_passed"`
			VersionMismatch bool   `json:"version_mismatch"`
		}{
			PointID:         snap.point.PointID,
			PointDir:        snap.point.PointDir,
			Tokens:          snap.point.Tokens,
			TestsPassed:     snap.point.TestsPassed,
			VersionMismatch: snap.point.VersionMismatch,
		},
	})
}

type validateSpecSnapshot struct {
	corpus    *spec.Corpus
	graph     *spec.Graph
	findings  []spec.Finding
	hasErrors bool
}

func snapshotValidateSpec(vs *ValidateSpecState) validateSpecSnapshot {
	return validateSpecSnapshot{
		corpus:    vs.Corpus,
		graph:     vs.Graph,
		findings:  append([]spec.Finding(nil), vs.Findings...),
		hasErrors: vs.HasErrors,
	}
}

func (s validateSpecSnapshot) restore(vs *ValidateSpecState) {
	vs.Corpus = s.corpus
	vs.Graph = s.graph
	vs.Findings = append([]spec.Finding(nil), s.findings...)
	vs.HasErrors = s.hasErrors
}

func undoValidateSpecSnapshot(commandName string, vs *ValidateSpecState, snap validateSpecSnapshot, ok bool) core.Result {
	if !ok {
		err := fmt.Errorf("undo %s: no validation state snapshot recorded", commandName)
		return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
	}
	snap.restore(vs)
	return core.Result{Signal: core.ToolDone, CommandName: commandName, Output: "undo: restored validation state"}
}

func validateSpecMemento(commandName string, snap validateSpecSnapshot, ok bool) (core.UndoMemento, error) {
	if !ok {
		return core.UndoMemento{}, fmt.Errorf("%w: no validation state snapshot recorded for %s", core.ErrUndoMementoMissing, commandName)
	}
	return core.NewUndoMemento(commandName, core.UndoMementoReversible, struct {
		DomainState struct {
			CorpusLoaded bool `json:"corpus_loaded"`
			GraphLoaded  bool `json:"graph_loaded"`
			Findings     int  `json:"findings"`
			HasErrors    bool `json:"has_errors"`
		} `json:"domain_state"`
	}{
		DomainState: struct {
			CorpusLoaded bool `json:"corpus_loaded"`
			GraphLoaded  bool `json:"graph_loaded"`
			Findings     int  `json:"findings"`
			HasErrors    bool `json:"has_errors"`
		}{
			CorpusLoaded: snap.corpus != nil,
			GraphLoaded:  snap.graph != nil,
			Findings:     len(snap.findings),
			HasErrors:    snap.hasErrors,
		},
	})
}

func cloneSuiteConfig(in SuiteConfig) SuiteConfig {
	out := in
	out.Harnesses = append([]Harness(nil), in.Harnesses...)
	out.Models = append([]string(nil), in.Models...)
	out.Profiles = append([]SuiteProfile(nil), in.Profiles...)
	out.Samples = append([]Sample(nil), in.Samples...)
	if in.Grid != nil {
		out.Grid = make(map[string][]any, len(in.Grid))
		for k, values := range in.Grid {
			out.Grid[k] = append([]any(nil), values...)
		}
	}
	return out
}

func cloneSessionResult(in SessionResult) SessionResult {
	out := in
	out.Points = append([]PointResult(nil), in.Points...)
	return out
}

func cloneGridPoints(in []GridPoint) []GridPoint {
	if in == nil {
		return nil
	}
	out := make([]GridPoint, len(in))
	for i, gp := range in {
		out[i] = cloneGridPoint(gp)
	}
	return out
}

func cloneGridPoint(in GridPoint) GridPoint {
	if in == nil {
		return nil
	}
	out := make(GridPoint, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func clonePointContext(in *PointContext) *PointContext {
	if in == nil {
		return nil
	}
	out := *in
	out.GridPoint = cloneGridPoint(in.GridPoint)
	if in.Harness.Flags != nil {
		out.Harness.Flags = make(map[string]interface{}, len(in.Harness.Flags))
		for k, v := range in.Harness.Flags {
			out.Harness.Flags[k] = v
		}
	}
	return &out
}
