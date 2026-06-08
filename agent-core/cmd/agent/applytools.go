// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/materialize"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/plan"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/worktree"
)

// applyInputTask is one entry in the tasks.yaml input file.
type applyInputTask struct {
	ID    string          `yaml:"id"`
	SRDID string          `yaml:"srd_id"`
	Items []plan.TaskItem `yaml:"items"`
	SRD   plan.SRDContext `yaml:"srd"`
	Deps  []string        `yaml:"deps,omitempty"`
}

// applyTasksFile is the top-level structure of a tasks.yaml file.
type applyTasksFile struct {
	Tasks []applyInputTask `yaml:"tasks"`
}

// applyState holds shared mutable state for the apply state machine tools.
type applyState struct {
	directory    string
	tasksPath    string
	runID        string
	ctx          context.Context
	tracer       tracing.Tracer
	client       llm.Client
	conversation *llm.Conversation
	assembler    llm.PromptAssembler
	model        string
	numCtx       int

	tasks    []applyInputTask
	cursor   int
	current  *applyInputTask
	wt       *worktree.Worktree
	lastPlan plan.ImplementationPlan
	issued   map[string]string
	failures int
}

// loadApplyTasksBuilder loads the tasks file.
type loadApplyTasksBuilder struct {
	as *applyState
}

func (b *loadApplyTasksBuilder) Build(_ core.Result) core.Command {
	return &loadApplyTasksCmd{as: b.as}
}

type loadApplyTasksCmd struct {
	as *applyState
}

func (c *loadApplyTasksCmd) Name() string { return "load_apply_tasks" }
func (c *loadApplyTasksCmd) Execute() core.Result {
	data, err := os.ReadFile(c.as.tasksPath)
	if err != nil {
		return core.Result{
			Signal:      core.CommandError,
			Err:         err,
			Output:      fmt.Sprintf("read tasks file: %v", err),
			CommandName: "load_apply_tasks",
		}
	}
	var tf applyTasksFile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		return core.Result{
			Signal:      core.CommandError,
			Err:         err,
			Output:      fmt.Sprintf("parse tasks file: %v", err),
			CommandName: "load_apply_tasks",
		}
	}
	if len(tf.Tasks) == 0 {
		return core.Result{
			Signal:      core.CommandError,
			Err:         fmt.Errorf("tasks file contains no tasks"),
			Output:      "tasks file contains no tasks",
			CommandName: "load_apply_tasks",
		}
	}
	c.as.tasks = tf.Tasks
	c.as.cursor = 0
	c.as.issued = make(map[string]string)
	if c.as.runID == "" {
		c.as.runID = fmt.Sprintf("%d", time.Now().Unix())
	}
	return core.Result{
		Signal:      core.ToolDone,
		Output:      fmt.Sprintf("loaded %d tasks", len(tf.Tasks)),
		CommandName: "load_apply_tasks",
	}
}

// createWorktreeBuilder creates a git worktree.
type createWorktreeBuilder struct {
	as *applyState
}

func (b *createWorktreeBuilder) Build(_ core.Result) core.Command {
	return &createWorktreeCmd{as: b.as}
}

type createWorktreeCmd struct {
	as *applyState
}

func (c *createWorktreeCmd) Name() string { return "create_worktree" }
func (c *createWorktreeCmd) Execute() core.Result {
	wt, err := worktree.Create(c.as.ctx, c.as.tracer, c.as.directory, c.as.runID)
	if err != nil {
		return core.Result{
			Signal:      core.CommandError,
			Err:         err,
			Output:      fmt.Sprintf("create worktree: %v", err),
			CommandName: "create_worktree",
		}
	}
	c.as.wt = wt
	return core.Result{
		Signal:      core.ToolDone,
		Output:      fmt.Sprintf("worktree created at %s", wt.Dir),
		CommandName: "create_worktree",
	}
}

// nextApplyTaskBuilder advances to the next task or signals AllDone.
type nextApplyTaskBuilder struct {
	as *applyState
}

func (b *nextApplyTaskBuilder) Build(_ core.Result) core.Command {
	return &nextApplyTaskCmd{as: b.as}
}

type nextApplyTaskCmd struct {
	as *applyState
}

