// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"errors"
	"fmt"
)

// Resume error classifications. Resume distinguishes three failure modes so an
// operator or caller can tell a nonexistent checkpoint from persisted state the
// current machine can no longer resume, from a backend that failed to load
// (srd025 R6.5). Missing is reported through the existing ErrNoCheckpoint
// sentinel; the two below cover the other classes.
var (
	// ErrCheckpointIncompatible signals the persisted checkpoint does not match
	// the current machine, for example its restored state no longer exists.
	ErrCheckpointIncompatible = errors.New("resume: checkpoint incompatible with current machine")
	// ErrCheckpointLoadFailed signals the checkpoint backend failed to load the
	// persisted snapshot (as opposed to there being none).
	ErrCheckpointLoadFailed = errors.New("resume: checkpoint load failed")
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
		if errors.Is(err, ErrNoCheckpoint) {
			// Nothing persisted: keep the ErrNoCheckpoint classification so
			// callers can tell "no checkpoint" from a backend load failure.
			return ResumeState{}, fmt.Errorf("resume: %w", err)
		}
		return ResumeState{}, fmt.Errorf("%w: %v", ErrCheckpointLoadFailed, err)
	}
	if err := validateResumeCompatibility(params, pos); err != nil {
		return ResumeState{}, err
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

// validateResumeCompatibility rejects a checkpoint the current machine cannot
// resume before the loop re-enters at a dropped state and dead-ends on an
// unhandled state-signal pair. A checkpoint is compatible when its restored
// state is terminal or is still defined in the current machine (srd025 R6.4,
// R6.5). An empty restored state carries no position to validate.
func validateResumeCompatibility(params LoopParams, pos Position) error {
	if pos.CurrentState == "" {
		return nil
	}
	if params.IsTerminal != nil && params.IsTerminal(pos.CurrentState) {
		return nil
	}
	if params.InitialState == pos.CurrentState || params.Table.HasState(pos.CurrentState) {
		return nil
	}
	return fmt.Errorf("%w: restored state %q is not defined in the current machine", ErrCheckpointIncompatible, pos.CurrentState)
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
