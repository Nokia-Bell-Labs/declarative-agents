// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	// ErrConversationReferenceUnavailable reports that persistence cannot
	// provide an authoritative conversation checkpoint reference.
	ErrConversationReferenceUnavailable = errors.New("conversation checkpoint reference unavailable")
	// ErrConversationReferenceInvalid reports a malformed, cross-run, or
	// backend-incompatible conversation checkpoint reference.
	ErrConversationReferenceInvalid = errors.New("invalid conversation checkpoint reference")
	// ErrDomainReferenceUnavailable reports that persistence cannot provide
	// the authoritative opaque domain snapshot named by a checkpoint reference.
	ErrDomainReferenceUnavailable = errors.New("domain checkpoint reference unavailable")
	// ErrDomainReferenceInvalid reports a malformed, cross-run, or
	// backend-incompatible domain checkpoint reference.
	ErrDomainReferenceInvalid = errors.New("invalid domain checkpoint reference")
)

const (
	maxCheckpointReferenceLength = 512
	maxReferencePartLength       = 255
)

// ConversationReferenceProvider is an optional capability beside Checkpoint.
// It reports the latest completed dispatch whose AgentSnapshot.Conversation is
// authoritative. Checkpoint remains the two-method Save/Load port.
type ConversationReferenceProvider interface {
	ConversationReference() (string, bool)
}

// ConversationSnapshotResolver is an optional capability beside Checkpoint.
// It resolves an opaque reference to the authoritative conversation JSON.
type ConversationSnapshotResolver interface {
	ResolveConversationSnapshot(reference string) (json.RawMessage, error)
}

// DomainReferenceProvider is an optional capability beside Checkpoint. It
// reports the latest completed dispatch whose opaque AgentSnapshot.Domain bytes
// are authoritative without coupling core to a domain-owned schema.
type DomainReferenceProvider interface {
	DomainReference() (string, bool)
}

// DomainSnapshotResolver resolves an opaque checkpoint reference to the exact
// AgentSnapshot.Domain bytes stored at that immutable run/step/revision.
type DomainSnapshotResolver interface {
	ResolveDomainSnapshot(reference string) ([]byte, error)
}

// CheckpointReference is the parsed form of a checkpoint:v1 wire reference.
// External checkpoint backends format and parse this grammar; the backend-name
// switch and revision shape stay in core because they are the serialized-format
// contract, not adapter code (srd035-checkpoint-port G8).
type CheckpointReference struct {
	Backend  string
	RunID    string
	Step     int
	Revision string
}

// FormatCheckpointReference encodes a backend, run, step, and revision into the
// checkpoint:v1 wire form. Invalid parts or an oversized encoding return
// ErrConversationReferenceInvalid.
func FormatCheckpointReference(backend, runID string, step int, revision string) (string, error) {
	if !validReferenceBackend(backend) || !ValidReferencePart(runID) ||
		step < 0 || !ValidReferenceRevision(backend, revision) {
		return "", ErrConversationReferenceInvalid
	}
	encode := base64.RawURLEncoding.EncodeToString
	reference := fmt.Sprintf(
		"checkpoint:v1:%s:%s:%d:%s",
		backend, encode([]byte(runID)), step, encode([]byte(revision)),
	)
	if len(reference) > maxCheckpointReferenceLength {
		return "", ErrConversationReferenceInvalid
	}
	return reference, nil
}

// ParseCheckpointReference decodes a checkpoint:v1 wire reference. Malformed,
// oversized, or grammar-violating input returns ErrConversationReferenceInvalid.
func ParseCheckpointReference(reference string) (CheckpointReference, error) {
	if len(reference) == 0 || len(reference) > maxCheckpointReferenceLength {
		return CheckpointReference{}, ErrConversationReferenceInvalid
	}
	parts := strings.Split(reference, ":")
	if len(parts) != 6 || parts[0] != "checkpoint" || parts[1] != "v1" {
		return CheckpointReference{}, ErrConversationReferenceInvalid
	}
	runID, err := decodeReferencePart(parts[3])
	if err != nil {
		return CheckpointReference{}, err
	}
	step, err := strconv.Atoi(parts[4])
	if err != nil || step < 0 || strconv.Itoa(step) != parts[4] {
		return CheckpointReference{}, ErrConversationReferenceInvalid
	}
	revision, err := decodeReferencePart(parts[5])
	if err != nil || !validReferenceBackend(parts[2]) ||
		!ValidReferenceRevision(parts[2], revision) {
		return CheckpointReference{}, ErrConversationReferenceInvalid
	}
	return CheckpointReference{
		Backend: parts[2], RunID: runID, Step: step, Revision: revision,
	}, nil
}

func decodeReferencePart(encoded string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != encoded {
		return "", ErrConversationReferenceInvalid
	}
	value := string(decoded)
	if !ValidReferencePart(value) {
		return "", ErrConversationReferenceInvalid
	}
	return value, nil
}

func validReferenceBackend(backend string) bool {
	return backend == "dolt" || backend == "memory"
}

// ValidReferenceRevision reports whether revision matches the serialized
// grammar for backend: 32-character lowercase base32 (digits plus a-v) for
// dolt, 64-character lowercase hex for memory.
func ValidReferenceRevision(backend, revision string) bool {
	switch backend {
	case "dolt":
		return validLowerAlphaNumeric(revision, 32, 'v')
	case "memory":
		return validLowerAlphaNumeric(revision, 64, 'f')
	default:
		return false
	}
}

func validLowerAlphaNumeric(value string, length int, maxLetter byte) bool {
	if len(value) != length {
		return false
	}
	for i := range len(value) {
		if (value[i] < '0' || value[i] > '9') &&
			(value[i] < 'a' || value[i] > maxLetter) {
			return false
		}
	}
	return true
}

// ValidReferencePart reports whether value is a non-empty, trimmed, UTF-8 run
// or revision identity with no control characters and at most 255 bytes.
func ValidReferencePart(value string) bool {
	if value == "" || len(value) > maxReferencePartLength ||
		!utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
