// Copyright (c) 2026 Nokia. All rights reserved.

package validation

import (
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

type specSnapshot struct {
	targetDirectory string
	suitePaths      []string
	corpus          *spec.Corpus
	graph           *spec.Graph
	charters        []spec.Charter
	findings        []spec.Finding
	hasErrors       bool
}

func snapshotSpec(vs *SpecState) specSnapshot {
	return specSnapshot{
		targetDirectory: vs.TargetDirectory,
		suitePaths:      append([]string(nil), vs.SuitePaths...),
		corpus:          vs.Corpus,
		graph:           vs.Graph,
		charters:        append([]spec.Charter(nil), vs.Charters...),
		findings:        append([]spec.Finding(nil), vs.Findings...),
		hasErrors:       vs.HasErrors,
	}
}

func (s specSnapshot) restore(vs *SpecState) {
	vs.TargetDirectory = s.targetDirectory
	vs.SuitePaths = append([]string(nil), s.suitePaths...)
	vs.Corpus = s.corpus
	vs.Graph = s.graph
	vs.Charters = append([]spec.Charter(nil), s.charters...)
	vs.Findings = append([]spec.Finding(nil), s.findings...)
	vs.HasErrors = s.hasErrors
}

// specReceipt is the opaque, tool-owned rollback context the spec-validation tools
// encode into Result.Receipt. The corpus and graph are in-process artifacts rebuilt
// from the project directory, so the receipt records only whether each was present
// before the step (to clear ones the step created) alongside the serializable
// findings and error flag (srd035-checkpoint-port R3; #44 R2).
type specReceipt struct {
	TargetDirectory string         `json:"target_directory,omitempty"`
	SuitePaths      []string       `json:"suite_paths,omitempty"`
	CorpusLoaded    bool           `json:"corpus_loaded"`
	GraphLoaded     bool           `json:"graph_loaded"`
	Charters        []spec.Charter `json:"charters,omitempty"`
	Findings        []spec.Finding `json:"findings,omitempty"`
	HasErrors       bool           `json:"has_errors,omitempty"`
}

func encodeSpecReceipt(snap specSnapshot) string {
	b, err := json.Marshal(specReceipt{
		TargetDirectory: snap.targetDirectory,
		SuitePaths:      snap.suitePaths,
		CorpusLoaded:    snap.corpus != nil,
		GraphLoaded:     snap.graph != nil,
		Charters:        snap.charters,
		Findings:        snap.findings,
		HasErrors:       snap.hasErrors,
	})
	if err != nil {
		return ""
	}
	return string(b)
}

func decodeSpecReceipt(receipt string) (specReceipt, bool, error) {
	if receipt == "" {
		return specReceipt{}, false, nil
	}
	var r specReceipt
	if err := json.Unmarshal([]byte(receipt), &r); err != nil {
		return specReceipt{}, false, err
	}
	return r, true, nil
}

func (r specReceipt) restore(vs *SpecState) {
	vs.TargetDirectory = r.TargetDirectory
	vs.SuitePaths = append([]string(nil), r.SuitePaths...)
	if !r.CorpusLoaded {
		vs.Corpus = nil
	}
	if !r.GraphLoaded {
		vs.Graph = nil
	}
	vs.Charters = append([]spec.Charter(nil), r.Charters...)
	vs.Findings = append([]spec.Finding(nil), r.Findings...)
	vs.HasErrors = r.HasErrors
}

// undoSpecState reverses a spec-validation step. It prefers the opaque receipt on
// the prior Result (so a fresh command instance can undo after a process restart)
// and falls back to the in-memory snapshot on the live path.
func undoSpecState(commandName string, vs *SpecState, prior core.Result, snap specSnapshot, ok bool) core.Result {
	if r, present, err := decodeSpecReceipt(prior.Receipt); err != nil {
		e := fmt.Errorf("undo %s: decode receipt: %w", commandName, err)
		return core.Result{Signal: core.CommandError, CommandName: commandName, Output: e.Error(), Err: e}
	} else if present {
		r.restore(vs)
		return core.Result{Signal: core.ToolDone, CommandName: commandName, Output: "undo: restored validation state"}
	}
	if !ok {
		err := fmt.Errorf("undo %s: no validation state snapshot recorded", commandName)
		return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
	}
	snap.restore(vs)
	return core.Result{Signal: core.ToolDone, CommandName: commandName, Output: "undo: restored validation state"}
}
