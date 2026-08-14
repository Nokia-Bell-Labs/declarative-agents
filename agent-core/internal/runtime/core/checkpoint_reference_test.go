// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoopCheckpointReportsConversationReferencesUnavailable(t *testing.T) {
	t.Parallel()
	checkpoint := NoopCheckpoint{}
	ref, ok := checkpoint.ConversationReference()
	require.False(t, ok)
	require.Empty(t, ref)
	_, err := checkpoint.ResolveConversationSnapshot("checkpoint:v1")
	require.ErrorIs(t, err, ErrConversationReferenceUnavailable)
}

func TestInMemoryCheckpointResolvesAuthoritativeConversationReference(t *testing.T) {
	t.Parallel()
	checkpoint := NewInMemoryCheckpoint("run-memory")
	position := samplePosition()
	require.NoError(t, checkpoint.Save(position, sampleExecution()[:1]))

	ref, ok := checkpoint.ConversationReference()
	require.True(t, ok)
	resolved, err := checkpoint.ResolveConversationSnapshot(ref)
	require.NoError(t, err)
	require.JSONEq(t, string(position.Snapshot.Conversation), string(resolved))

	resolved[0] = '!'
	again, err := checkpoint.ResolveConversationSnapshot(ref)
	require.NoError(t, err)
	require.Equal(t, byte('['), again[0], "resolved caller memory is isolated")
}

func TestInMemoryCheckpointWithoutStableRunDoesNotInventReference(t *testing.T) {
	t.Parallel()
	checkpoint := &InMemoryCheckpoint{}
	require.NoError(t, checkpoint.Save(samplePosition(), sampleExecution()[:1]))
	ref, ok := checkpoint.ConversationReference()
	require.False(t, ok)
	require.Empty(t, ref)
}

func TestCheckpointReferenceParsingIsStrict(t *testing.T) {
	t.Parallel()
	const doltRevision = "8f09la6epq7omn89khmr0o1kfjgbgugn"
	valid, err := formatCheckpointReference("dolt", "run/one", 3, doltRevision)
	require.NoError(t, err)
	parsed, err := parseCheckpointReference(valid)
	require.NoError(t, err)
	require.Equal(t, checkpointReference{
		backend: "dolt", runID: "run/one", step: 3, revision: doltRevision,
	}, parsed)

	for _, invalid := range []string{
		"", "checkpoint:v1", valid + ":extra",
		"checkpoint:v1:unknown:cnVu:3:aGFzaA",
		"checkpoint:v1:dolt:***:3:aGFzaA",
		"checkpoint:v1:dolt:cnVu:03:aGFzaA",
		"checkpoint:v1:dolt:cnVu:-1:aGFzaA",
	} {
		_, err := parseCheckpointReference(invalid)
		require.ErrorIs(t, err, ErrConversationReferenceInvalid, invalid)
	}
}

func TestCheckpointReferenceRevisionGrammarIsBackendSpecific(t *testing.T) {
	t.Parallel()
	const (
		doltRevision   = "8f09la6epq7omn89khmr0o1kfjgbgugn"
		memoryRevision = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	)
	_, err := formatCheckpointReference("dolt", "run", 0, doltRevision)
	require.NoError(t, err)
	_, err = formatCheckpointReference("memory", "run", 0, memoryRevision)
	require.NoError(t, err)

	for backend, revisions := range map[string][]string{
		"dolt":   {memoryRevision, "8f09la6epq7omn89khmr0o1kfjgbgugw", "8F09la6epq7omn89khmr0o1kfjgbgugn"},
		"memory": {doltRevision, "g123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
	} {
		for _, revision := range revisions {
			_, err := formatCheckpointReference(backend, "run", 0, revision)
			require.ErrorIs(t, err, ErrConversationReferenceInvalid)
		}
	}
}

func TestInMemoryCheckpointReferenceRequiresConversationSnapshot(t *testing.T) {
	t.Parallel()
	checkpoint := NewInMemoryCheckpoint("run-memory")
	position := samplePosition()
	position.Snapshot.Conversation = json.RawMessage(nil)
	require.NoError(t, checkpoint.Save(position, sampleExecution()[:1]))
	_, ok := checkpoint.ConversationReference()
	require.False(t, ok)
}
