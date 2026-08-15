// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

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

func (NoopCheckpoint) DomainReference() (string, bool) { return "", false }

func (NoopCheckpoint) ResolveDomainSnapshot(string) ([]byte, error) {
	return nil, ErrDomainReferenceUnavailable
}

var (
	_ Checkpoint                    = NoopCheckpoint{}
	_ ConversationReferenceProvider = NoopCheckpoint{}
	_ ConversationSnapshotResolver  = NoopCheckpoint{}
	_ DomainReferenceProvider       = NoopCheckpoint{}
	_ DomainSnapshotResolver        = NoopCheckpoint{}
)

// InMemoryCheckpoint is the reference adapter for tests. It round-trips a
// Position and Execution in process, including the folded conversation and
// per-entry receipts, and is safe for concurrent use
// (srd035-checkpoint-port R5.2).
type InMemoryCheckpoint struct {
	mu               sync.Mutex
	runID            string
	saved            bool
	position         Position
	execution        Execution
	currentRef       string
	currentDomainRef string
	conversations    map[string][]byte
	domains          map[string][]byte
}

type checkpointSnapshotReferences struct {
	conversationRef string
	conversation    []byte
	domainRef       string
	domain          []byte
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
	references, err := c.prepareSnapshotReferences(position, execution)
	if err != nil {
		return fmt.Errorf("in-memory checkpoint save: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.position = clonePosition(position)
	c.execution = sanitized
	c.saved = true
	c.currentRef = references.conversationRef
	c.currentDomainRef = references.domainRef
	if references.conversationRef != "" {
		if c.conversations == nil {
			c.conversations = make(map[string][]byte)
		}
		c.conversations[references.conversationRef] = references.conversation
	}
	if references.domainRef != "" {
		if c.domains == nil {
			c.domains = make(map[string][]byte)
		}
		c.domains[references.domainRef] = references.domain
	}
	return nil
}

func (c *InMemoryCheckpoint) prepareSnapshotReferences(
	position Position,
	execution Execution,
) (checkpointSnapshotReferences, error) {
	conversationRef, conversation, err := c.snapshotReferenceFor(
		position.Snapshot.Conversation,
		execution,
	)
	if err != nil {
		return checkpointSnapshotReferences{}, err
	}
	domainRef, domain, err := c.snapshotReferenceFor(position.Snapshot.Domain, execution)
	if err != nil {
		return checkpointSnapshotReferences{}, err
	}
	return checkpointSnapshotReferences{
		conversationRef: conversationRef,
		conversation:    conversation,
		domainRef:       domainRef,
		domain:          domain,
	}, nil
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
	c.mu.Lock()
	defer c.mu.Unlock()
	snapshot, err := c.resolveSnapshot(
		reference,
		c.conversations,
		ErrConversationReferenceInvalid,
		ErrConversationReferenceUnavailable,
	)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(snapshot), nil
}

func (c *InMemoryCheckpoint) DomainReference() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentDomainRef, c.currentDomainRef != ""
}

func (c *InMemoryCheckpoint) ResolveDomainSnapshot(reference string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resolveSnapshot(
		reference,
		c.domains,
		ErrDomainReferenceInvalid,
		ErrDomainReferenceUnavailable,
	)
}

func (c *InMemoryCheckpoint) snapshotReferenceFor(
	snapshot []byte,
	execution Execution,
) (string, []byte, error) {
	if c.runID == "" || len(execution) == 0 || len(snapshot) == 0 {
		return "", nil, nil
	}
	digest := sha256.Sum256(snapshot)
	ref, err := formatCheckpointReference("memory", c.runID, len(execution)-1, fmt.Sprintf("%x", digest))
	if err != nil {
		return "", nil, err
	}
	return ref, append([]byte(nil), snapshot...), nil
}

func (c *InMemoryCheckpoint) resolveSnapshot(
	reference string,
	snapshots map[string][]byte,
	invalid, unavailable error,
) ([]byte, error) {
	parsed, err := parseCheckpointReference(reference)
	if err != nil || parsed.backend != "memory" || parsed.runID != c.runID {
		return nil, fmt.Errorf("%w: in-memory checkpoint", invalid)
	}
	if snapshot, ok := snapshots[reference]; ok {
		return append([]byte(nil), snapshot...), nil
	}
	for knownReference := range snapshots {
		known, knownErr := parseCheckpointReference(knownReference)
		if knownErr == nil && (known.step == parsed.step || known.revision == parsed.revision) {
			return nil, fmt.Errorf("%w: in-memory checkpoint", invalid)
		}
	}
	return nil, fmt.Errorf("%w: in-memory checkpoint", unavailable)
}

var (
	_ Checkpoint                    = (*InMemoryCheckpoint)(nil)
	_ ConversationReferenceProvider = (*InMemoryCheckpoint)(nil)
	_ ConversationSnapshotResolver  = (*InMemoryCheckpoint)(nil)
	_ DomainReferenceProvider       = (*InMemoryCheckpoint)(nil)
	_ DomainSnapshotResolver        = (*InMemoryCheckpoint)(nil)
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
