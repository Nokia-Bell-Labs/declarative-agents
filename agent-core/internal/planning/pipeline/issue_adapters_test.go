// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/extract"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/plan"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestFormatIssueEmitsTrackerAgnosticParameters(t *testing.T) {
	t.Parallel()
	ps := &State{
		CurrentPlan: &plan.ImplementationPlan{
			Title:              "Implement parser",
			Files:              []plan.PlanFile{{Path: "parser.go", Action: "create"}},
			Requirements:       []plan.PlanRequirement{{ID: "R1", Text: "Parse input"}},
			AcceptanceCriteria: []plan.PlanCriterion{{ID: "AC1", Text: "Tests pass"}},
		},
		Directory: "/tmp/workspace",
		TaskDeps:  map[string]string{"later": "issue-9", "earlier": "issue-2"},
	}
	cmd := (&FormatIssueBuilder{
		PS: ps, BodyPath: ".planner/body.yaml", DeliverableType: "documentation",
	}).Build(core.Result{})
	result := cmd.Execute()
	require.Equal(t, SigIssueFormatted, result.Signal)

	var output struct {
		Parameters map[string]string `json:"parameters"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	assert.Equal(t, "Implement parser", output.Parameters["title"])
	assert.Equal(t, ".planner/body.yaml", output.Parameters["path"])
	assert.Equal(t, ".planner/body.yaml", output.Parameters["body_file"])
	assert.Equal(t, "/tmp/workspace", output.Parameters["directory"])
	assert.Equal(t, "issue-2,issue-9", output.Parameters["deps"])
	assert.Contains(t, output.Parameters["content"], "deliverable_type: documentation")
	assert.Contains(t, output.Parameters["content"], "parser.go")

	require.Equal(t, core.ToolDone, cmd.Undo(result).Signal)
	assert.Equal(t, "Implement parser", ps.CurrentPlan.Title)
}

func TestFormatIssueRequiresCurrentPlan(t *testing.T) {
	t.Parallel()
	result := (&FormatIssueBuilder{PS: &State{}}).Build(core.Result{}).Execute()
	assert.Equal(t, core.CommandError, result.Signal)
	assert.Contains(t, result.Output, "no current plan")
}

func TestRecordTrackerIssueMapsConfiguredOutputAndUndo(t *testing.T) {
	t.Parallel()
	ps := &State{
		CurrentTask: &extract.Task{ID: "task-1"},
		IssueID:     "old-issue",
		TaskDeps:    map[string]string{"old-task": "old-issue"},
	}
	cmd := (&RecordTrackerIssueBuilder{PS: ps}).Build(core.Result{
		Signal: core.ToolDone,
		Output: `{"id":"tracker-42"}`,
	})
	result := cmd.Execute()
	require.Equal(t, SigMaterialized, result.Signal)
	assert.JSONEq(t, `{"issue_id":"tracker-42"}`, result.Output)
	assert.Equal(t, "tracker-42", ps.IssueID)
	assert.Equal(t, "tracker-42", ps.TaskDeps["task-1"])

	require.Equal(t, core.ToolDone, cmd.Undo(result).Signal)
	assert.Equal(t, "old-issue", ps.IssueID)
	assert.Equal(t, map[string]string{"old-task": "old-issue"}, ps.TaskDeps)
}

func TestRecordTrackerIssueRejectsInvalidOutputWithoutMutation(t *testing.T) {
	t.Parallel()
	for name, prior := range map[string]core.Result{
		"failed tracker": {Signal: core.ToolFailed, Output: "failed"},
		"invalid json":   {Signal: core.ToolDone, Output: "not-json"},
		"empty id":       {Signal: core.ToolDone, Output: `{"id":" "}`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ps := &State{
				CurrentTask: &extract.Task{ID: "task-1"},
				IssueID:     "old-issue",
				TaskDeps:    map[string]string{"old-task": "old-issue"},
			}
			result := (&RecordTrackerIssueBuilder{PS: ps}).Build(prior).Execute()
			assert.Equal(t, core.CommandError, result.Signal)
			assert.Equal(t, "old-issue", ps.IssueID)
			assert.Equal(t, map[string]string{"old-task": "old-issue"}, ps.TaskDeps)
		})
	}
}
