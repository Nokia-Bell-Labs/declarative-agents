// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"time"
)

// CheckpointPhase identifies where in the loop lifecycle a checkpoint decision
// is being considered.
type CheckpointPhase string

const (
	CheckpointBeforeDispatch CheckpointPhase = "before_dispatch"
	CheckpointAfterDispatch  CheckpointPhase = "after_dispatch"
	CheckpointSuspend        CheckpointPhase = "suspend"
	CheckpointManual         CheckpointPhase = "manual"
)

// CheckpointEvent describes the loop context supplied to a CheckpointPolicy.
// It is behavior-neutral until later history/checkpoint wiring consumes it.
type CheckpointEvent struct {
	Iteration   int
	Phase       CheckpointPhase
	State       State
	Signal      Signal
	CommandName string
}

// CheckpointPolicy decides whether the runtime should capture state for a
// given loop event. A nil policy disables automatic checkpointing.
type CheckpointPolicy interface {
	ShouldCheckpoint(event CheckpointEvent) bool
}

// Checkpoint captures the three rollback layers:
//
//   - agent state: loop position and accumulated budget/cost counters
//   - command/domain state: optional JSON owned by commands or domains
//   - environment state: an opaque Workspace ref such as a git commit
type Checkpoint struct {
	ID              string          `json:"id"`
	Iteration       int             `json:"iteration"`
	Timestamp       time.Time       `json:"timestamp"`
	AgentState      AgentSnapshot   `json:"agent_state"`
	ConversationLog json.RawMessage `json:"conversation,omitempty"`
	DomainState     json.RawMessage `json:"domain_state,omitempty"`
	WorkspaceRef    string          `json:"workspace_ref,omitempty"`
	History         []HistoryDigest `json:"history,omitempty"`
}

// AgentSnapshot is the serializable loop-owned portion of a checkpoint.
type AgentSnapshot struct {
	State     State   `json:"state"`
	Signal    Signal  `json:"signal"`
	Iteration int     `json:"iteration"`
	TokensIn  int     `json:"tokens_in"`
	TokensOut int     `json:"tokens_out"`
	TotalCost float64 `json:"total_cost"`
}

// HistoryDigest is a compact serializable record of one completed loop step.
// Full command undo data remains command-owned; this digest is for inspection,
// resume summaries, and future rollback traversal.
type HistoryDigest struct {
	Iteration    int          `json:"iteration"`
	CommandName  string       `json:"command_name"`
	FromState    State        `json:"from_state"`
	ToState      State        `json:"to_state"`
	Signal       Signal       `json:"signal"`
	Undo         *UndoMemento `json:"undo,omitempty"`
	UndoError    string       `json:"undo_error,omitempty"`
	WorkspaceRef string       `json:"workspace_ref,omitempty"`
}

// History is the rollback-oriented record of dispatched commands in a loop run.
// Unlike RunEvent, it is opt-in and keeps the command object out of JSON while
// preserving it in memory for future rollback traversal.
type History []HistoryEntry

// HistoryEntry records one completed command dispatch for replay summaries and
// rollback traversal. Signal is the transition signal that selected the command;
// Result contains the command's outcome.
type HistoryEntry struct {
	Iteration    int          `json:"iteration"`
	Timestamp    time.Time    `json:"timestamp"`
	Command      Command      `json:"-"`
	CommandName  string       `json:"command_name"`
	FromState    State        `json:"from_state"`
	ToState      State        `json:"to_state"`
	Signal       Signal       `json:"signal"`
	Result       ResultDigest `json:"result"`
	Undo         *UndoMemento `json:"undo,omitempty"`
	UndoError    string       `json:"undo_error,omitempty"`
	WorkspaceRef string       `json:"workspace_ref,omitempty"`
}

// ResultDigest is the serializable portion of a command result retained in
// HistoryEntry.
type ResultDigest struct {
	Signal Signal `json:"signal"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
	Cost   Cost   `json:"cost"`
}
