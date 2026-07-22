// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/extract"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/graph"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/plan"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

const validRawPlan = `title: Implement config parser
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

func TestExtractTaskBuilder_UndoRestoresPipelineState(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)
	ps.retryCount = 3

	builder := &ExtractTaskBuilder{PS: ps}
	cmd := builder.Build(core.Result{})
	result := cmd.Execute()
	require.Equal(t, SigTaskExtracted, result.Signal)
	require.NotNil(t, ps.CurrentTask)
	require.Equal(t, 0, ps.retryCount)

	undo := cmd.Undo(core.Result{})
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Nil(t, ps.CurrentTask)
	require.Equal(t, 3, ps.retryCount)
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

func TestExtractAllBuilder_UndoRestoresPipelineState(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)
	ps.retryCount = 4

	builder := &ExtractAllBuilder{PS: ps}
	cmd := builder.Build(core.Result{})
	result := cmd.Execute()
	require.Equal(t, SigTaskExtracted, result.Signal)
	require.NotNil(t, ps.CurrentTask)
	require.Equal(t, 0, ps.retryCount)

	undo := cmd.Undo(core.Result{})
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Nil(t, ps.CurrentTask)
	require.Equal(t, 4, ps.retryCount)
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

	builder := &ParsePlanBuilder{PS: ps}
	cmd := builder.Build(core.Result{Output: validRawPlan})
	result := cmd.Execute()

	assert.Equal(t, SigPlanReady, result.Signal)
	assert.NotNil(t, ps.CurrentPlan)
	assert.Equal(t, "Implement config parser", ps.CurrentPlan.Title)
}

func TestParsePlanBuilder_UndoRestoresPreviousPlan(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)
	previous := &plan.ImplementationPlan{Title: "previous"}
	ps.CurrentPlan = previous

	builder := &ParsePlanBuilder{PS: ps}
	cmd := builder.Build(core.Result{Output: validRawPlan})
	result := cmd.Execute()
	require.Equal(t, SigPlanReady, result.Signal)
	require.Equal(t, "Implement config parser", ps.CurrentPlan.Title)

	undo := cmd.Undo(core.Result{})
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, "previous", ps.CurrentPlan.Title)
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
	// The extract and execute phases advance the nodes before check runs; mirror
	// that so check_result marks them Done from Executing (GH-507).
	for _, id := range task.NodeIDs {
		n, ok := ps.Graph.Node(id)
		require.True(t, ok)
		require.NoError(t, n.MarkPlanning())
		require.NoError(t, n.MarkExecuting())
	}

	builder := &CheckResultBuilder{PS: ps}
	cmd := builder.Build(core.Result{Signal: core.ToolDone})
	result := cmd.Execute()

	assert.Equal(t, core.ValidationPassed, result.Signal)
	assert.Contains(t, result.Output, "completed")
	for _, id := range task.NodeIDs {
		n, _ := ps.Graph.Node(id)
		assert.Equal(t, graph.Done, n.Status)
	}
}

func TestCheckResultBuilder_UndoRestoresGraphStatusAfterPass(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)

	task := ps.Extractor.ExtractNext(ps.Graph, ps.MaxWeight)
	require.NotNil(t, task)
	ps.CurrentTask = task
	for _, id := range task.NodeIDs {
		n, ok := ps.Graph.Node(id)
		require.True(t, ok)
		require.NoError(t, n.MarkPlanning())
		require.NoError(t, n.MarkExecuting())
	}

	builder := &CheckResultBuilder{PS: ps}
	cmd := builder.Build(core.Result{Signal: core.ToolDone})
	result := cmd.Execute()
	require.Equal(t, core.ValidationPassed, result.Signal)
	for _, id := range task.NodeIDs {
		n, _ := ps.Graph.Node(id)
		require.Equal(t, graph.Done, n.Status)
	}

	undo := cmd.Undo(core.Result{})
	require.Equal(t, core.ToolDone, undo.Signal)
	for _, id := range task.NodeIDs {
		n, _ := ps.Graph.Node(id)
		require.Equal(t, graph.Executing, n.Status)
	}
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

func TestCheckResultBuilder_UndoRestoresRetryCount(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)
	ps.retryCount = 0

	builder := &CheckResultBuilder{PS: ps}
	cmd := builder.Build(core.Result{Signal: core.ToolFailed, Output: "build error"})
	result := cmd.Execute()
	require.Equal(t, SigRetryAvailable, result.Signal)
	require.Equal(t, 1, ps.retryCount)

	undo := cmd.Undo(core.Result{})
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, 0, ps.retryCount)
}

func TestCreateIssueBuilder_UndoRestoresIssueState(t *testing.T) {
	ps := minimalState(t)
	task := ps.Extractor.ExtractNext(ps.Graph, ps.MaxWeight)
	require.NotNil(t, task)
	ps.CurrentTask = task
	ps.CurrentPlan = &plan.ImplementationPlan{Title: "plan"}
	ps.IssueID = "old-issue"
	ps.TaskDeps = map[string]string{"old-task": "old-issue"}

	prevMaterializePlan := materializePlan
	materializePlan = func(context.Context, tracing.Tracer, plan.ImplementationPlan, string, map[string]string, string) (string, core.Result) {
		return "new-issue", core.Result{Signal: SigMaterialized, Output: "created issue"}
	}
	t.Cleanup(func() { materializePlan = prevMaterializePlan })

	builder := &CreateIssueBuilder{PS: ps}
	cmd := builder.Build(core.Result{})
	result := cmd.Execute()
	require.Equal(t, SigMaterialized, result.Signal)
	require.Equal(t, "new-issue", ps.IssueID)
	require.Equal(t, "new-issue", ps.TaskDeps[task.ID])

	undo := cmd.Undo(core.Result{})
	require.Equal(t, core.ToolDone, undo.Signal)
	require.Equal(t, "old-issue", ps.IssueID)
	require.Equal(t, map[string]string{"old-task": "old-issue"}, ps.TaskDeps)
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

func TestRegisterFactoriesExecuteTaskRequiresChildConfig(t *testing.T) {
	t.Parallel()
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{Ctx: context.Background()})

	factory, ok := br.Resolve("execute_task")
	require.True(t, ok)

	_, err := factory(catalog.ToolDef{Name: "execute_task", Init: "execute_task"}, nil)
	require.ErrorContains(t, err, "requires profile")
}

func TestRegisterFactoriesExecuteTaskAcceptsProfileConfig(t *testing.T) {
	t.Parallel()
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{Ctx: context.Background()})

	factory, ok := br.Resolve("execute_task")
	require.True(t, ok)

	builder, err := factory(catalog.ToolDef{
		Name: "execute_task",
		Init: "execute_task",
		Config: map[string]interface{}{
			"profile": "agents/executor/profile.yaml",
		},
	}, nil)
	require.NoError(t, err)

	execBuilder, ok := builder.(*ExecuteTaskBuilder)
	require.True(t, ok)
	require.Equal(t, "agents/executor/profile.yaml", execBuilder.PS.ExecConfig.Profile)
}

var _ llm.PromptAssembler = (*PlannerAssembler)(nil)

// Ensure unused plan import is consumed (types used in assertions above).
var _ plan.ImplementationPlan

// TestPlannerNodeLifecycleAdvancesAndDoesNotRepeat proves the GH-507 fix: a ready
// node is selected once, advances Pending -> Planning -> Executing -> Done across
// the extract/execute/check phases, leaves Graph.Ready as soon as it is selected,
// and is never re-selected once complete.
func TestPlannerNodeLifecycleAdvancesAndDoesNotRepeat(t *testing.T) {
	t.Parallel()
	ps := minimalState(t)
	require.Len(t, ps.Graph.Ready(), 1, "one node ready at start")
	nid := ps.Graph.Ready()[0].ID

	// Extract selects the node and marks it Planning, so it leaves Ready.
	extract := (&ExtractTaskBuilder{PS: ps}).Build(core.Result{})
	res := extract.Execute()
	require.Equal(t, SigTaskExtracted, res.Signal, res.Output)
	n, _ := ps.Graph.Node(nid)
	require.Equal(t, graph.Planning, n.Status)
	require.Empty(t, ps.Graph.Ready(), "selected node must not stay ready")

	// Execute advances it to Executing (the real execute runs a child; here we
	// drive the phase transition the executor owns).
	require.NoError(t, ps.advanceTaskNodesTo(graph.Executing))
	require.Equal(t, graph.Executing, n.Status)

	// Check marks it Done on success.
	check := (&CheckResultBuilder{PS: ps}).Build(core.Result{Signal: core.ToolDone})
	res = check.Execute()
	require.Equal(t, core.ValidationPassed, res.Signal, res.Output)
	require.Equal(t, graph.Done, n.Status)

	// A completed node is never re-selected: the next extract finds no work.
	res = (&ExtractTaskBuilder{PS: ps}).Build(core.Result{}).Execute()
	require.NotEqual(t, SigTaskExtracted, res.Signal, "completed node must not be re-extracted")
}
