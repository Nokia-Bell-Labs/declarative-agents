// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/extract"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/graph"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/plan"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/spec"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
)

func minimalState(t *testing.T) *State {
	t.Helper()
	corpus := &spec.Corpus{
		SRDs: map[string]spec.SRD{
			"srd001": {
				ID:      "srd001",
				Title:   "Test SRD",
				Problem: "Implement a thing",
				Goals:   []string{"Make it work"},
				Requirements: map[string]spec.RequirementGroup{
					"R1": {
						Title: "Core",
						Items: []spec.RequirementItem{
							{ID: "R1.1", Text: "Create the config parser", Weight: 1},
						},
					},
				},
				OrderedGroups: []string{"R1"},
				AcceptanceCriteria: []spec.AcceptanceCriterion{
					{ID: "AC1", Criterion: "It compiles"},
				},
			},
		},
		SRDOrder: []string{"srd001"},
	}

	g, err := graph.BuildGraph(corpus)
	require.NoError(t, err)

	return &State{
		Graph:     g,
		Corpus:    corpus,
		Extractor: extract.NewExtractor(),
		MaxWeight: 10,
		Tracer:    tracing.NoopTracer{},
		TaskDeps:  make(map[string]string),
		Directory: t.TempDir(),
		Ctx:       context.Background(),
	}
}

func TestExtractTaskBuilder_ExtractsTask(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)
	builder := &ExtractTaskBuilder{PS: ps}

	cmd := builder.Build(core.Result{})
	result := cmd.Execute()

	assert.Equal(t, SigTaskExtracted, result.Signal)
	assert.NotNil(t, ps.CurrentTask)
	assert.Contains(t, result.Output, "extracted task")
}

func TestExtractTaskBuilder_NoMoreTasks(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)

	for _, n := range ps.Graph.Nodes() {
		_ = n.MarkPlanning()
		_ = n.MarkExecuting()
		_ = n.MarkDone()
	}

	builder := &ExtractTaskBuilder{PS: ps}
	cmd := builder.Build(core.Result{})
	result := cmd.Execute()

	assert.Equal(t, SigAllDone, result.Signal)
}

func TestExtractAllBuilder_ExtractsAllReady(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)
	builder := &ExtractAllBuilder{PS: ps}

	cmd := builder.Build(core.Result{})
	result := cmd.Execute()

	assert.Equal(t, SigTaskExtracted, result.Signal)
	assert.NotNil(t, ps.CurrentTask)
	assert.Contains(t, result.Output, "extracted all")
}

func TestExtractAllBuilder_NoReady(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)
	for _, n := range ps.Graph.Nodes() {
		_ = n.MarkPlanning()
		_ = n.MarkExecuting()
		_ = n.MarkDone()
	}

	builder := &ExtractAllBuilder{PS: ps}
	cmd := builder.Build(core.Result{})
	result := cmd.Execute()

	assert.Equal(t, SigAllDone, result.Signal)
}

func TestAssemblePromptBuilder_ProducesPrompt(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)

	task := ps.Extractor.ExtractNext(ps.Graph, ps.MaxWeight)
	require.NotNil(t, task)
	ps.CurrentTask = task

	builder := &AssemblePromptBuilder{PS: ps}
	cmd := builder.Build(core.Result{})
	result := cmd.Execute()

	assert.Equal(t, core.ToolDone, result.Signal)
	assert.Contains(t, result.Output, "Implementation Planning")
}

func TestAssemblePromptBuilder_NoTask(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)

	builder := &AssemblePromptBuilder{PS: ps}
	cmd := builder.Build(core.Result{})
	result := cmd.Execute()

	assert.Equal(t, core.CommandError, result.Signal)
}

func TestParsePlanBuilder_ValidYAML(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)

	rawPlan := `title: Implement config parser
files:
  - path: config.go
    action: create
requirements:
  - id: R1
    text: Define struct
acceptance_criteria:
  - id: AC1
    text: Struct exists
`
	builder := &ParsePlanBuilder{PS: ps}
	cmd := builder.Build(core.Result{Output: rawPlan})
	result := cmd.Execute()

	assert.Equal(t, SigPlanReady, result.Signal)
	assert.NotNil(t, ps.CurrentPlan)
	assert.Equal(t, "Implement config parser", ps.CurrentPlan.Title)
}

func TestParsePlanBuilder_InvalidYAML(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)

	builder := &ParsePlanBuilder{PS: ps}
	cmd := builder.Build(core.Result{Output: "not: [valid yaml"})
	result := cmd.Execute()

	assert.Equal(t, core.ParseFailed, result.Signal)
	assert.Nil(t, ps.CurrentPlan)
}

func TestCheckResultBuilder_Pass(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)

	task := ps.Extractor.ExtractNext(ps.Graph, ps.MaxWeight)
	require.NotNil(t, task)
	ps.CurrentTask = task

	builder := &CheckResultBuilder{PS: ps}
	cmd := builder.Build(core.Result{Signal: core.ToolDone})
	result := cmd.Execute()

	assert.Equal(t, core.ValidationPassed, result.Signal)
	assert.Contains(t, result.Output, "completed")
}

func TestCheckResultBuilder_Fail(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)

	builder := &CheckResultBuilder{PS: ps}
	cmd := builder.Build(core.Result{Signal: core.ToolFailed, Output: "build error"})
	result := cmd.Execute()

	assert.Equal(t, SigRetryAvailable, result.Signal)
	assert.Contains(t, result.Output, "retry")
}

func TestPlannerAssembler_PrependsSystem(t *testing.T) {
	t.Parallel()

	conv := llm.NewConversation(nil, "", llm.ChatOptions{})
	conv.Append(llm.Message{Role: llm.User, Content: "plan this"})

	asm := &PlannerAssembler{}
	msgs := asm.AssembleMessages(conv, nil, "")

	require.Len(t, msgs, 2)
	assert.Equal(t, llm.System, msgs[0].Role)
	assert.Contains(t, msgs[0].Content, "implementation planner")
	assert.Equal(t, llm.User, msgs[1].Role)
	assert.Equal(t, "plan this", msgs[1].Content)
}

func TestMarshalPipelineTask(t *testing.T) {
	t.Parallel()
	task := &extract.Task{
		ID:      "test-1",
		SRDID:   "srd001",
		Weight:  5,
		NodeIDs: []string{"n1", "n2"},
	}

	result := marshalPipelineTask(task, "issue-abc")
	assert.Contains(t, result, `"task_id":"test-1"`)
	assert.Contains(t, result, `"issue_id":"issue-abc"`)
	assert.Contains(t, result, `"nodes":2`)
}

// Verify that the pipeline state struct matches the test helper's expectations.
func TestMinimalState_GraphHasNodes(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)

	nodes := ps.Graph.Nodes()
	assert.Greater(t, len(nodes), 0, "minimal corpus should produce at least one graph node")

	ready := ps.Graph.Ready()
	assert.Greater(t, len(ready), 0, "minimal corpus should have ready nodes")
}

var _ llm.PromptAssembler = (*PlannerAssembler)(nil)

// Ensure unused plan import is consumed (types used in assertions above).
var _ plan.ImplementationPlan
