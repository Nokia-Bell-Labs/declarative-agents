// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ResolveLatestCheckpointID scans the store for all checkpoint/ keys and
// returns the ID of the most recent one by timestamp. If requested is a
// non-empty string other than "latest", it is returned as-is.
func ResolveLatestCheckpointID(ctx context.Context, store StateStore, requested string) (string, error) {
	if requested != "" && requested != "latest" {
		return requested, nil
	}
	keys, err := store.List(ctx, "checkpoint/")
	if err != nil {
		return "", fmt.Errorf("list checkpoints: %w", err)
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("no checkpoints found")
	}
	sort.Strings(keys)
	var latest Checkpoint
	var latestID string
	for _, key := range keys {
		id := strings.TrimPrefix(key, "checkpoint/")
		cp, err := LoadCheckpoint(ctx, store, id)
		if err != nil {
			continue
		}
		if latestID == "" || cp.Timestamp.After(latest.Timestamp) || (cp.Timestamp.Equal(latest.Timestamp) && id > latestID) {
			latest = cp
			latestID = id
		}
	}
	if latestID == "" {
		return "", fmt.Errorf("no readable checkpoints found")
	}
	return latestID, nil
}

// FormatCheckpointHistory renders a checkpoint as a human-readable digest
// showing the checkpoint metadata, current state, and history entries.
func FormatCheckpointHistory(cp Checkpoint) string {
	var b strings.Builder
	fmt.Fprintf(&b, "checkpoint: %s\n", cp.ID)
	fmt.Fprintf(&b, "iteration: %d\n", cp.Iteration)
	fmt.Fprintf(&b, "state: %s\n", cp.AgentState.State)
	if cp.WorkspaceRef != "" {
		fmt.Fprintf(&b, "workspace_ref: %s\n", cp.WorkspaceRef)
	}
	if len(cp.History) == 0 {
		b.WriteString("history: <empty>\n")
		return b.String()
	}
	b.WriteString("history:\n")
	for _, entry := range cp.History {
		fmt.Fprintf(&b, "  %d  %s  %s -> %s  signal=%s", entry.Iteration, entry.CommandName, entry.FromState, entry.ToState, entry.Signal)
		if entry.WorkspaceRef != "" {
			fmt.Fprintf(&b, "  workspace=%s", entry.WorkspaceRef)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// RollbackCheckpointResult contains the rewritten checkpoint and the target
// workspace ref (if any) so the caller can restore the workspace.
type RollbackCheckpointResult struct {
	Checkpoint   Checkpoint
	WorkspaceRef string
}

// RollbackCheckpoint rewrites a persisted checkpoint to the target iteration
// by walking history backward through each entry's undo memento. It produces
// a new Checkpoint that can be persisted. Workspace restore is NOT performed
// here — the caller decides how to handle the returned WorkspaceRef.
func RollbackCheckpoint(cp Checkpoint, targetIteration int) (RollbackCheckpointResult, error) {
	if targetIteration < 0 {
		return RollbackCheckpointResult{}, fmt.Errorf("target iteration must be >= 0, got %d", targetIteration)
	}
	restorer := &persistedRollbackRestorer{}
	rollbackResult, err := RollbackTo(RollbackOptions{
		History:         checkpointHistoryWithPersistedCommands(cp.History, restorer),
		TargetIteration: targetIteration,
	})
	if err != nil {
		return RollbackCheckpointResult{}, fmt.Errorf("rollback command restore: %w", err)
	}

	out := cp
	out.ID = fmt.Sprintf("rollback-%s-to-%d-%d", cp.ID, targetIteration, time.Now().UTC().UnixNano())
	out.Timestamp = time.Now().UTC()
	out.Iteration = targetIteration
	out.AgentState.Iteration = targetIteration
	targetState := rollbackResult.State
	targetRef := rollbackResult.WorkspaceRef
	switch {
	case targetIteration == 0:
		out.History = nil
	case targetIteration == cp.AgentState.Iteration && len(cp.History) == 0:
		targetState = cp.AgentState.State
		targetRef = cp.WorkspaceRef
	default:
		found := false
		history := make([]HistoryDigest, 0, len(cp.History))
		for _, entry := range cp.History {
			if entry.Iteration <= targetIteration {
				history = append(history, entry)
			}
			if entry.Iteration == targetIteration {
				found = true
			}
		}
		if !found {
			return RollbackCheckpointResult{}, fmt.Errorf("target iteration %d not found in checkpoint %s", targetIteration, cp.ID)
		}
		out.History = history
	}
	if targetState == "" {
		targetState = cp.AgentState.State
	}
	out.AgentState.State = targetState
	out.AgentState.Signal = Approved
	out.WorkspaceRef = targetRef
	if restorer.conversationLog != nil {
		out.ConversationLog = restorer.conversationLog
	}
	if restorer.domainState != nil {
		out.DomainState = restorer.domainState
	}
	return RollbackCheckpointResult{Checkpoint: out, WorkspaceRef: targetRef}, nil
}

// RollbackFromCheckpointOptions describes a full checkpoint rollback: resolve
// the checkpoint, rewrite it, optionally restore the workspace, and persist
// the new checkpoint. This is the rollback peer to ResumeFromCheckpoint.
type RollbackFromCheckpointOptions struct {
	Store           StateStore
	Workspace       Workspace
	CheckpointID    string
	TargetIteration int
	Ctx             context.Context
}

// RollbackFromCheckpointResult contains the original and rewritten checkpoints.
type RollbackFromCheckpointResult struct {
	Original Checkpoint
	RollbackCheckpointResult
}

// RollbackFromCheckpoint loads a checkpoint, rolls it back to the target
// iteration, optionally restores the workspace, and persists the new
// checkpoint. It is the rollback counterpart to ResumeFromCheckpoint.
func RollbackFromCheckpoint(opts RollbackFromCheckpointOptions) (RollbackFromCheckpointResult, error) {
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Store == nil {
		return RollbackFromCheckpointResult{}, fmt.Errorf("%w: StateStore is required", ErrCheckpointMissing)
	}

	checkpointID, err := ResolveLatestCheckpointID(ctx, opts.Store, opts.CheckpointID)
	if err != nil {
		return RollbackFromCheckpointResult{}, err
	}
	cp, err := LoadCheckpoint(ctx, opts.Store, checkpointID)
	if err != nil {
		return RollbackFromCheckpointResult{}, err
	}

	rbResult, err := RollbackCheckpoint(cp, opts.TargetIteration)
	if err != nil {
		return RollbackFromCheckpointResult{Original: cp}, err
	}

	if rbResult.WorkspaceRef != "" && opts.Workspace != nil {
		if err := opts.Workspace.Restore(ctx, rbResult.WorkspaceRef); err != nil {
			return RollbackFromCheckpointResult{Original: cp, RollbackCheckpointResult: rbResult},
				fmt.Errorf("rollback restore workspace to %s: %w", rbResult.WorkspaceRef, err)
		}
	}

	data, err := json.Marshal(rbResult.Checkpoint)
	if err != nil {
		return RollbackFromCheckpointResult{Original: cp, RollbackCheckpointResult: rbResult},
			fmt.Errorf("rollback checkpoint marshal: %w", err)
	}
	key := "checkpoint/" + rbResult.Checkpoint.ID
	if err := opts.Store.Save(ctx, key, data); err != nil {
		return RollbackFromCheckpointResult{Original: cp, RollbackCheckpointResult: rbResult},
			fmt.Errorf("rollback checkpoint save %s: %w", key, err)
	}

	return RollbackFromCheckpointResult{Original: cp, RollbackCheckpointResult: rbResult}, nil
}

func checkpointHistoryWithPersistedCommands(digest []HistoryDigest, restorer *persistedRollbackRestorer) History {
	if len(digest) == 0 {
		return nil
	}
	history := make(History, 0, len(digest))
	for _, entry := range digest {
		history = append(history, HistoryEntry{
			Iteration:    entry.Iteration,
			CommandName:  entry.CommandName,
			Command:      persistedHistoryCommand{entry: entry, restorer: restorer},
			FromState:    entry.FromState,
			ToState:      entry.ToState,
			Result:       ResultDigest{Signal: entry.Signal},
			Undo:         entry.Undo,
			UndoError:    entry.UndoError,
			WorkspaceRef: entry.WorkspaceRef,
		})
	}
	return history
}

type persistedHistoryCommand struct {
	entry    HistoryDigest
	restorer *persistedRollbackRestorer
}

func (p persistedHistoryCommand) Name() string {
	return p.entry.CommandName
}

func (p persistedHistoryCommand) Execute() Result {
	return Result{Signal: ToolDone, CommandName: p.Name()}
}

func (p persistedHistoryCommand) Undo() Result {
	if err := p.restorer.Restore(p.entry); err != nil {
		return Result{
			Signal:      CommandError,
			CommandName: p.Name(),
			Output:      err.Error(),
			Err:         err,
		}
	}
	return NoopUndo(p.Name())
}

type persistedRollbackRestorer struct {
	conversationLog json.RawMessage
	domainState     json.RawMessage
}

type persistedUndoPayload struct {
	ConversationLog json.RawMessage `json:"conversation,omitempty"`
	DomainState     json.RawMessage `json:"domain_state,omitempty"`
}

func (p *persistedRollbackRestorer) Restore(entry HistoryDigest) error {
	if entry.UndoError != "" {
		return fmt.Errorf("%w: %s", ErrUndoMementoIncompatible, entry.UndoError)
	}
	if entry.Undo == nil {
		return fmt.Errorf("%w: command %s at iteration %d", ErrUndoMementoMissing, entry.CommandName, entry.Iteration)
	}
	if err := ValidateUndoMemento(*entry.Undo); err != nil {
		return err
	}
	switch entry.Undo.Kind {
	case UndoMementoNoop:
		return nil
	case UndoMementoIrreversible:
		return fmt.Errorf("%w: command %s is irreversible: %s", ErrUndoMementoIncompatible, entry.CommandName, entry.Undo.Description)
	case UndoMementoReversible, UndoMementoCompensatable:
		return p.restorePayload(entry)
	default:
		return fmt.Errorf("%w: unsupported undo kind %s", ErrUndoMementoIncompatible, entry.Undo.Kind)
	}
}

func (p *persistedRollbackRestorer) restorePayload(entry HistoryDigest) error {
	var payload persistedUndoPayload
	if err := json.Unmarshal(entry.Undo.Payload, &payload); err != nil {
		return fmt.Errorf("%w: decode payload for %s: %v", ErrUndoMementoIncompatible, entry.CommandName, err)
	}
	if len(payload.DomainState) > 0 {
		p.domainState = append(json.RawMessage(nil), payload.DomainState...)
	}
	if len(payload.ConversationLog) > 0 {
		p.conversationLog = append(json.RawMessage(nil), payload.ConversationLog...)
	}
	return nil
}
