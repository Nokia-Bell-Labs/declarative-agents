// Copyright (c) 2026 Nokia. All rights reserved.

package control

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func noopTracer() tracing.Tracer { return tracing.NoopTracer{} }

func TestNudgeReread_UndoIsNoopBecauseCommandDoesNotMutateHistory(t *testing.T) {
	builder := &NudgeRereadBuilder{Tracer: noopTracer()}
	cmd := builder.Build(core.Result{Output: "edited file"})

	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal)
	require.Contains(t, res.Output, RereadNudge)

	undo := cmd.Undo(core.Result{})
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Contains(t, undo.Output, "no-op")
}
