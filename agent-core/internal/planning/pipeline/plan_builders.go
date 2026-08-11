// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
)

type projectPlannerContextCmd struct {
	ps *State
}

func (c *projectPlannerContextCmd) Name() string { return "project_planner_context" }
func (c *projectPlannerContextCmd) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.Name())
}

func (c *projectPlannerContextCmd) Execute() core.Result {
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
	context := map[string]string{
		"task_id": task.ID,
		"srd_id":  task.SRDID,
		"problem": srd.Problem,
		"goals":   bulletList(srd.Goals),
	}
	var items []string
	for _, nid := range task.NodeIDs {
		if n, _ := c.ps.Graph.Node(nid); n != nil {
			items = append(items, n.ID+": "+n.Text)
		}
	}
	context["items"] = bulletList(items)
	var criteria []string
	for _, criterion := range srd.AcceptanceCriteria {
		criteria = append(criteria, criterion.ID+": "+criterion.Criterion)
	}
	context["acceptance_criteria"] = bulletList(criteria)
	output, err := json.Marshal(context)
	if err != nil {
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: err, Output: fmt.Sprintf("project planner context: %v", err)}
	}
	return core.Result{CommandName: c.Name(), Signal: SigPlannerContextProjected, Output: string(output)}
}

func bulletList(items []string) string {
	if len(items) == 0 {
		return "- None"
	}
	return "- " + strings.Join(items, "\n- ")
}

// ProjectPlannerContextBuilder constructs prompt-neutral context projections.
type ProjectPlannerContextBuilder struct {
	PS *State
}

func (b *ProjectPlannerContextBuilder) Build(_ core.Result) core.Command {
	return &projectPlannerContextCmd{ps: b.PS}
}

type capturePlannerFailureCmd struct {
	failure string
}

func (c *capturePlannerFailureCmd) Name() string { return "capture_planner_failure" }
func (c *capturePlannerFailureCmd) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.Name())
}

func (c *capturePlannerFailureCmd) Execute() core.Result {
	output, err := json.Marshal(map[string]string{"output": c.failure})
	if err != nil {
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: err, Output: err.Error()}
	}
	return core.Result{CommandName: c.Name(), Signal: SigFailureCaptured, Output: string(output)}
}

// CapturePlannerFailureBuilder publishes a validation failure for retry prompts.
type CapturePlannerFailureBuilder struct{}

func (b *CapturePlannerFailureBuilder) Build(previous core.Result) core.Command {
	return &capturePlannerFailureCmd{failure: previous.Output}
}

type parsePlanCmd struct {
	ps      *State
	rawResp string
	retry   *toollm.ParseErrorRetryTracker
}

func (c *parsePlanCmd) Name() string { return "parse_plan" }
func (c *parsePlanCmd) Undo(prior core.Result) core.Result {
	return undoPipelineReceipt(c.Name(), c.ps, c.retry, prior.Receipt)
}

func (c *parsePlanCmd) Execute() (result core.Result) {
	snapshot := snapshotPipelineState(c.ps)
	var previousRetry *int
	if c.retry != nil {
		value := c.retry.Snapshot()
		previousRetry = &value
	}
	defer func() { result = withPipelineReceipt(result, snapshot, previousRetry) }()
	p, res := DoParsePlan(c.Name(), c.rawResp)
	c.retry.RecordParseResult(res.Signal)
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
	PS    *State
	Retry *toollm.ParseErrorRetryTracker
}

func (b *ParsePlanBuilder) Build(res core.Result) core.Command {
	return &parsePlanCmd{ps: b.PS, rawResp: res.Output, retry: b.Retry}
}

func (b *ParsePlanBuilder) BuildReverser() core.Command {
	return &parsePlanCmd{ps: b.PS, retry: b.Retry}
}
