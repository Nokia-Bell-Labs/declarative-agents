// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/extract"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/graph"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/plan"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestMarkNodesExecutingIsIndependentAndReversible(t *testing.T) {
	ps := minimalState(t)
	node := ps.Graph.Nodes()[0]
	ps.CurrentTask = &extract.Task{ID: "task-1", NodeIDs: []string{node.ID}}
	require.NoError(t, node.MarkPlanning())

	cmd := (&MarkNodesExecutingBuilder{PS: ps}).Build(core.Result{})
	result := cmd.Execute()
	require.Equal(t, SigNodesExecuting, result.Signal)
	require.Equal(t, graph.Executing, node.Status)

	require.Equal(t, core.ToolDone, cmd.Undo(result).Signal)
	require.Equal(t, graph.Planning, node.Status)
}

func TestFormatTaskFileProducesWriteParametersWithoutWriting(t *testing.T) {
	ps := &State{CurrentPlan: &plan.ImplementationPlan{Title: "Implement parser", Summary: "Add parser behavior."}}
	result := (&FormatTaskFileBuilder{PS: ps}).Build(core.Result{}).Execute()
	require.Equal(t, SigTaskFileFormatted, result.Signal)

	var output struct {
		Parameters struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		} `json:"parameters"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Equal(t, plannerTaskFilePath, output.Parameters.Path)
	require.Contains(t, output.Parameters.Content, "title: Implement parser")
	require.Contains(t, output.Parameters.Content, "summary: Add parser behavior.")
}

func TestFormatTaskFileRequiresCurrentPlan(t *testing.T) {
	result := (&FormatTaskFileBuilder{PS: &State{}}).Build(core.Result{}).Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.ErrorContains(t, result.Err, "current plan is required")
}
