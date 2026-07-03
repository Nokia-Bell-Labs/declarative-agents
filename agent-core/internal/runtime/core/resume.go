// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrCheckpointMissing      = errors.New("checkpoint missing")
	ErrCheckpointIncompatible = errors.New("checkpoint incompatible")
	ErrResumeRestore          = errors.New("checkpoint restore failed")
)

// ResumeOptions describes how to restore a checkpoint and re-enter Loop.
type ResumeOptions struct {
	Store               StateStore
	Workspace           Workspace
	CheckpointID        string
	Params              LoopParams
	ResumeSignal        Signal
	RestoreConversation func(json.RawMessage) error
	RestoreDomain       func(json.RawMessage) error
	ValidateCheckpoint  func(CheckpointRecord, LoopParams) error
	Ctx                 context.Context
}

// ResumeResult contains the loaded checkpoint and the resumed run result.
type ResumeResult struct {
	Checkpoint CheckpointRecord
	Run        RunResult
}

// ResumeFromCheckpoint loads a checkpoint, restores state layers, and resumes
// execution through the normal Loop path so machine/tool validation still runs.
func ResumeFromCheckpoint(opts ResumeOptions) (ResumeResult, error) {
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Store == nil {
		return ResumeResult{}, fmt.Errorf("%w: StateStore is required", ErrCheckpointMissing)
	}
	if opts.CheckpointID == "" {
		return ResumeResult{}, fmt.Errorf("%w: checkpoint id is required", ErrCheckpointMissing)
	}

	cp, err := LoadCheckpoint(ctx, opts.Store, opts.CheckpointID)
	if err != nil {
		return ResumeResult{}, err
	}
	if opts.ValidateCheckpoint != nil {
		if err := opts.ValidateCheckpoint(cp, opts.Params); err != nil {
			return ResumeResult{Checkpoint: cp}, fmt.Errorf("%w: %v", ErrCheckpointIncompatible, err)
		}
	}
	if opts.Workspace != nil && cp.WorkspaceRef != "" {
		if err := opts.Workspace.Restore(ctx, cp.WorkspaceRef); err != nil {
			return ResumeResult{Checkpoint: cp}, fmt.Errorf("%w: workspace %s: %v", ErrResumeRestore, cp.WorkspaceRef, err)
		}
	}
	if len(cp.ConversationLog) > 0 {
		if opts.RestoreConversation == nil {
			return ResumeResult{Checkpoint: cp}, fmt.Errorf("%w: conversation restore hook is required", ErrResumeRestore)
		}
		if err := opts.RestoreConversation(cp.ConversationLog); err != nil {
			return ResumeResult{Checkpoint: cp}, fmt.Errorf("%w: conversation: %v", ErrResumeRestore, err)
		}
	}
	if len(cp.DomainState) > 0 && opts.RestoreDomain != nil {
		if err := opts.RestoreDomain(cp.DomainState); err != nil {
			return ResumeResult{Checkpoint: cp}, fmt.Errorf("%w: domain: %v", ErrResumeRestore, err)
		}
	}

	params := opts.Params
	params.InitialState = cp.AgentState.State
	resumeSignal := opts.ResumeSignal
	if resumeSignal == "" {
		resumeSignal = Approved
	}
	params.InitialSignal = resumeSignal
	params.InitialResult = Result{Signal: resumeSignal, Output: "Resume from checkpoint " + cp.ID}
	params.InitialRun = RunResult{
		Iterations: cp.AgentState.Iteration,
		TokensIn:   cp.AgentState.TokensIn,
		TokensOut:  cp.AgentState.TokensOut,
		TotalCost:  cp.AgentState.TotalCost,
		History:    historyFromDigest(cp.History),
	}
	params.StateStore = opts.Store
	params.Workspace = opts.Workspace

	rr, err := Loop(params, ctx)
	return ResumeResult{Checkpoint: cp, Run: rr}, err
}

// LoadCheckpoint loads checkpoint/<id> from store and decodes it.
func LoadCheckpoint(ctx context.Context, store StateStore, id string) (CheckpointRecord, error) {
	key := "checkpoint/" + id
	data, err := store.Load(ctx, key)
	if err != nil {
		return CheckpointRecord{}, fmt.Errorf("%w: load %s: %v", ErrCheckpointMissing, key, err)
	}
	var cp CheckpointRecord
	if err := json.Unmarshal(data, &cp); err != nil {
		return CheckpointRecord{}, fmt.Errorf("%w: decode %s: %v", ErrCheckpointIncompatible, key, err)
	}
	if cp.ID == "" {
		return CheckpointRecord{}, fmt.Errorf("%w: decode %s: missing checkpoint id", ErrCheckpointIncompatible, key)
	}
	return cp, nil
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
