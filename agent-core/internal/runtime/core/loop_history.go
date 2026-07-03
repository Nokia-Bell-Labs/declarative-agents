// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"time"
)

func historyEnabled(p LoopParams) bool {
	return p.CheckpointPolicy != nil
}

// resolveCheckpoint returns the configured Checkpoint port, or NoopCheckpoint
// when none is injected so disabled-mode execution keeps its current behavior
// (srd035-checkpoint-port R5.1).
func resolveCheckpoint(c Checkpoint) Checkpoint {
	if c == nil {
		return NoopCheckpoint{}
	}
	return c
}

// dispatchPosition builds the resumable Position from loop-owned state. The
// conversation is folded into the snapshot by the domain that owns it (core
// cannot import the llm package); loop-owned code leaves it empty.
func dispatchPosition(state State, signal Signal, iteration int, rr *RunResult) Position {
	return Position{
		CurrentState: state,
		LastSignal:   signal,
		Snapshot: AgentSnapshot{
			State:     state,
			Signal:    signal,
			Iteration: iteration,
			TokensIn:  rr.TokensIn,
			TokensOut: rr.TokensOut,
			TotalCost: rr.TotalCost,
		},
	}
}

// dispatchEntry builds the Execution entry for one completed dispatch. Signal is
// the transition signal that selected the command; Receipt is the tool-owned
// opaque receipt carried verbatim from the Result (srd035-checkpoint-port R2.4, R3).
func dispatchEntry(iteration int, fromState, toState State, transitionSignal Signal, res Result) Entry {
	return Entry{
		Iteration:   iteration,
		Timestamp:   time.Now().UTC(),
		CommandName: res.CommandName,
		FromState:   fromState,
		ToState:     toState,
		Signal:      transitionSignal,
		Result:      digestResult(res),
		Receipt:     res.Receipt,
	}
}

func newHistoryEntry(iteration int, cmd Command, res Result, fromState, toState State, signal Signal, workspaceRef string) HistoryEntry {
	return HistoryEntry{
		Iteration:    iteration,
		Timestamp:    time.Now(),
		Command:      cmd,
		CommandName:  res.CommandName,
		FromState:    fromState,
		ToState:      toState,
		Signal:       signal,
		Result:       digestResult(res),
		WorkspaceRef: workspaceRef,
	}
}

func digestResult(res Result) ResultDigest {
	digest := ResultDigest{
		Signal: res.Signal,
		Output: res.Output,
		Cost:   res.Cost,
	}
	if res.Err != nil {
		digest.Error = res.Err.Error()
	}
	return digest
}

func historyDigest(history History) []HistoryDigest {
	if len(history) == 0 {
		return nil
	}
	digest := make([]HistoryDigest, 0, len(history))
	for _, entry := range history {
		digest = append(digest, HistoryDigest{
			Iteration:    entry.Iteration,
			CommandName:  entry.CommandName,
			FromState:    entry.FromState,
			ToState:      entry.ToState,
			Signal:       entry.Result.Signal,
			WorkspaceRef: entry.WorkspaceRef,
		})
	}
	return digest
}