func (c *nextApplyTaskCmd) Name() string { return "next_apply_task" }
func (c *nextApplyTaskCmd) Execute() core.Result {
	if c.as.cursor >= len(c.as.tasks) {
		summary := fmt.Sprintf("all %d tasks processed", len(c.as.tasks))
		if c.as.failures > 0 {
			summary = fmt.Sprintf("%d of %d tasks failed", c.as.failures, len(c.as.tasks))
		}
		if c.as.wt != nil {
			_ = c.as.wt.Remove(c.as.ctx, c.as.tracer)
		}
		return core.Result{
			Signal:      "AllDone",
			Output:      summary,
			CommandName: "next_apply_task",
		}
	}
	task := &c.as.tasks[c.as.cursor]
	c.as.current = task
	c.as.cursor++
	return core.Result{
		Signal:      "TaskExtracted",
		Output:      fmt.Sprintf("task %d/%d: %s", c.as.cursor, len(c.as.tasks), task.ID),
		CommandName: "next_apply_task",
	}
}

// assembleApplyPromptBuilder builds the planning prompt for the current task.
type assembleApplyPromptBuilder struct {
	as *applyState
}

func (b *assembleApplyPromptBuilder) Build(_ core.Result) core.Command {
	return &assembleApplyPromptCmd{as: b.as}
}

type assembleApplyPromptCmd struct {
	as *applyState
}

func (c *assembleApplyPromptCmd) Name() string { return "assemble_apply_prompt" }
func (c *assembleApplyPromptCmd) Execute() core.Result {
	task := c.as.current
	tc := plan.TaskContext{ID: task.ID, SRDID: task.SRDID, Items: task.Items}
	p, err := plan.AssemblePrompt(tc, task.SRD, nil, nil)
	if err != nil {
		return core.Result{
			Signal:      core.CommandError,
			Err:         err,
			Output:      fmt.Sprintf("assemble prompt: %v", err),
			CommandName: "assemble_apply_prompt",
		}
	}
	c.as.conversation.Reset()
	c.as.conversation.Append(llm.Message{Role: llm.User, Content: p})
	return core.Result{
		Signal:      core.ToolDone,
		Output:      fmt.Sprintf("prompt assembled for task %s (%d chars)", task.ID, len(p)),
		CommandName: "assemble_apply_prompt",
	}
}

// parseApplyPlanBuilder parses the LLM response as an implementation plan.
type parseApplyPlanBuilder struct {
	as *applyState
}

func (b *parseApplyPlanBuilder) Build(res core.Result) core.Command {
	return &parseApplyPlanCmd{as: b.as, llmOutput: res.Output}
}

type parseApplyPlanCmd struct {
	as        *applyState
	llmOutput string
}

func (c *parseApplyPlanCmd) Name() string { return "parse_apply_plan" }
func (c *parseApplyPlanCmd) Execute() core.Result {
	c.as.conversation.Append(llm.Message{Role: llm.Assistant, Content: c.llmOutput})
	result, err := plan.ParsePlan(c.llmOutput)
	if err != nil {
		c.as.failures++
		return core.Result{
			Signal:      "PlanFailed",
			Output:      fmt.Sprintf("parse plan for %s: %v", c.as.current.ID, err),
			CommandName: "parse_apply_plan",
		}
	}
	c.as.lastPlan = result
	return core.Result{
		Signal:      "PlanReady",
		Output:      fmt.Sprintf("plan parsed for %s: %d files", c.as.current.ID, len(result.Files)),
		CommandName: "parse_apply_plan",
	}
}

// materializeIssueBuilder materializes the plan as a bd issue.
type materializeIssueBuilder struct {
	as *applyState
}

func (b *materializeIssueBuilder) Build(_ core.Result) core.Command {
	return &materializeIssueCmd{as: b.as}
}

type materializeIssueCmd struct {
	as *applyState
}

func (c *materializeIssueCmd) Name() string { return "materialize_issue" }
func (c *materializeIssueCmd) Execute() core.Result {
	task := c.as.current
	taskDeps := make(map[string]string)
	for _, dep := range task.Deps {
		if id, ok := c.as.issued[dep]; ok {
			taskDeps[dep] = id
		}
	}

	mt := materialize.NewMaterializeTask()
	issueID, err := mt.Execute(c.as.ctx, c.as.tracer, c.as.lastPlan, c.as.wt.Dir, taskDeps)
	if err != nil {
		c.as.failures++
		return core.Result{
			Signal:      "MaterializeFailed",
			Output:      fmt.Sprintf("materialize %s: %v", task.ID, err),
			CommandName: "materialize_issue",
		}
	}
	c.as.issued[task.ID] = issueID
	return core.Result{
		Signal:      "Materialized",
		Output:      fmt.Sprintf("task %s → %s", task.ID, issueID),
		CommandName: "materialize_issue",
	}
}
