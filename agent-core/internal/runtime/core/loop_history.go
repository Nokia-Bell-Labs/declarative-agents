// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
)

func historyEnabled(p LoopParams) bool {
	return p.CheckpointPolicy != nil
}

func captureWorkspaceRef(p LoopParams, tr tracing.Tracer, ctx context.Context, iteration int, commandName string) string {
	if p.Workspace == nil {
		return ""
	}
	ref, err := p.Workspace.CurrentRef(ctx)
	if err != nil {
		tr.Event("history.workspace_ref_failed",
			attribute.Int("iteration", iteration),
			attribute.String("command", commandName),
			attribute.String("error", err.Error()),
		)
		return ""
	}
	return ref
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

func persistSuspendCheckpoint(ctx context.Context, p LoopParams, tr tracing.Tracer, rr *RunResult, state State, sig Signal, iteration int) error {
	if p.StateStore == nil {
		tr.Event("suspend.checkpoint_skipped",
			attribute.String("reason", "state_store_not_configured"),
			attribute.Int("iteration", iteration),
		)
		return nil
	}
	workspaceRef := latestWorkspaceRef(rr.History)
	if workspaceRef == "" && p.Workspace != nil {
		workspaceRef = captureWorkspaceRef(p, tr, ctx, iteration, "suspend")
	}
	cp := suspendCheckpoint(p, rr, state, sig, iteration, workspaceRef)
	if err := validateCheckpointHistory(cp.History); err != nil {
		return err
	}
	if err := addCheckpointSnapshots(&cp, p.Hooks); err != nil {
		return err
	}
	return saveSuspendCheckpoint(ctx, p, tr, cp, iteration)
}

func suspendCheckpoint(p LoopParams, rr *RunResult, state State, sig Signal, iteration int, workspaceRef string) Checkpoint {
	return Checkpoint{
		ID:        fmt.Sprintf("suspend-%d-%d", iteration, time.Now().UTC().UnixNano()),
		Iteration: iteration,
		Timestamp: time.Now().UTC(),
		AgentState: AgentSnapshot{
			State:     state,
			Signal:    sig,
			Iteration: iteration,
			TokensIn:  rr.TokensIn,
			TokensOut: rr.TokensOut,
			TotalCost: rr.TotalCost,
		},
		WorkspaceRef: workspaceRef,
		History:      historyDigest(rr.History),
	}
}

func addCheckpointSnapshots(cp *Checkpoint, hooks LoopHooks) error {
	if hooks.SnapshotConversation != nil {
		conversationLog, err := hooks.SnapshotConversation()
		if err != nil {
			return fmt.Errorf("suspend checkpoint conversation snapshot: %w", err)
		}
		cp.ConversationLog = conversationLog
	}
	if hooks.SnapshotDomain != nil {
		domainState, err := hooks.SnapshotDomain()
		if err != nil {
			return fmt.Errorf("suspend checkpoint domain snapshot: %w", err)
		}
		cp.DomainState = domainState
	}
	return nil
}

func saveSuspendCheckpoint(ctx context.Context, p LoopParams, tr tracing.Tracer, cp Checkpoint, iteration int) error {
	data, err := json.Marshal(cp)
	if err != nil {
		return fmt.Errorf("suspend checkpoint marshal: %w", err)
	}
	key := "checkpoint/" + cp.ID
	if err := p.StateStore.Save(ctx, key, data); err != nil {
		return fmt.Errorf("suspend checkpoint save %s: %w", key, err)
	}
	tr.Event("suspend.checkpoint_saved",
		attribute.String("checkpoint_id", cp.ID),
		attribute.String("checkpoint_key", key),
		attribute.Int("iteration", iteration),
	)
	return nil
}

func latestWorkspaceRef(history History) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].WorkspaceRef != "" {
			return history[i].WorkspaceRef
		}
	}
	return ""
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

func validateCheckpointHistory(history []HistoryDigest) error {
	for _, entry := range history {
		if entry.UndoError != "" {
			return fmt.Errorf("%w: command %s at iteration %d: %s", ErrUndoMementoIncompatible, entry.CommandName, entry.Iteration, entry.UndoError)
		}
		if entry.Undo == nil {
			continue
		}
		if err := ValidateUndoMemento(*entry.Undo); err != nil {
			return fmt.Errorf("checkpoint history undo memento for %s at iteration %d: %w", entry.CommandName, entry.Iteration, err)
		}
	}
	return nil
}
