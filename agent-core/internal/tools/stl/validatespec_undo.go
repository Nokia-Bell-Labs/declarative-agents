// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/spec"
)

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
