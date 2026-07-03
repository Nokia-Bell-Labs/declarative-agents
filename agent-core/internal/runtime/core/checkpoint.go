// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

// CheckpointReverter is the port the rollback lifecycle tool depends on: the
// two-method Checkpoint plus a git-style Revert that resets a run's persisted
// DB state to a prior step. External effects (files, resources) are reversed
// separately by the lifecycle receipt walk, never here
// (srd036-dolt-state-persistence R6). *DoltCheckpoint satisfies it; tests
// supply fakes.
type CheckpointReverter interface {
	Checkpoint
	Revert(runID string, stepIndex int) error
}

// FormatExecutionHistory renders the resumable Position and the ordered
// Execution log as a human-readable digest for the checkpoint_history tool. Each
// step shows its step index (the Revert target), iteration, command, transition,
// and signal; entries that carry an opaque receipt are marked reversible. The
// receipt itself is never parsed or printed (srd035-checkpoint-port R3).
func FormatExecutionHistory(pos Position, exec Execution) string {
	var b strings.Builder
	fmt.Fprintf(&b, "state: %s\n", pos.CurrentState)
	fmt.Fprintf(&b, "iteration: %d\n", pos.Snapshot.Iteration)
	fmt.Fprintf(&b, "last_signal: %s\n", pos.LastSignal)
	if len(exec) == 0 {
		b.WriteString("history: <empty>\n")
		return b.String()
	}
	b.WriteString("history:\n")
	for step, e := range exec {
		fmt.Fprintf(&b, "  step=%d  iteration=%d  %s  %s -> %s  signal=%s",
			step, e.Iteration, e.CommandName, e.FromState, e.ToState, e.Signal)
		if e.Receipt != "" {
			b.WriteString("  reversible")
		}
		b.WriteByte('\n')
	}
	return b.String()
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

// ResultDigest is the serializable portion of a command result retained in
// each Execution Entry.
type ResultDigest struct {
	Signal Signal `json:"signal"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
	Cost   Cost   `json:"cost"`
}
