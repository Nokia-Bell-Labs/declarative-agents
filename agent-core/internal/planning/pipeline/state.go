// Copyright (c) 2026 Nokia. All rights reserved.

// Package pipeline implements the builtin tool builders for the planner
// pipeline state machine. These tools orchestrate task extraction,
// prompt assembly, LLM-based planning, issue creation, and task execution.
package pipeline

import (
	"context"
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/planning/extract"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/planning/graph"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/planning/plan"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/support/execute"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/spec"
)

// Pipeline signals aligned with agents/planner/machine.yaml.
const (
	SigTaskExtracted    core.Signal = "TaskExtracted"
	SigAllDone          core.Signal = "AllDone"
	SigBlocked          core.Signal = "Blocked"
	SigPlanReady        core.Signal = "PlanReady"
	SigMaterialized     core.Signal = "Materialized"
	SigExecutionDone    core.Signal = "ExecutionDone"
	SigExecutionFailed  core.Signal = "ExecutionFailed"
	SigRetryAvailable   core.Signal = "RetryAvailable"
	SigRetriesExhausted core.Signal = "RetriesExhausted"
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
	MaxRetries  int
	Tracer      tracing.Tracer

	ExecConfig execute.Config
	Ctx        context.Context
	retryCount int
}

type pipelineSnapshot struct {
	currentTask *extract.Task
	currentPlan *plan.ImplementationPlan
	issueID     string
	taskDeps    map[string]string
	retryCount  int
	nodeStates  map[string]nodeSnapshot
}

type nodeSnapshot struct {
	status  graph.Status
	retries int
}

type pipelineSnapshotPayload struct {
	CurrentTask *extract.Task                        `json:"current_task,omitempty"`
	CurrentPlan *plan.ImplementationPlan             `json:"current_plan,omitempty"`
	IssueID     string                               `json:"issue_id,omitempty"`
	TaskDeps    map[string]string                    `json:"task_deps,omitempty"`
	RetryCount  int                                  `json:"retry_count"`
	NodeStates  map[string]pipelineNodeStateSnapshot `json:"node_states,omitempty"`
}

type pipelineNodeStateSnapshot struct {
	Status  graph.Status `json:"status"`
	Retries int          `json:"retries"`
}

type pipelineUndoPayload struct {
	DomainState pipelineSnapshotPayload `json:"domain_state"`
}

type BoundaryCompensationInfo struct {
	Strategy       string   `json:"strategy"`
	Reason         string   `json:"reason,omitempty"`
	Requires       []string `json:"requires,omitempty"`
	WorkspacePaths []string `json:"workspace_paths,omitempty"`
	IssueID        string   `json:"issue_id,omitempty"`
	ChildRunID     string   `json:"child_run_id,omitempty"`
}

func snapshotPipelineState(ps *State) pipelineSnapshot {
	snap := pipelineSnapshot{
		currentTask: cloneTask(ps.CurrentTask),
		currentPlan: clonePlan(ps.CurrentPlan),
		issueID:     ps.IssueID,
		taskDeps:    cloneStringMap(ps.TaskDeps),
		retryCount:  ps.retryCount,
	}
	if ps.Graph != nil {
		snap.nodeStates = make(map[string]nodeSnapshot)
		for _, n := range ps.Graph.Nodes() {
			snap.nodeStates[n.ID] = nodeSnapshot{status: n.Status, retries: n.Retries}
		}
	}
	return snap
}

func mementoPipelineSnapshot(
	commandName string,
	snap pipelineSnapshot,
	ok bool,
	kind core.UndoMementoKind,
) (core.UndoMemento, error) {
	if !ok {
		return core.UndoMemento{}, fmt.Errorf("%w: no pipeline snapshot recorded for %s", core.ErrUndoMementoMissing, commandName)
	}
	memento, err := core.NewUndoMemento(commandName, kind, pipelineUndoPayload{DomainState: pipelineSnapshotToPayload(snap)})
	if err != nil {
		return core.UndoMemento{}, err
	}
	if kind == core.UndoMementoCompensatable {
		memento.Description = "restores planner state snapshot; external side effects require compensation"
	}
	return memento, nil
}

func pipelineSnapshotToPayload(snap pipelineSnapshot) pipelineSnapshotPayload {
	payload := pipelineSnapshotPayload{
		CurrentTask: cloneTask(snap.currentTask),
		CurrentPlan: clonePlan(snap.currentPlan),
		IssueID:     snap.issueID,
		TaskDeps:    cloneStringMap(snap.taskDeps),
		RetryCount:  snap.retryCount,
	}
	if len(snap.nodeStates) > 0 {
		payload.NodeStates = make(map[string]pipelineNodeStateSnapshot, len(snap.nodeStates))
		for id, ns := range snap.nodeStates {
			payload.NodeStates[id] = pipelineNodeStateSnapshot{Status: ns.status, Retries: ns.retries}
		}
	}
	return payload
}

func (s pipelineSnapshot) restore(ps *State) {
	ps.CurrentTask = cloneTask(s.currentTask)
	ps.CurrentPlan = clonePlan(s.currentPlan)
	ps.IssueID = s.issueID
	ps.TaskDeps = cloneStringMap(s.taskDeps)
	ps.retryCount = s.retryCount
	if ps.Graph != nil {
		for id, ns := range s.nodeStates {
			if n, ok := ps.Graph.Node(id); ok {
				n.Status = ns.status
				n.Retries = ns.retries
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

func undoPipelineSnapshot(commandName string, ps *State, snap pipelineSnapshot, ok bool) core.Result {
	if !ok {
		err := fmt.Errorf("undo %s: no pipeline snapshot recorded", commandName)
		return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
	}
	snap.restore(ps)
	return core.Result{Signal: core.ToolDone, CommandName: commandName, Output: "undo: restored pipeline state"}
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

func (s *State) currentTaskID() string {
	if s.CurrentTask != nil {
		return s.CurrentTask.ID
	}
	return ""
}
