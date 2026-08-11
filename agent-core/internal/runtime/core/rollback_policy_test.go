// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUndoStrategyVocabulary(t *testing.T) {
	t.Parallel()

	require.True(t, KnownUndoStrategy("file_snapshot_restore"))
	require.True(t, UndoStrategyAllowed("reversible", "file_snapshot_restore"))
	require.False(t, UndoStrategyAllowed("irreversible", "workspace_restore"))
	require.False(t, KnownUndoStrategy("workpace_restore"))
}

func TestExecUndoStrategySupportIsClosed(t *testing.T) {
	t.Parallel()

	for _, strategy := range []string{"noop", "workspace_restore", "compensating_action", "irreversible"} {
		require.True(t, UndoStrategySupported("exec", strategy), strategy)
	}
	require.False(t, UndoStrategySupported("exec", "conversation_restore"))
	require.Contains(t, SupportedUndoStrategies("exec"), "compensating_action")
}
