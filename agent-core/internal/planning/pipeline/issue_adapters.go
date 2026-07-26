// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/plan"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const SigIssueFormatted core.Signal = "IssueFormatted"

const plannerIssueBodyPath = ".git/agent-planner/issue-body.yaml"

type formatIssueCmd struct {
	ps *State
}

func (c *formatIssueCmd) Name() string { return "format_issue" }

func (c *formatIssueCmd) Execute() core.Result {
	if c.ps.CurrentPlan == nil {
		return pipelineCommandError(c.Name(), "no current plan to format")
	}
	body, err := plan.FormatIssueDescription(*c.ps.CurrentPlan)
	if err != nil {
		return pipelineCommandError(c.Name(), err.Error())
	}
	deps := make([]string, 0, len(c.ps.TaskDeps))
	for _, issueID := range c.ps.TaskDeps {
		deps = append(deps, issueID)
	}
	sort.Strings(deps)
	output, err := json.Marshal(map[string]any{
		"parameters": map[string]string{
			"title":     c.ps.CurrentPlan.Title,
			"path":      plannerIssueBodyPath,
			"body_file": plannerIssueBodyPath,
			"content":   body,
			"directory": c.ps.Directory,
			"deps":      strings.Join(deps, ","),
		},
	})
	if err != nil {
		return pipelineCommandError(c.Name(), fmt.Sprintf("encode issue parameters: %v", err))
	}
	return core.Result{CommandName: c.Name(), Signal: SigIssueFormatted, Output: string(output)}
}

func (c *formatIssueCmd) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.Name())
}

// FormatIssueBuilder constructs the state adapter that prepares parameters for
// the planner's write and tracker exec words.
type FormatIssueBuilder struct {
	PS *State
}

func (b *FormatIssueBuilder) Build(_ core.Result) core.Command {
	return &formatIssueCmd{ps: b.PS}
}

type trackerIssueResult struct {
	ID string `json:"id"`
}

type recordTrackerIssueCmd struct {
	ps    *State
	prior core.Result
}

func (c *recordTrackerIssueCmd) Name() string { return "record_tracker_issue" }

func (c *recordTrackerIssueCmd) Execute() (result core.Result) {
	snapshot := snapshotPipelineState(c.ps)
	defer func() { result = withPipelineReceipt(result, snapshot, nil) }()
	if c.prior.Signal != core.ToolDone {
		return pipelineCommandError(c.Name(), fmt.Sprintf("tracker command returned %s", c.prior.Signal))
	}
	var created trackerIssueResult
	if err := json.Unmarshal([]byte(c.prior.Output), &created); err != nil {
		return pipelineCommandError(c.Name(), fmt.Sprintf("parse tracker output: %v", err))
	}
	created.ID = strings.TrimSpace(created.ID)
	if created.ID == "" {
		return pipelineCommandError(c.Name(), "tracker output contains an empty issue ID")
	}

	c.ps.IssueID = created.ID
	if c.ps.CurrentTask != nil {
		if c.ps.TaskDeps == nil {
			c.ps.TaskDeps = make(map[string]string)
		}
		c.ps.TaskDeps[c.ps.CurrentTask.ID] = created.ID
	}
	output, err := json.Marshal(map[string]string{"issue_id": created.ID})
	if err != nil {
		snapshot.restore(c.ps)
		return pipelineCommandError(c.Name(), fmt.Sprintf("encode issue result: %v", err))
	}
	return core.Result{CommandName: c.Name(), Signal: SigMaterialized, Output: string(output)}
}

func (c *recordTrackerIssueCmd) Undo(prior core.Result) core.Result {
	return undoPipelineReceipt(c.Name(), c.ps, nil, prior.Receipt)
}

// RecordTrackerIssueBuilder constructs the state adapter that records the ID
// returned by the selected tracker exec word.
type RecordTrackerIssueBuilder struct {
	PS *State
}

func (b *RecordTrackerIssueBuilder) Build(prior core.Result) core.Command {
	return &recordTrackerIssueCmd{ps: b.PS, prior: prior}
}

func (b *RecordTrackerIssueBuilder) BuildReverser() core.Command {
	return &recordTrackerIssueCmd{ps: b.PS}
}

func pipelineCommandError(commandName, message string) core.Result {
	err := fmt.Errorf("%s: %s", commandName, message)
	return core.Result{CommandName: commandName, Signal: core.CommandError, Output: err.Error(), Err: err}
}
