// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"fmt"
)

// ResumeState is the loaded snapshot plus loop params seeded to re-enter the
// machine at the restored position. Callers restore domain-owned state (for
// example the conversation carried in Position.Snapshot.Conversation) from the
// typed snapshot before running (srd035-checkpoint-port R4, R6.2).
type ResumeState struct {
	Params    LoopParams
	Position  Position
	Execution Execution
}

// LoadResume loads the persisted Position and Execution through params.Checkpoint
// and returns params seeded to re-enter the loop at the restored position. It is
// the single, hook-free resume contract: no ValidateCheckpoint, RestoreConversation,
// RestoreDomain, or workspace restore fan-out. The resume signal is params.InitialSignal
// when set, otherwise Approved, so a run suspended at an approval gate advances.
func LoadResume(params LoopParams) (ResumeState, error) {
	pos, exec, err := resolveCheckpoint(params.Checkpoint).Load()
	if err != nil {
		return ResumeState{}, fmt.Errorf("resume: load: %w", err)
	}
	sig := params.InitialSignal
	if sig == "" {
		sig = Approved
	}
	params.InitialState = pos.CurrentState
	params.InitialSignal = sig
	params.InitialResult = Result{Signal: sig, Output: "Resume from checkpoint"}
	params.InitialRun = RunResult{
		Iterations: pos.Snapshot.Iteration,
		TokensIn:   pos.Snapshot.TokensIn,
		TokensOut:  pos.Snapshot.TokensOut,
		TotalCost:  pos.Snapshot.TotalCost,
	}
	params.InitialExecution = exec
	return ResumeState{Params: params, Position: pos, Execution: exec}, nil
}

// Resume loads the persisted snapshot through params.Checkpoint and re-enters the
// loop at the restored position (srd035-checkpoint-port R6.2). It is the
// convenience entrypoint for callers that hold no domain-owned state; callers
// that must restore conversation or other domain state use LoadResume, restore
// from ResumeState.Position, then call Loop.
func Resume(params LoopParams, ctx context.Context) (RunResult, error) {
	state, err := LoadResume(params)
	if err != nil {
		return RunResult{}, err
	}
	return Loop(state.Params, ctx)
}

func historyFromDigest(digest []HistoryDigest) History {
	if len(digest) == 0 {
		return nil
	}
	history := make(History, 0, len(digest))
	for _, entry := range digest {
		history = append(history, HistoryEntry{
			Iteration:    entry.Iteration,
			CommandName:  entry.CommandName,
			FromState:    entry.FromState,
			ToState:      entry.ToState,
			Result:       ResultDigest{Signal: entry.Signal},
			Undo:         cloneUndoMemento(entry.Undo),
			UndoError:    entry.UndoError,
			WorkspaceRef: entry.WorkspaceRef,
		})
	}
	return history
}
