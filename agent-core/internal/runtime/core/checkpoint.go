// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"errors"
	"time"
)

// ErrNoCheckpoint is the typed not-found result Load returns when nothing has
// been persisted (srd035-checkpoint-port R1.3).
var ErrNoCheckpoint = errors.New("no checkpoint persisted")

// Checkpoint is the typed persistence port (srd035-checkpoint-port). It exposes
// exactly two methods: Save records the resumable Position and the ordered
// Execution log as one unit, and Load restores the most recently saved pair or
// reports ErrNoCheckpoint. Adapters own serialization and storage; the engine
// depends only on this interface.
type Checkpoint interface {
	Save(position Position, execution Execution) error
	Load() (Position, Execution, error)
}

// Position identifies the resumable machine position and carries the loop-owned
// counters through an embedded AgentSnapshot (srd035-checkpoint-port R2.1, R2.2).
type Position struct {
	CurrentState State         `json:"current_state"`
	LastSignal   Signal        `json:"last_signal"`
	Snapshot     AgentSnapshot `json:"snapshot"`
}

// Execution is the ordered dispatch log preserved for forward inspection and
// reverse traversal (srd035-checkpoint-port R2.3). It serializes as a JSON
// array, or null when empty.
type Execution []Entry

// Entry records one completed dispatch. Receipt is an opaque, tool-owned string
// persisted verbatim; the engine and every adapter treat it as opaque and never
// parse it (srd035-checkpoint-port R2.4, R3).
type Entry struct {
	Iteration   int          `json:"iteration"`
	Timestamp   time.Time    `json:"timestamp"`
	CommandName string       `json:"command_name"`
	FromState   State        `json:"from_state"`
	ToState     State        `json:"to_state"`
	Signal      Signal       `json:"signal"`
	Result      ResultDigest `json:"result"`
	Receipt     string       `json:"receipt,omitempty"`
}

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

// CheckpointRecord captures the three rollback layers:
//
//   - agent state: loop position and accumulated budget/cost counters
//   - command/domain state: optional JSON owned by commands or domains
//   - environment state: an opaque Workspace ref such as a git commit
type CheckpointRecord struct {
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
// Conversation folds the model context into the typed snapshot so a resumed run
// re-enters the loop with its prior conversation, replacing the former separate
// conversation snapshot hook and store path (srd035-checkpoint-port R4).
type AgentSnapshot struct {
	State        State           `json:"state"`
	Signal       Signal          `json:"signal"`
	Iteration    int             `json:"iteration"`
	TokensIn     int             `json:"tokens_in"`
	TokensOut    int             `json:"tokens_out"`
	TotalCost    float64         `json:"total_cost"`
	Conversation json.RawMessage `json:"conversation,omitempty"`
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
