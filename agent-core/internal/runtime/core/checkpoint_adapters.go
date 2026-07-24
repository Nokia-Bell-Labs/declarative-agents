// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"fmt"
	"sync"
)

// NoopCheckpoint is the default adapter when persistence is disabled. Save is a
// no-op and Load reports ErrNoCheckpoint, so disabled-mode execution keeps its
// current behavior with no persistence overhead (srd035-checkpoint-port R5.1,
// R5.4).
type NoopCheckpoint struct{}

func (NoopCheckpoint) Save(Position, Execution) error { return nil }

func (NoopCheckpoint) Load() (Position, Execution, error) {
	return Position{}, nil, ErrNoCheckpoint
}

var _ Checkpoint = NoopCheckpoint{}

// InMemoryCheckpoint is the reference adapter for tests. It round-trips a
// Position and Execution in process, including the folded conversation and
// per-entry receipts, and is safe for concurrent use
// (srd035-checkpoint-port R5.2).
type InMemoryCheckpoint struct {
	mu        sync.Mutex
	saved     bool
	position  Position
	execution Execution
}

func (c *InMemoryCheckpoint) Save(position Position, execution Execution) error {
	if conversation := position.Snapshot.Conversation; len(conversation) > 0 && !json.Valid(conversation) {
		return fmt.Errorf("in-memory checkpoint save: conversation is not valid JSON")
	}
	sanitized, err := sanitizeExecutionForSave(execution)
	if err != nil {
		return fmt.Errorf("in-memory checkpoint save: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.position = clonePosition(position)
	c.execution = sanitized
	c.saved = true
	return nil
}

func (c *InMemoryCheckpoint) Load() (Position, Execution, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.saved {
		return Position{}, nil, ErrNoCheckpoint
	}
	return clonePosition(c.position), cloneExecution(c.execution), nil
}

var _ Checkpoint = (*InMemoryCheckpoint)(nil)

// clonePosition copies a Position so callers cannot mutate persisted state
// through the shared conversation byte slice.
func clonePosition(p Position) Position {
	if len(p.Snapshot.Conversation) > 0 {
		p.Snapshot.Conversation = append(json.RawMessage(nil), p.Snapshot.Conversation...)
	}
	return p
}

// cloneExecution copies the ordered dispatch log so callers cannot mutate
// persisted entries after Save or Load.
func cloneExecution(e Execution) Execution {
	if e == nil {
		return nil
	}
	out := make(Execution, len(e))
	copy(out, e)
	for i := range out {
		out[i].Result.RedactedPaths = cloneOutputRedactionPaths(out[i].Result.RedactedPaths)
	}
	return out
}

// sanitizeExecutionForSave reapplies typed field removal before an adapter
// retains Execution. It validates into a detached copy, so a failure cannot
// partially replace the adapter's last valid state (srd035 R7.6).
func sanitizeExecutionForSave(execution Execution) (Execution, error) {
	sanitized := cloneExecution(execution)
	for i := range sanitized {
		result, err := sanitizeResultDigestForSave(sanitized[i].Result)
		if err != nil {
			return nil, fmt.Errorf("step %d output redaction: %w", i, err)
		}
		sanitized[i].Result = result
	}
	return sanitized, nil
}

func sanitizeResultDigestForSave(result ResultDigest) (ResultDigest, error) {
	if result.RedactionVersion != OutputRedactionVersion1 {
		return omitResultDigest(result), nil
	}
	switch result.RedactionStatus {
	case OutputRedactionApplied:
		output, paths, status := applyOutputRedaction(
			result.Output,
			result.RedactionVersion,
			result.RedactedPaths,
		)
		if status != OutputRedactionApplied {
			return omitResultDigest(result), nil
		}
		result.Output = output
		result.RedactedPaths = paths
		return result, nil
	case OutputRedactionOmitted:
		if result.Output != "" || len(result.RedactedPaths) != 0 {
			return omitResultDigest(result), nil
		}
		return result, nil
	default:
		return omitResultDigest(result), nil
	}
}
