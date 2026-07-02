// Copyright (c) 2026 Nokia. All rights reserved.

package validation

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

type specSnapshot struct {
	corpus    *spec.Corpus
	graph     *spec.Graph
	findings  []spec.Finding
	hasErrors bool
}

func snapshotSpec(vs *SpecState) specSnapshot {
	return specSnapshot{
		corpus:    vs.Corpus,
		graph:     vs.Graph,
		findings:  append([]spec.Finding(nil), vs.Findings...),
		hasErrors: vs.HasErrors,
	}
}

func (s specSnapshot) restore(vs *SpecState) {
	vs.Corpus = s.corpus
	vs.Graph = s.graph
	vs.Findings = append([]spec.Finding(nil), s.findings...)
	vs.HasErrors = s.hasErrors
}

func undoSpecSnapshot(commandName string, vs *SpecState, snap specSnapshot, ok bool) core.Result {
	if !ok {
		err := fmt.Errorf("undo %s: no validation state snapshot recorded", commandName)
		return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
	}
	snap.restore(vs)
	return core.Result{Signal: core.ToolDone, CommandName: commandName, Output: "undo: restored validation state"}
}

func specMemento(commandName string, snap specSnapshot, ok bool) (core.UndoMemento, error) {
	if !ok {
		return core.UndoMemento{}, fmt.Errorf("%w: no validation state snapshot recorded for %s", core.ErrUndoMementoMissing, commandName)
	}
	payload := struct {
		DomainState struct {
			CorpusLoaded bool `json:"corpus_loaded"`
			GraphLoaded  bool `json:"graph_loaded"`
			Findings     int  `json:"findings"`
			HasErrors    bool `json:"has_errors"`
		} `json:"domain_state"`
	}{}
	payload.DomainState.CorpusLoaded = snap.corpus != nil
	payload.DomainState.GraphLoaded = snap.graph != nil
	payload.DomainState.Findings = len(snap.findings)
	payload.DomainState.HasErrors = snap.hasErrors
	return core.NewUndoMemento(commandName, core.UndoMementoReversible, payload)
}
