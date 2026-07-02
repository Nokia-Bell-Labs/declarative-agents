// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/extract"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/plan"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

type assemblePromptCmd struct {
	ps *State
}

func (c *assemblePromptCmd) Name() string      { return "assemble_prompt" }
func (c *assemblePromptCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

func (c *assemblePromptCmd) Execute() core.Result {
	task := c.ps.CurrentTask
	if task == nil {
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Output: "no current task"}
	}
	srd, ok := c.ps.Corpus.SRDs[task.SRDID]
	if !ok {
		return core.Result{
			CommandName: c.Name(),
			Signal:      core.CommandError,
			Output:      fmt.Sprintf("SRD %q not found in corpus", task.SRDID),
		}
	}
	prompt, err := plan.AssemblePrompt(taskContext(c.ps, task), srdContext(srd), nil, nil)
	if err != nil {
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: err, Output: fmt.Sprintf("assemble prompt: %v", err)}
	}
	return core.Result{CommandName: c.Name(), Signal: core.ToolDone, Output: prompt}
}

func taskContext(ps *State, task *extract.Task) plan.TaskContext {
	var items []plan.TaskItem
	for _, nid := range task.NodeIDs {
		n, _ := ps.Graph.Node(nid)
		if n != nil {
			items = append(items, plan.TaskItem{ID: nid, Text: n.Text})
		}
	}
	return plan.TaskContext{ID: task.ID, SRDID: task.SRDID, Items: items}
}

func srdContext(srd spec.SRD) plan.SRDContext {
	ctx := plan.SRDContext{Problem: srd.Problem, Goals: srd.Goals}
	for _, ac := range srd.AcceptanceCriteria {
		ctx.AcceptanceCriteria = append(ctx.AcceptanceCriteria, ac.ID+": "+ac.Criterion)
	}
	return ctx
}

// AssemblePromptBuilder constructs assemble_prompt commands.
type AssemblePromptBuilder struct {
	PS *State
}

func (b *AssemblePromptBuilder) Build(_ core.Result) core.Command {
	return &assemblePromptCmd{ps: b.PS}
}

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
		c.ps.Tracer.Event("pipeline.parse_plan_failed", attribute.String("error", res.Output))
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

// PlannerAssembler implements llm.PromptAssembler for the planning pipeline.
type PlannerAssembler struct{}

func (a *PlannerAssembler) AssembleMessages(conv *llm.Conversation, _ *core.Registry, _ core.State) []llm.Message {
	systemPrompt := strings.Join([]string{
		"You are an implementation planner for a Go software project.",
		"Given a task description with requirements and SRD context,",
		"produce an implementation plan in YAML format.",
		"The plan must include: title, files (path + action), requirements,",
		"design_decisions (optional), and acceptance_criteria.",
	}, " ")
	return append([]llm.Message{{Role: llm.System, Content: systemPrompt}}, conv.Messages()...)
}

var _ llm.PromptAssembler = (*PlannerAssembler)(nil)

// marshalPipelineTask serializes pipeline task info for tracing.
func marshalPipelineTask(task *extract.Task, issueID string) string {
	j := struct {
		TaskID  string `json:"task_id"`
		SRDID   string `json:"srd_id"`
		Weight  int    `json:"weight"`
		Nodes   int    `json:"nodes"`
		IssueID string `json:"issue_id,omitempty"`
	}{
		TaskID:  task.ID,
		SRDID:   task.SRDID,
		Weight:  task.Weight,
		Nodes:   len(task.NodeIDs),
		IssueID: issueID,
	}
	data, _ := json.Marshal(j)
	return string(data)
}
