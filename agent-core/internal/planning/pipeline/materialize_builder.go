// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"

var materializePlan = DoMaterialize

type createIssueCmd struct {
	ps          *State
	snapshot    pipelineSnapshot
	hasSnapshot bool
	issueID     string
}

func (c *createIssueCmd) Name() string { return "create_issue" }
func (c *createIssueCmd) Undo(_ core.Result) core.Result {
	return undoPipelineSnapshot(c.Name(), c.ps, c.snapshot, c.hasSnapshot)
}

func (c *createIssueCmd) Execute() core.Result {
	c.snapshot = snapshotPipelineState(c.ps)
	c.hasSnapshot = true
	if c.ps.CurrentPlan == nil {
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Output: "no current plan to materialize"}
	}
	issueID, res := materializePlan(c.ps.Ctx, c.ps.Tracer, *c.ps.CurrentPlan, c.ps.Directory, c.ps.TaskDeps, c.Name())
	if res.Signal == core.CommandError {
		return res
	}
	c.recordIssue(issueID)
	return res
}

func (c *createIssueCmd) recordIssue(issueID string) {
	c.ps.IssueID = issueID
	c.issueID = issueID
	if c.ps.CurrentTask == nil {
		return
	}
	if c.ps.TaskDeps == nil {
		c.ps.TaskDeps = make(map[string]string)
	}
	c.ps.TaskDeps[c.ps.CurrentTask.ID] = issueID
}

// CreateIssueBuilder constructs create_issue commands.
type CreateIssueBuilder struct {
	PS *State
}

func (b *CreateIssueBuilder) Build(_ core.Result) core.Command {
	return &createIssueCmd{ps: b.PS}
}
