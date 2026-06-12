// Copyright (c) 2026 Nokia. All rights reserved.

// Package pipeline implements the builtin tool builders for the planner
// pipeline state machine. These tools orchestrate task extraction,
// prompt assembly, LLM-based planning, issue creation, and task execution.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/planning/extract"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/planning/graph"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/planning/plan"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/support/execute"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/spec"
)

var materializePlan = DoMaterialize

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

func mementoPipelineSnapshot(commandName string, snap pipelineSnapshot, ok bool, kind core.UndoMementoKind) (core.UndoMemento, error) {
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

// --- extract_task ---

type extractTaskCmd struct {
	ps          *State
	snapshot    pipelineSnapshot
	hasSnapshot bool
}

func (c *extractTaskCmd) Name() string { return "extract_task" }
func (c *extractTaskCmd) Undo() core.Result {
	return undoPipelineSnapshot(c.Name(), c.ps, c.snapshot, c.hasSnapshot)
}

func (c *extractTaskCmd) UndoMemento() (core.UndoMemento, error) {
	return mementoPipelineSnapshot(c.Name(), c.snapshot, c.hasSnapshot, core.UndoMementoReversible)
}

func (c *extractTaskCmd) Execute() core.Result {
	c.snapshot = snapshotPipelineState(c.ps)
	c.hasSnapshot = true
	task := c.ps.Extractor.ExtractNext(c.ps.Graph, c.ps.MaxWeight)
	if task == nil {
		sig, msg := c.ps.classifyEmpty()
		return core.Result{
			CommandName: c.Name(),
			Signal:      sig,
			Output:      msg,
		}
	}
	c.ps.retryCount = 0

	c.ps.CurrentTask = task
	c.ps.Tracer.Event("pipeline.task_extracted",
		attribute.String("task.id", task.ID),
		attribute.String("task.srd_id", task.SRDID),
		attribute.Int("task.weight", task.Weight),
		attribute.Int("task.node_count", len(task.NodeIDs)),
	)

	return core.Result{
		CommandName: c.Name(),
		Signal:      SigTaskExtracted,
		Output:      fmt.Sprintf("extracted task %s (weight=%d, nodes=%d)", task.ID, task.Weight, len(task.NodeIDs)),
	}
}

// ExtractTaskBuilder constructs extract_task commands.
type ExtractTaskBuilder struct {
	PS *State
}

func (b *ExtractTaskBuilder) Build(_ core.Result) core.Command {
	return &extractTaskCmd{ps: b.PS}
}

// --- extract_all ---

type extractAllCmd struct {
	ps          *State
	snapshot    pipelineSnapshot
	hasSnapshot bool
}

func (c *extractAllCmd) Name() string { return "extract_all" }
func (c *extractAllCmd) Undo() core.Result {
	return undoPipelineSnapshot(c.Name(), c.ps, c.snapshot, c.hasSnapshot)
}

func (c *extractAllCmd) UndoMemento() (core.UndoMemento, error) {
	return mementoPipelineSnapshot(c.Name(), c.snapshot, c.hasSnapshot, core.UndoMementoReversible)
}

func (c *extractAllCmd) Execute() core.Result {
	c.snapshot = snapshotPipelineState(c.ps)
	c.hasSnapshot = true
	ready := c.ps.Graph.Ready()
	if len(ready) == 0 {
		sig, msg := c.ps.classifyEmpty()
		return core.Result{
			CommandName: c.Name(),
			Signal:      sig,
			Output:      msg,
		}
	}
	c.ps.retryCount = 0

	nodeIDs := make([]string, len(ready))
	for i, n := range ready {
		nodeIDs[i] = n.ID
	}

	task := &extract.Task{
		ID:      "all",
		NodeIDs: nodeIDs,
		Weight:  len(nodeIDs),
		SRDID:   ready[0].SRDID,
	}
	c.ps.CurrentTask = task

	return core.Result{
		CommandName: c.Name(),
		Signal:      SigTaskExtracted,
		Output:      fmt.Sprintf("extracted all %d nodes as single task", len(nodeIDs)),
	}
}

// ExtractAllBuilder constructs extract_all commands.
type ExtractAllBuilder struct {
	PS *State
}

func (b *ExtractAllBuilder) Build(_ core.Result) core.Command {
	return &extractAllCmd{ps: b.PS}
}

// --- assemble_prompt ---

type assemblePromptCmd struct {
	ps *State
}

func (c *assemblePromptCmd) Name() string      { return "assemble_prompt" }
func (c *assemblePromptCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

func (c *assemblePromptCmd) Execute() core.Result {
	task := c.ps.CurrentTask
	if task == nil {
		return core.Result{
			CommandName: c.Name(),
			Signal:      core.CommandError,
			Output:      "no current task",
		}
	}

	srd, ok := c.ps.Corpus.SRDs[task.SRDID]
	if !ok {
		return core.Result{
			CommandName: c.Name(),
			Signal:      core.CommandError,
			Output:      fmt.Sprintf("SRD %q not found in corpus", task.SRDID),
		}
	}

	var items []plan.TaskItem
	for _, nid := range task.NodeIDs {
		n, _ := c.ps.Graph.Node(nid)
		if n != nil {
			items = append(items, plan.TaskItem{ID: nid, Text: n.Text})
		}
	}

	tc := plan.TaskContext{ID: task.ID, SRDID: task.SRDID, Items: items}
	sc := plan.SRDContext{Problem: srd.Problem, Goals: srd.Goals}
	for _, ac := range srd.AcceptanceCriteria {
		sc.AcceptanceCriteria = append(sc.AcceptanceCriteria, ac.ID+": "+ac.Criterion)
	}

	prompt, err := plan.AssemblePrompt(tc, sc, nil, nil)
	if err != nil {
		return core.Result{
			CommandName: c.Name(),
			Signal:      core.CommandError,
			Err:         err,
			Output:      fmt.Sprintf("assemble prompt: %v", err),
		}
	}

	return core.Result{
		CommandName: c.Name(),
		Signal:      core.ToolDone,
		Output:      prompt,
	}
}

// AssemblePromptBuilder constructs assemble_prompt commands.
type AssemblePromptBuilder struct {
	PS *State
}

func (b *AssemblePromptBuilder) Build(_ core.Result) core.Command {
	return &assemblePromptCmd{ps: b.PS}
}

// --- parse_plan ---

type parsePlanCmd struct {
	ps          *State
	rawResp     string
	snapshot    pipelineSnapshot
	hasSnapshot bool
}

func (c *parsePlanCmd) Name() string { return "parse_plan" }
func (c *parsePlanCmd) Undo() core.Result {
	return undoPipelineSnapshot(c.Name(), c.ps, c.snapshot, c.hasSnapshot)
}

func (c *parsePlanCmd) UndoMemento() (core.UndoMemento, error) {
	return mementoPipelineSnapshot(c.Name(), c.snapshot, c.hasSnapshot, core.UndoMementoReversible)
}

func (c *parsePlanCmd) Execute() core.Result {
	c.snapshot = snapshotPipelineState(c.ps)
	c.hasSnapshot = true
	p, res := DoParsePlan(c.Name(), c.rawResp)
	if res.Signal == core.ParseFailed {
		c.ps.Tracer.Event("pipeline.parse_plan_failed",
			attribute.String("error", res.Output),
		)
		return res
	}

	c.ps.CurrentPlan = &p
	c.ps.Tracer.Event("pipeline.plan_parsed",
		attribute.String("plan.title", p.Title),
		attribute.Int("plan.files", len(p.Files)),
		attribute.Int("plan.requirements", len(p.Requirements)),
	)
	return res
}

// ParsePlanBuilder constructs parse_plan commands.
type ParsePlanBuilder struct {
	PS *State
}

func (b *ParsePlanBuilder) Build(res core.Result) core.Command {
	return &parsePlanCmd{ps: b.PS, rawResp: res.Output}
}

// --- create_issue ---

type createIssueCmd struct {
	ps          *State
	snapshot    pipelineSnapshot
	hasSnapshot bool
	issueID     string
}

func (c *createIssueCmd) Name() string { return "create_issue" }
func (c *createIssueCmd) Undo() core.Result {
	return undoPipelineSnapshot(c.Name(), c.ps, c.snapshot, c.hasSnapshot)
}

func (c *createIssueCmd) UndoMemento() (core.UndoMemento, error) {
	memento, err := mementoPipelineSnapshot(c.Name(), c.snapshot, c.hasSnapshot, core.UndoMementoCompensatable)
	if err != nil {
		return core.UndoMemento{}, err
	}
	if c.issueID == "" {
		return memento, nil
	}
	memento, err = core.NewUndoMemento(c.Name(), core.UndoMementoCompensatable, struct {
		DomainState          pipelineSnapshotPayload  `json:"domain_state"`
		BoundaryCompensation BoundaryCompensationInfo `json:"boundary_compensation"`
	}{
		DomainState: pipelineSnapshotToPayload(c.snapshot),
		BoundaryCompensation: BoundaryCompensationInfo{
			Strategy: "close_or_delete_created_issue",
			Reason:   "planner materialized a Beads issue",
			Requires: []string{"issue_id"},
			IssueID:  c.issueID,
		},
	})
	if err != nil {
		return core.UndoMemento{}, err
	}
	memento.Description = "restore planner state and compensate created issue"
	return memento, nil
}

func (c *createIssueCmd) Execute() core.Result {
	c.snapshot = snapshotPipelineState(c.ps)
	c.hasSnapshot = true
	if c.ps.CurrentPlan == nil {
		return core.Result{
			CommandName: c.Name(),
			Signal:      core.CommandError,
			Output:      "no current plan to materialize",
		}
	}

	issueID, res := materializePlan(c.ps.Ctx, c.ps.Tracer, *c.ps.CurrentPlan, c.ps.Directory, c.ps.TaskDeps, c.Name())
	if res.Signal == core.CommandError {
		return res
	}

	c.ps.IssueID = issueID
	c.issueID = issueID
	if c.ps.CurrentTask != nil {
		if c.ps.TaskDeps == nil {
			c.ps.TaskDeps = make(map[string]string)
		}
		c.ps.TaskDeps[c.ps.CurrentTask.ID] = issueID
	}
	return res
}

// CreateIssueBuilder constructs create_issue commands.
type CreateIssueBuilder struct {
	PS *State
}

func (b *CreateIssueBuilder) Build(_ core.Result) core.Command {
	return &createIssueCmd{ps: b.PS}
}

// --- execute_task ---

type executeTaskCmd struct {
	ps *State
}

func (c *executeTaskCmd) Name() string { return "execute_task" }
func (c *executeTaskCmd) Undo() core.Result {
	return core.Result{
		Signal:      core.CommandError,
		CommandName: c.Name(),
		Output:      "undo execute_task requires child agent history or workspace compensation",
		Err:         fmt.Errorf("undo execute_task requires child agent history or workspace compensation"),
	}
}
func (c *executeTaskCmd) UndoMemento() (core.UndoMemento, error) {
	currentTaskID := ""
	if c.ps.CurrentTask != nil {
		currentTaskID = c.ps.CurrentTask.ID
	}
	memento, err := core.NewUndoMemento(c.Name(), core.UndoMementoCompensatable, struct {
		BoundaryCompensation BoundaryCompensationInfo `json:"boundary_compensation"`
	}{
		BoundaryCompensation: BoundaryCompensationInfo{
			Strategy:       "child_agent_workspace_restore",
			Reason:         "execute_task runs the generator agent for a planner task",
			Requires:       []string{"child_history", "Workspace"},
			WorkspacePaths: []string{c.ps.Directory},
			ChildRunID:     currentTaskID,
		},
	})
	if err != nil {
		return core.UndoMemento{}, err
	}
	memento.Description = "restore or compensate child generator workspace effects"
	return memento, nil
}

func (c *executeTaskCmd) Execute() core.Result {
	if c.ps.CurrentTask == nil || c.ps.CurrentPlan == nil {
		return core.Result{
			CommandName: c.Name(),
			Signal:      core.CommandError,
			Output:      "no current task or plan",
		}
	}

	result, err := execute.Execute(
		c.ps.Ctx,
		c.ps.Tracer,
		c.ps.ExecConfig,
		c.ps.CurrentTask.ID,
		c.ps.Directory,
		c.ps.CurrentPlan,
	)
	if err != nil {
		return core.Result{
			CommandName: c.Name(),
			Signal:      core.CommandError,
			Err:         err,
			Output:      err.Error(),
		}
	}

	signal := SigExecutionDone
	output := result.Stdout
	if !result.Success() {
		signal = SigExecutionFailed
		output = fmt.Sprintf("exit %d\nstdout: %s\nstderr: %s",
			result.ExitCode,
			llm.Truncate(result.Stdout, 2000),
			llm.Truncate(result.Stderr, 2000),
		)
	}

	return core.Result{
		CommandName: c.Name(),
		Signal:      signal,
		Output:      output,
		Cost:        core.Cost{Duration: result.Duration},
	}
}

// ExecuteTaskBuilder constructs execute_task commands.
type ExecuteTaskBuilder struct {
	PS *State
}

func (b *ExecuteTaskBuilder) Build(_ core.Result) core.Command {
	return &executeTaskCmd{ps: b.PS}
}

// --- check_result ---

type checkResultCmd struct {
	ps          *State
	prevRes     core.Result
	snapshot    pipelineSnapshot
	hasSnapshot bool
}

func (c *checkResultCmd) Name() string { return "check_result" }
func (c *checkResultCmd) Undo() core.Result {
	return undoPipelineSnapshot(c.Name(), c.ps, c.snapshot, c.hasSnapshot)
}

func (c *checkResultCmd) UndoMemento() (core.UndoMemento, error) {
	return mementoPipelineSnapshot(c.Name(), c.snapshot, c.hasSnapshot, core.UndoMementoReversible)
}

func (c *checkResultCmd) Execute() core.Result {
	c.snapshot = snapshotPipelineState(c.ps)
	c.hasSnapshot = true
	if c.prevRes.Signal == core.ToolFailed || c.prevRes.Signal == core.CommandError {
		c.ps.retryCount++
		maxRetries := c.ps.MaxRetries
		if maxRetries <= 0 {
			maxRetries = 2
		}

		if c.ps.retryCount >= maxRetries {
			c.ps.Tracer.Event("pipeline.retries_exhausted",
				attribute.String("task.id", c.ps.currentTaskID()),
				attribute.Int("retries", c.ps.retryCount),
			)
			return core.Result{
				CommandName: c.Name(),
				Signal:      SigRetriesExhausted,
				Output:      fmt.Sprintf("retries exhausted (%d/%d): %s", c.ps.retryCount, maxRetries, c.prevRes.Output),
			}
		}

		c.ps.Tracer.Event("pipeline.retry_available",
			attribute.String("task.id", c.ps.currentTaskID()),
			attribute.Int("retry", c.ps.retryCount),
			attribute.Int("max_retries", maxRetries),
		)
		return core.Result{
			CommandName: c.Name(),
			Signal:      SigRetryAvailable,
			Output:      fmt.Sprintf("retry %d/%d: %s", c.ps.retryCount, maxRetries, c.prevRes.Output),
		}
	}

	if c.ps.CurrentTask != nil && c.ps.Graph != nil {
		for _, nid := range c.ps.CurrentTask.NodeIDs {
			if n, _ := c.ps.Graph.Node(nid); n != nil {
				_ = n.MarkDone()
			}
		}
		c.ps.Tracer.Event("pipeline.task_completed",
			attribute.String("task.id", c.ps.CurrentTask.ID),
			attribute.Int("remaining", len(c.ps.Graph.Ready())),
		)
	}

	remaining := len(c.ps.Graph.Ready())
	msg := fmt.Sprintf("task completed; %d tasks remaining", remaining)
	if remaining == 0 {
		msg = "all tasks completed"
	}

	return core.Result{
		CommandName: c.Name(),
		Signal:      core.ValidationPassed,
		Output:      msg,
	}
}

// CheckResultBuilder constructs check_result commands.
type CheckResultBuilder struct {
	PS *State
}

func (b *CheckResultBuilder) Build(res core.Result) core.Command {
	return &checkResultCmd{ps: b.PS, prevRes: res}
}

// --- PlannerAssembler ---

// PlannerAssembler implements llm.PromptAssembler for the planning
// pipeline. Unlike the generator's DefaultAssembler which uses a
// prompt.Prompt, the planner assembler sends the planning prompt
// (from assemble_prompt) as the sole user message.
type PlannerAssembler struct{}

func (a *PlannerAssembler) AssembleMessages(conv *llm.Conversation, _ *core.Registry, _ core.State) []llm.Message {
	var messages []llm.Message

	systemPrompt := strings.Join([]string{
		"You are an implementation planner for a Go software project.",
		"Given a task description with requirements and SRD context,",
		"produce an implementation plan in YAML format.",
		"The plan must include: title, files (path + action), requirements,",
		"design_decisions (optional), and acceptance_criteria.",
	}, " ")

	messages = append(messages, llm.Message{Role: llm.System, Content: systemPrompt})
	messages = append(messages, conv.Messages()...)

	return messages
}

var _ llm.PromptAssembler = (*PlannerAssembler)(nil)

// marshalPipelineTask serializes pipeline task info for tracing.
func marshalPipelineTask(task *extract.Task, issueID string) string {
	type taskJSON struct {
		TaskID  string `json:"task_id"`
		SRDID   string `json:"srd_id"`
		Weight  int    `json:"weight"`
		Nodes   int    `json:"nodes"`
		IssueID string `json:"issue_id,omitempty"`
	}
	j := taskJSON{
		TaskID:  task.ID,
		SRDID:   task.SRDID,
		Weight:  task.Weight,
		Nodes:   len(task.NodeIDs),
		IssueID: issueID,
	}
	data, _ := json.Marshal(j)
	return string(data)
}
