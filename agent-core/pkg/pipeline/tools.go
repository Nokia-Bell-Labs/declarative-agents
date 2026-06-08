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

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/execute"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/extract"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/graph"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/materialize"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/plan"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/spec"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
)

// Pipeline signals emitted by pipeline tools.
const (
	SigTaskExtracted core.Signal = "TaskExtracted"
	SigNoMoreTasks   core.Signal = "NoMoreTasks"
	SigPromptReady   core.Signal = "PromptReady"
	SigPlanParsed    core.Signal = "PlanParsed"
	SigIssueCreated  core.Signal = "IssueCreated"
	SigTaskExecuted  core.Signal = "TaskExecuted"
	SigCheckPassed   core.Signal = "CheckPassed"
	SigCheckFailed   core.Signal = "CheckFailed"
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

	ExecConfig execute.Config
	Ctx        context.Context
}

// --- extract_task ---

type extractTaskCmd struct {
	ps *State
}

func (c *extractTaskCmd) Name() string { return "extract_task" }

func (c *extractTaskCmd) Execute() core.Result {
	task := c.ps.Extractor.ExtractNext(c.ps.Graph, c.ps.MaxWeight)
	if task == nil {
		return core.Result{
			CommandName: c.Name(),
			Signal:      SigNoMoreTasks,
			Output:      "no more tasks available",
		}
	}

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
	ps *State
}

func (c *extractAllCmd) Name() string { return "extract_all" }

func (c *extractAllCmd) Execute() core.Result {
	ready := c.ps.Graph.Ready()
	if len(ready) == 0 {
		return core.Result{
			CommandName: c.Name(),
			Signal:      SigNoMoreTasks,
			Output:      "no ready nodes in graph",
		}
	}

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

func (c *assemblePromptCmd) Name() string { return "assemble_prompt" }

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
		Signal:      SigPromptReady,
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
	ps      *State
	rawResp string
}

func (c *parsePlanCmd) Name() string { return "parse_plan" }

func (c *parsePlanCmd) Execute() core.Result {
	p, err := plan.ParsePlan(c.rawResp)
	if err != nil {
		c.ps.Tracer.Event("pipeline.parse_plan_failed",
			attribute.String("error", err.Error()),
		)
		return core.Result{
			CommandName: c.Name(),
			Signal:      core.ParseFailed,
			Output:      err.Error(),
		}
	}

	c.ps.CurrentPlan = &p
	c.ps.Tracer.Event("pipeline.plan_parsed",
		attribute.String("plan.title", p.Title),
		attribute.Int("plan.files", len(p.Files)),
		attribute.Int("plan.requirements", len(p.Requirements)),
	)

	return core.Result{
		CommandName: c.Name(),
		Signal:      SigPlanParsed,
		Output:      fmt.Sprintf("parsed plan: %s (%d files, %d requirements)", p.Title, len(p.Files), len(p.Requirements)),
	}
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
	ps *State
}

func (c *createIssueCmd) Name() string { return "create_issue" }

func (c *createIssueCmd) Execute() core.Result {
	if c.ps.CurrentPlan == nil {
		return core.Result{
			CommandName: c.Name(),
			Signal:      core.CommandError,
			Output:      "no current plan to materialize",
		}
	}

	m := materialize.NewMaterializeTask()
	issueID, err := m.Execute(c.ps.Ctx, c.ps.Tracer, *c.ps.CurrentPlan, c.ps.Directory, c.ps.TaskDeps)
	if err != nil {
		return core.Result{
			CommandName: c.Name(),
			Signal:      core.CommandError,
			Err:         err,
			Output:      err.Error(),
		}
	}

	c.ps.IssueID = issueID
	if c.ps.CurrentTask != nil {
		if c.ps.TaskDeps == nil {
			c.ps.TaskDeps = make(map[string]string)
		}
		c.ps.TaskDeps[c.ps.CurrentTask.ID] = issueID
	}

	return core.Result{
		CommandName: c.Name(),
		Signal:      SigIssueCreated,
		Output:      fmt.Sprintf("created issue %s for plan %q", issueID, c.ps.CurrentPlan.Title),
	}
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

	signal := SigTaskExecuted
	output := result.Stdout
	if !result.Success() {
		signal = core.ToolFailed
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
	ps      *State
	prevRes core.Result
}

func (c *checkResultCmd) Name() string { return "check_result" }

func (c *checkResultCmd) Execute() core.Result {
	if c.prevRes.Signal == core.ToolFailed || c.prevRes.Signal == core.CommandError {
		return core.Result{
			CommandName: c.Name(),
			Signal:      SigCheckFailed,
			Output:      fmt.Sprintf("previous step failed: %s", c.prevRes.Output),
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
		Signal:      SigCheckPassed,
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
