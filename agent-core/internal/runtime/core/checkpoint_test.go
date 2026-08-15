// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoopUndoReturnsSuccessfulResult(t *testing.T) {
	t.Parallel()

	res := NoopUndo("read")

	require.Equal(t, ToolDone, res.Signal)
	require.Equal(t, "read", res.CommandName)
	require.Contains(t, res.Output, "no-op")
}
