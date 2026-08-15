// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConversationHistoryLifecycle(t *testing.T) {
	t.Parallel()

	conversation := NewConversation(nil, "", ChatOptions{})
	conversation.Append(Message{Role: User, Content: "one"})
	conversation.Append(Message{Role: Assistant, Content: "two"})
	require.Equal(t, 2, conversation.Len())
	require.Equal(t, conversation.History(), conversation.Messages())

	snapshot := conversation.Snapshot()
	conversation.Append(Message{Role: User, Content: "three"})
	require.NoError(t, conversation.TruncateTo(2))
	require.Equal(t, snapshot, conversation.Messages())

	conversation.Restore([]Message{{Role: User, Content: "restored"}})
	require.Equal(t, []Message{{Role: User, Content: "restored"}}, conversation.Messages())
	conversation.Reset()
	require.Empty(t, conversation.Messages())
}

func TestConversationHistoryReturnsCopies(t *testing.T) {
	t.Parallel()

	conversation := NewConversation(nil, "", ChatOptions{})
	conversation.Append(Message{Role: User, Content: "original"})
	history := conversation.History()
	history[0].Content = "mutated"

	require.Equal(t, "original", conversation.Messages()[0].Content)
	require.Error(t, conversation.TruncateTo(2))
}
