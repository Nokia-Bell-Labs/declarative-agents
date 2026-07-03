// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"fmt"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
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
	es.pIdx, es.gIdx, es.sIdx, es.rIdx = s.pIdx, s.gIdx, s.sIdx, s.rIdx
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

func cloneSuiteConfig(in SuiteConfig) SuiteConfig {
	out := in
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
	return &out
}
