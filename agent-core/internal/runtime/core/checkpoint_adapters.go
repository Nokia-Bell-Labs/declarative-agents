// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"crypto/sha256"
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

func (NoopCheckpoint) ConversationReference() (string, bool) { return "", false }

func (NoopCheckpoint) ResolveConversationSnapshot(string) (json.RawMessage, error) {
	return nil, ErrConversationReferenceUnavailable
}

var (
	_ Checkpoint                    = NoopCheckpoint{}
	_ ConversationReferenceProvider = NoopCheckpoint{}
	_ ConversationSnapshotResolver  = NoopCheckpoint{}
)

// InMemoryCheckpoint is the reference adapter for tests. It round-trips a
// Position and Execution in process, including the folded conversation and
// per-entry receipts, and is safe for concurrent use
// (srd035-checkpoint-port R5.2).
type InMemoryCheckpoint struct {
	mu            sync.Mutex
	runID         string
	saved         bool
	position      Position
	execution     Execution
	currentRef    string
	conversations map[string]json.RawMessage
}

// NewInMemoryCheckpoint creates a reference adapter with stable run isolation.
// A zero-value InMemoryCheckpoint still supports Save/Load but reports
// conversation references unavailable.
func NewInMemoryCheckpoint(runID string) *InMemoryCheckpoint {
	return &InMemoryCheckpoint{runID: runID}
}

func (c *InMemoryCheckpoint) Save(position Position, execution Execution) error {
	if conversation := position.Snapshot.Conversation; len(conversation) > 0 && !json.Valid(conversation) {
		return fmt.Errorf("in-memory checkpoint save: conversation is not valid JSON")
	}
	if domain := position.Snapshot.Domain; len(domain) > 0 && !json.Valid(domain) {
		return fmt.Errorf("in-memory checkpoint save: domain is not valid JSON")
	}
	sanitized, err := sanitizeExecutionForSave(execution)
	if err != nil {
		return fmt.Errorf("in-memory checkpoint save: %w", err)
	}
	ref, conversation, err := c.conversationReferenceFor(position, execution)
	if err != nil {
		return fmt.Errorf("in-memory checkpoint save: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.position = clonePosition(position)
	c.execution = sanitized
	c.saved = true
	c.currentRef = ref
	if ref != "" {
		if c.conversations == nil {
			c.conversations = make(map[string]json.RawMessage)
		}
		c.conversations[ref] = conversation
	}
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

func (c *InMemoryCheckpoint) ConversationReference() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentRef, c.currentRef != ""
}

func (c *InMemoryCheckpoint) ResolveConversationSnapshot(reference string) (json.RawMessage, error) {
	parsed, err := parseCheckpointReference(reference)
	if err != nil || parsed.backend != "memory" || parsed.runID != c.runID {
		return nil, fmt.Errorf("%w: in-memory checkpoint", ErrConversationReferenceInvalid)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	conversation, ok := c.conversations[reference]
	if !ok {
		return nil, fmt.Errorf("%w: in-memory checkpoint", ErrConversationReferenceUnavailable)
	}
	return append(json.RawMessage(nil), conversation...), nil
}

func (c *InMemoryCheckpoint) conversationReferenceFor(
	position Position,
	execution Execution,
) (string, json.RawMessage, error) {
	if c.runID == "" || len(execution) == 0 || len(position.Snapshot.Conversation) == 0 {
		return "", nil, nil
	}
	digest := sha256.Sum256(position.Snapshot.Conversation)
	ref, err := formatCheckpointReference("memory", c.runID, len(execution)-1, fmt.Sprintf("%x", digest))
	if err != nil {
		return "", nil, err
	}
	conversation := append(json.RawMessage(nil), position.Snapshot.Conversation...)
	return ref, conversation, nil
}

var (
	_ Checkpoint                    = (*InMemoryCheckpoint)(nil)
	_ ConversationReferenceProvider = (*InMemoryCheckpoint)(nil)
	_ ConversationSnapshotResolver  = (*InMemoryCheckpoint)(nil)
)

// clonePosition copies a Position so callers cannot mutate persisted state
// through the shared conversation byte slice.
func clonePosition(p Position) Position {
	if len(p.Snapshot.Conversation) > 0 {
		p.Snapshot.Conversation = append(json.RawMessage(nil), p.Snapshot.Conversation...)
	}
	if len(p.Snapshot.Domain) > 0 {
		p.Snapshot.Domain = append(json.RawMessage(nil), p.Snapshot.Domain...)
	}
	p.Snapshot.Iterator = cloneIteratorSnapshot(p.Snapshot.Iterator)
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
