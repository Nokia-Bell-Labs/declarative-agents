// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package pipeline

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/graph"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestMarkTaskFailedMutatesBeforeRemainingWorkAndUndoes(t *testing.T) {
	ps := minimalState(t)
	selected := (&ExtractTaskBuilder{PS: ps}).Build(core.Result{}).Execute()
	require.Equal(t, SigTaskExtracted, selected.Signal)
	planning := (&MarkNodesPlanningBuilder{PS: ps}).Build(core.Result{}).Execute()
	require.Equal(t, SigNodesPlanning, planning.Signal)
	executing := (&MarkNodesExecutingBuilder{PS: ps}).Build(core.Result{}).Execute()
	require.Equal(t, SigNodesExecuting, executing.Signal)

	cmd := (&MarkTaskFailedBuilder{PS: ps}).Build(core.Result{})
	failed := cmd.Execute()
	require.Equal(t, SigTaskFailed, failed.Signal)
	for _, id := range ps.CurrentTask.NodeIDs {
		node, ok := ps.Graph.Node(id)
		require.True(t, ok)
		require.Equal(t, graph.Failed, node.Status)
	}
	remaining := (&RemainingWorkBuilder{PS: ps}).Build(core.Result{}).Execute()
	require.Equal(t, SigBlocked, remaining.Signal)

	require.Equal(t, core.ToolDone, cmd.Undo(failed).Signal)
	for _, id := range ps.CurrentTask.NodeIDs {
		node, ok := ps.Graph.Node(id)
		require.True(t, ok)
		require.Equal(t, graph.Executing, node.Status)
	}
}
