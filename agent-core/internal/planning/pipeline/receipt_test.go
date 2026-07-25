// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/extract"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/graph"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/plan"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
)

func TestPipelineReceiptRestoresFreshCommandAfterCheckpointRoundTrip(t *testing.T) {
	t.Parallel()

	ps := minimalState(t)
	node := ps.Graph.Nodes()[0]
	node.Status = graph.Executing
	node.Retries = 2
	ps.CurrentTask = &extract.Task{ID: "prior-task", SRDID: "srd001", NodeIDs: []string{node.ID}}
	ps.CurrentPlan = &plan.ImplementationPlan{Title: "prior-plan"}
	ps.IssueID = "issue-prior"
	ps.TaskDeps = map[string]string{"prior-task": "issue-prior"}
	retry := &toollm.ParseErrorRetryTracker{MaxConsecutive: 3}
	require.Equal(t, core.ToolDone, retry.ReportParseError())

	builder := &ParsePlanBuilder{PS: ps, Retry: retry}
	result := builder.Build(core.Result{Output: validRawPlan}).Execute()
	require.Equal(t, SigPlanReady, result.Signal)
	require.NotEmpty(t, result.Receipt)

	checkpoint := &core.InMemoryCheckpoint{}
	require.NoError(t, checkpoint.Save(core.Position{}, core.Execution{{
		CommandName: "parse_plan",
		Receipt:     result.Receipt,
	}}))
	_, execution, err := checkpoint.Load()
	require.NoError(t, err)
	require.Len(t, execution, 1)

	ps.CurrentTask = &extract.Task{ID: "replacement-task"}
	ps.CurrentPlan = &plan.ImplementationPlan{Title: "replacement-plan"}
	ps.IssueID = "issue-replacement"
	ps.TaskDeps = map[string]string{"replacement-task": "issue-replacement"}
	node.Status = graph.Done
	node.Retries = 9
	retry.Restore(3)

	undoResult := builder.BuildReverser().Undo(core.Result{Receipt: execution[0].Receipt})

	require.Equal(t, core.ToolDone, undoResult.Signal, undoResult.Output)
	require.Equal(t, "prior-task", ps.CurrentTask.ID)
	require.Equal(t, "prior-plan", ps.CurrentPlan.Title)
	require.Equal(t, "issue-prior", ps.IssueID)
	require.Equal(t, map[string]string{"prior-task": "issue-prior"}, ps.TaskDeps)
	require.Equal(t, graph.Executing, node.Status)
	require.Equal(t, 2, node.Retries)
	require.Equal(t, 1, retry.Snapshot())
}

func TestPipelineMutationBuildersImplementReverser(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)
	retry := &toollm.ParseErrorRetryTracker{}
	builders := []core.Builder{
		&ExtractTaskBuilder{PS: ps},
		&ExtractAllBuilder{PS: ps},
		&ParsePlanBuilder{PS: ps, Retry: retry},
		&LoadGraphBuilder{PS: ps},
		&RecordTrackerIssueBuilder{PS: ps},
		&MarkTaskDoneBuilder{PS: ps},
	}
	for _, builder := range builders {
		_, ok := builder.(core.Reverser)
		require.Truef(t, ok, "%T must implement core.Reverser", builder)
	}
}
