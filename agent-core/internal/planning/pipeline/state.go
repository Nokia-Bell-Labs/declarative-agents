// Copyright (c) 2026 Nokia. All rights reserved.

// Package pipeline implements the builtin tool builders for the planner
// pipeline state machine. These tools orchestrate task extraction,
// prompt assembly, LLM-based planning, issue creation, and task execution.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/extract"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/graph"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/plan"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

// Pipeline signals aligned with agents/planner/machine.yaml.
const (
	SigTaskExtracted     core.Signal = "TaskExtracted"
	SigNoTask            core.Signal = "NoTask"
	SigReadySelected     core.Signal = "ReadySelected"
	SigPlanSeeded        core.Signal = "PassThroughPlanSeeded"
	SigNodesPlanning     core.Signal = "NodesMarkedPlanning"
	SigNodesExecuting    core.Signal = "NodesMarkedExecuting"
	SigTaskFileFormatted core.Signal = "TaskFileFormatted"
	SigAllDone           core.Signal = "AllDone"
	SigBlocked           core.Signal = "Blocked"
	SigPlanReady         core.Signal = "PlanReady"
	SigMaterialized      core.Signal = "Materialized"
	SigExecutionDone     core.Signal = "ExecutionDone"
	SigExecutionFailed   core.Signal = "ExecutionFailed"
	SigTaskCompleted     core.Signal = "TaskCompleted"
	SigWorkRemaining     core.Signal = "WorkRemaining"
)

// State holds the shared mutable state for a pipeline run.
// All pipeline tools read and write through this struct.
type State struct {
	Graph       *graph.Graph
	Corpus      *spec.Corpus
	Extractor   *extract.Extractor
	CurrentTask *extract.Task
	CurrentPlan *plan.ImplementationPlan
	IssueID     string
	TaskDeps    map[string]string
	Directory   string
	MaxWeight   int
	Tracer      tracing.Tracer

	Ctx context.Context
}

type pipelineSnapshot struct {
	CurrentTask      *extract.Task            `json:"current_task,omitempty"`
	CurrentPlan      *plan.ImplementationPlan `json:"current_plan,omitempty"`
	IssueID          string                   `json:"issue_id,omitempty"`
	TaskDeps         map[string]string        `json:"task_deps,omitempty"`
	NodeStates       map[string]nodeSnapshot  `json:"node_states,omitempty"`
	GraphPresent     bool                     `json:"graph_present"`
	CorpusPresent    bool                     `json:"corpus_present"`
	ExtractorPresent bool                     `json:"extractor_present"`
}

type nodeSnapshot struct {
	Status  graph.Status `json:"status"`
	Retries int          `json:"retries"`
}

type pipelineReceipt struct {
	Version       int              `json:"version"`
	Snapshot      pipelineSnapshot `json:"snapshot"`
	PreviousRetry *int             `json:"previous_retry,omitempty"`
}

func snapshotPipelineState(ps *State) pipelineSnapshot {
	snap := pipelineSnapshot{
		CurrentTask:      cloneTask(ps.CurrentTask),
		CurrentPlan:      clonePlan(ps.CurrentPlan),
		IssueID:          ps.IssueID,
		TaskDeps:         cloneStringMap(ps.TaskDeps),
		GraphPresent:     ps.Graph != nil,
		CorpusPresent:    ps.Corpus != nil,
		ExtractorPresent: ps.Extractor != nil,
	}
	if ps.Graph != nil {
		snap.NodeStates = make(map[string]nodeSnapshot)
		for _, n := range ps.Graph.Nodes() {
			snap.NodeStates[n.ID] = nodeSnapshot{Status: n.Status, Retries: n.Retries}
		}
	}
	return snap
}

func (s pipelineSnapshot) restore(ps *State) {
	ps.CurrentTask = cloneTask(s.CurrentTask)
	ps.CurrentPlan = clonePlan(s.CurrentPlan)
	ps.IssueID = s.IssueID
	ps.TaskDeps = cloneStringMap(s.TaskDeps)
	if !s.GraphPresent {
		ps.Graph = nil
	}
	if !s.CorpusPresent {
		ps.Corpus = nil
	}
	if !s.ExtractorPresent {
		ps.Extractor = nil
	}
	if ps.Graph != nil {
		for id, ns := range s.NodeStates {
			if n, ok := ps.Graph.Node(id); ok {
				n.Status = ns.Status
				n.Retries = ns.Retries
			}
		}
	}
}

func cloneTask(t *extract.Task) *extract.Task {
	if t == nil {
		return nil
	}
	clone := *t
	clone.NodeIDs = append([]string(nil), t.NodeIDs...)
	return &clone
}

func clonePlan(p *plan.ImplementationPlan) *plan.ImplementationPlan {
	if p == nil {
		return nil
	}
	clone := *p
	clone.Files = append([]plan.PlanFile(nil), p.Files...)
	clone.Requirements = append([]plan.PlanRequirement(nil), p.Requirements...)
	clone.DesignDecisions = append([]plan.PlanDecision(nil), p.DesignDecisions...)
	clone.AcceptanceCriteria = append([]plan.PlanCriterion(nil), p.AcceptanceCriteria...)
	return &clone
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func withPipelineReceipt(result core.Result, snap pipelineSnapshot, previousRetry *int) core.Result {
	data, err := json.Marshal(pipelineReceipt{Version: 1, Snapshot: snap, PreviousRetry: previousRetry})
	if err != nil {
		return pipelineReceiptError(result.CommandName, fmt.Errorf("encode pipeline receipt: %w", err))
	}
	result.Receipt = string(data)
	return result
}

type retryRestorer interface {
	Restore(int)
}

func undoPipelineReceipt(commandName string, ps *State, retry retryRestorer, receipt string) core.Result {
	if receipt == "" {
		return pipelineReceiptError(commandName, fmt.Errorf("pipeline receipt is required"))
	}
	var decoded pipelineReceipt
	if err := json.Unmarshal([]byte(receipt), &decoded); err != nil {
		return pipelineReceiptError(commandName, fmt.Errorf("decode pipeline receipt: %w", err))
	}
	if decoded.Version != 1 {
		return pipelineReceiptError(commandName, fmt.Errorf("unsupported pipeline receipt version %d", decoded.Version))
	}
	decoded.Snapshot.restore(ps)
	if retry != nil && decoded.PreviousRetry != nil {
		retry.Restore(*decoded.PreviousRetry)
	}
	return core.Result{Signal: core.ToolDone, CommandName: commandName, Output: "undo: restored pipeline state"}
}

func pipelineReceiptError(commandName string, err error) core.Result {
	wrapped := fmt.Errorf("undo %s: %w", commandName, err)
	return core.Result{Signal: core.CommandError, CommandName: commandName, Output: wrapped.Error(), Err: wrapped}
}

// classifyEmpty determines whether the graph is fully done or blocked.
func (s *State) classifyEmpty() (core.Signal, string) {
	for _, n := range s.Graph.Nodes() {
		if n.Status == graph.Pending || n.Status == graph.Planning || n.Status == graph.Executing {
			return SigBlocked, fmt.Sprintf("blocked: %d nodes have unmet dependencies", s.countPending())
		}
	}
	return SigAllDone, "all tasks completed"
}

func (s *State) countPending() int {
	count := 0
	for _, n := range s.Graph.Nodes() {
		if n.Status != graph.Done && n.Status != graph.Failed {
			count++
		}
	}
	return count
}
