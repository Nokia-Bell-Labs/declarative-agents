// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/graph"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestPassThroughBuildersSplitSelectionPlanAndGraphMutation(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)

	selectCmd := (&SelectAllReadyBuilder{PS: ps}).Build(core.Result{})
	selected := selectCmd.Execute()
	require.Equal(t, SigReadySelected, selected.Signal)
	require.NotNil(t, ps.CurrentTask)
	require.Nil(t, ps.CurrentPlan)
	for _, id := range ps.CurrentTask.NodeIDs {
		node, ok := ps.Graph.Node(id)
		require.True(t, ok)
		assert.Equal(t, graph.Pending, node.Status)
	}

	seedCmd := (&SeedPassThroughPlanBuilder{
		PS: ps, Title: "Profile-owned title", Summary: "Profile-owned summary",
	}).Build(core.Result{})
	seeded := seedCmd.Execute()
	require.Equal(t, SigPlanSeeded, seeded.Signal)
	require.NotNil(t, ps.CurrentPlan)
	assert.Equal(t, "Profile-owned title", ps.CurrentPlan.Title)
	assert.Equal(t, "Profile-owned summary", ps.CurrentPlan.Summary)
	for _, id := range ps.CurrentTask.NodeIDs {
		node, ok := ps.Graph.Node(id)
		require.True(t, ok)
		assert.Equal(t, graph.Pending, node.Status)
	}

	markCmd := (&MarkNodesPlanningBuilder{PS: ps}).Build(core.Result{})
	marked := markCmd.Execute()
	require.Equal(t, SigNodesPlanning, marked.Signal)
	for _, id := range ps.CurrentTask.NodeIDs {
		node, ok := ps.Graph.Node(id)
		require.True(t, ok)
		assert.Equal(t, graph.Planning, node.Status)
	}
}

func TestPassThroughBuildersUndoIndependentMutations(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)

	selectCmd := (&SelectAllReadyBuilder{PS: ps}).Build(core.Result{})
	selected := selectCmd.Execute()
	require.Equal(t, SigReadySelected, selected.Signal)
	task := cloneTask(ps.CurrentTask)

	seedCmd := (&SeedPassThroughPlanBuilder{PS: ps, Title: "Title", Summary: "Summary"}).Build(core.Result{})
	seeded := seedCmd.Execute()
	require.Equal(t, SigPlanSeeded, seeded.Signal)
	require.NotNil(t, ps.CurrentPlan)
	require.Equal(t, core.ToolDone, seedCmd.Undo(seeded).Signal)
	require.Nil(t, ps.CurrentPlan)
	require.Equal(t, task, ps.CurrentTask)

	markCmd := (&MarkNodesPlanningBuilder{PS: ps}).Build(core.Result{})
	marked := markCmd.Execute()
	require.Equal(t, SigNodesPlanning, marked.Signal)
	require.Equal(t, core.ToolDone, markCmd.Undo(marked).Signal)
	for _, id := range ps.CurrentTask.NodeIDs {
		node, ok := ps.Graph.Node(id)
		require.True(t, ok)
		assert.Equal(t, graph.Pending, node.Status)
	}

	require.Equal(t, core.ToolDone, selectCmd.Undo(selected).Signal)
	require.Nil(t, ps.CurrentTask)
}

func TestSelectAllReadyBuilderNoReady(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)
	markAllDone(t, ps.Graph)

	cmd := (&SelectAllReadyBuilder{PS: ps}).Build(core.Result{})
	result := cmd.Execute()

	assert.Equal(t, SigNoTask, result.Signal)
	remaining := (&RemainingWorkBuilder{PS: ps}).Build(core.Result{}).Execute()
	assert.Equal(t, SigAllDone, remaining.Signal)
}
