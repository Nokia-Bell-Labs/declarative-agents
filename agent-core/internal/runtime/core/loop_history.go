// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
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

func newHistoryEntry(iteration int, cmd Command, res Result, fromState, toState State, signal Signal, workspaceRef string, tr tracing.Tracer) HistoryEntry {
	undo, undoErr := captureUndoMemento(cmd, res.CommandName, tr, iteration)
	return HistoryEntry{
		Iteration:    iteration,
		Timestamp:    time.Now(),
		Command:      cmd,
		CommandName:  res.CommandName,
		FromState:    fromState,
		ToState:      toState,
		Signal:       signal,
		Result:       digestResult(res),
		Undo:         undo,
		UndoError:    undoErr,
		WorkspaceRef: workspaceRef,
	}
}

func captureUndoMemento(cmd Command, commandName string, tr tracing.Tracer, iteration int) (*UndoMemento, string) {
	provider, ok := cmd.(UndoMementoProvider)
	if !ok {
		return nil, ""
	}
	memento, err := provider.UndoMemento()
	if err != nil {
		return nil, recordUndoMementoError(tr, iteration, commandName, err)
	}
	if err := ValidateUndoMemento(memento); err != nil {
		return nil, recordUndoMementoError(tr, iteration, commandName, err)
	}
	return cloneUndoMemento(&memento), ""
}

func recordUndoMementoError(tr tracing.Tracer, iteration int, commandName string, err error) string {
	if tr != nil {
		tr.Event("history.undo_memento_invalid",
			attribute.Int("iteration", iteration),
			attribute.String("command", commandName),
			attribute.String("error", err.Error()),
		)
	}
	return err.Error()
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
			Undo:         cloneUndoMemento(entry.Undo),
			UndoError:    entry.UndoError,
			WorkspaceRef: entry.WorkspaceRef,
		})
	}
	return digest
}
