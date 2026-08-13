// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestRunPointFactoryRequiresNestedConfig(t *testing.T) {
	factory := RunPointFactory(&EvalSessionState{})

	_, err := factory(catalog.ToolDef{Name: "run_point", Init: "run_point"}, nil)
	require.ErrorContains(t, err, "requires point_machine")

	_, err = factory(catalog.ToolDef{
		Name: "run_point",
		Init: "run_point",
		Config: map[string]interface{}{
			"point_machine": "agents/critic/point.yaml",
		},
	}, nil)
	require.ErrorContains(t, err, "requires point_tools")

	_, err = factory(catalog.ToolDef{
		Name: "run_point",
		Init: "run_point",
		Config: map[string]interface{}{
			"point_machine": "agents/critic/point.yaml",
			"point_tools":   "agents/critic/tools-point.yaml",
		},
	}, nil)
	require.ErrorContains(t, err, "requires point_tool_declarations")
}

func TestBuildPointRegistryUsesUnifiedBuiltinAndExecDeclarations(t *testing.T) {
	dir := t.TempDir()
	selection := filepath.Join(dir, "tools.yaml")
	declarations := filepath.Join(dir, "declarations.yaml")
	require.NoError(t, os.WriteFile(selection, []byte("tools:\n  - create_point_dir\n  - point_pwd\n"), 0o644))
	require.NoError(t, os.WriteFile(declarations, []byte(`tools:
  - name: create_point_dir
    type: builtin
    init: create_point_dir
    visibility: internal
  - name: point_pwd
    type: exec
    binary: pwd
    visibility: internal
`), 0o644))

	firstRoot := t.TempDir()
	es := &EvalState{PC: &PointContext{PointDir: firstRoot}}
	reg, err := buildPointRegistry(es, catalog.RunPointConfig{
		PointTools:            selection,
		PointToolDeclarations: []string{declarations},
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"create_point_dir", "point_pwd"}, reg.AllToolNames())

	builder, ok := reg.Resolve("point_pwd")
	require.True(t, ok)
	cmd := builder.Build(core.Result{})
	secondRoot := t.TempDir()
	es.PC.PointDir = secondRoot
	result := cmd.Execute()
	require.Equal(t, core.ToolDone, result.Signal, result.Output)
	require.Equal(t, secondRoot, strings.TrimSpace(result.Output))
}

func TestBuildPointRegistryRejectsUndeclaredSelection(t *testing.T) {
	dir := t.TempDir()
	selection := filepath.Join(dir, "tools.yaml")
	declarations := filepath.Join(dir, "declarations.yaml")
	require.NoError(t, os.WriteFile(selection, []byte("tools:\n  - absent\n"), 0o644))
	require.NoError(t, os.WriteFile(declarations, []byte("tools: []\n"), 0o644))

	_, err := buildPointRegistry(&EvalState{}, catalog.RunPointConfig{
		PointTools:            selection,
		PointToolDeclarations: []string{declarations},
	})
	require.ErrorContains(t, err, `tool "absent" is selected but not declared`)
}

func TestPointMachineRoutesFailureThroughMetricsBeforeRunPointReturns(t *testing.T) {
	t.Parallel()
	sessionDir := t.TempDir()
	machine := filepath.Join(t.TempDir(), "point.yaml")
	require.NoError(t, os.WriteFile(machine, []byte(`
name: failed-point
initial_state: Idle
states: [Idle, Running, RecordingFailure, CollectingMetrics, Failed]
terminal_states: [Failed]
signals: [Seed, CommandError, PointFailureRecorded, MetricsCollected]
transitions:
  - {state: Idle, signal: Seed, next: Running, action: fail_before_metrics}
  - {state: Running, signal: CommandError, next: RecordingFailure, action: record_point_failure}
  - {state: RecordingFailure, signal: PointFailureRecorded, next: CollectingMetrics, action: collect_metrics}
  - {state: CollectingMetrics, signal: MetricsCollected, next: Failed}
`), 0o600))

	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "fail_before_metrics"}, evalTestFailureBuilder{})
	var stderr bytes.Buffer
	recorder := tracing.NewRecordingTracer()
	pc := &PointContext{
		SessionDir: sessionDir,
		PointID:    "sample--executor--model--rep0",
		Sample:     Sample{Name: "sample"},
		Harness:    Harness{Name: "executor"},
		Model:      "model",
		Stderr:     &stderr,
	}
	pc.PointDir = filepath.Join(sessionDir, pc.PointID)
	require.NoError(t, os.MkdirAll(pc.PointDir, 0o755))
	es := &EvalSessionState{
		EvalState:    EvalState{PC: pc, Ctx: context.Background()},
		PointMachine: machine,
		Stderr:       &stderr,
		Tracer:       recorder,
	}
	reg.Register(core.ToolSpec{Name: "record_point_failure"}, &RecordPointFailureBuilder{ES: &es.EvalState})
	reg.Register(core.ToolSpec{Name: "collect_metrics"}, &CollectMetricsBuilder{ES: &es.EvalState})

	result := (&runPointCmd{
		es: es, pointRegistry: reg,
		config: catalog.RunPointConfig{
			PointMachine: machine, AgentName: "critic-point",
			MaxIterations: 20, SuccessState: "Done",
		},
	}).Execute()

	require.Equal(t, SigPointDone, result.Signal, result.Output)
	data, err := os.ReadFile(filepath.Join(pc.PointDir, ArtifactMeta))
	require.NoError(t, err)
	var meta EvalMeta
	require.NoError(t, json.Unmarshal(data, &meta))
	require.Equal(t, "sample", meta.Sample)
	require.Equal(t, "model", meta.Model)
	require.Equal(t, "fail_before_metrics", meta.FailureStage)
	require.Equal(t, "synthetic point failure", meta.FailureCause)
	require.False(t, meta.TestsPassed)
	require.Equal(t, 1, es.Result.TotalPoints)
	require.Equal(t, 1, es.Result.Failed)
	require.True(t, recordedSpanNamed(recorder.Spans, "execute_tool fail_before_metrics"))
}

func recordedSpanNamed(spans []tracing.RecordedSpan, name string) bool {
	for _, span := range spans {
		if span.Name == name {
			return true
		}
	}
	return false
}

func TestRunPointReturnsNestedLoopErrorsWithoutWritingMetadata(t *testing.T) {
	t.Parallel()
	pointDir := t.TempDir()
	pc := &PointContext{PointDir: pointDir, PointID: "point"}
	missingMachine := filepath.Join(pointDir, "missing.yaml")
	es := &EvalSessionState{
		EvalState:    EvalState{PC: pc, Ctx: context.Background()},
		PointMachine: missingMachine,
		Stderr:       &bytes.Buffer{},
	}

	result := (&runPointCmd{
		es: es, pointRegistry: core.NewRegistry(),
		config: catalog.RunPointConfig{PointMachine: missingMachine},
	}).Execute()

	require.Equal(t, core.CommandError, result.Signal)
	require.ErrorContains(t, result.Err, "nested point loop")
	require.NoFileExists(t, filepath.Join(pointDir, ArtifactMeta))
}

func TestRunPointBindsForEachPointFromCommandState(t *testing.T) {
	input := evalPointInput{
		PointID: "sample--profile--model--rep0",
		Sample:  Sample{Name: "sample"}, Harness: Harness{Name: "profile", Binary: "agent"},
		Model: "model", ProfilePath: "profile.yaml", GridPoint: GridPoint{"effort": "high"},
	}
	data, err := json.Marshal(input)
	require.NoError(t, err)
	es := &EvalSessionState{SessionDir: "session", CoreRoot: "core", Stderr: &bytes.Buffer{}}
	cmd := &runPointCmd{es: es}
	cmd.SetCommandState(evalTestCommandState{"point": string(data)})

	require.NoError(t, cmd.bindPointContext())
	require.Equal(t, input.PointID, es.PC.PointID)
	require.Equal(t, input.ProfilePath, es.PC.ProfilePath)
	require.Equal(t, input.GridPoint, es.PC.GridPoint)
	require.Equal(t, es.SessionDir, es.PC.SessionDir)
}

type evalTestCommandState map[string]string

func (s evalTestCommandState) Lookup(label string) (string, bool) {
	value, ok := s[label]
	return value, ok
}

type evalTestFailureBuilder struct{}

func (evalTestFailureBuilder) Build(core.Result) core.Command {
	return evalTestFailureCmd{}
}

type evalTestFailureCmd struct{}

func (evalTestFailureCmd) Name() string { return "fail_before_metrics" }
func (evalTestFailureCmd) Execute() core.Result {
	err := fmt.Errorf("synthetic point failure")
	return core.Result{CommandName: "fail_before_metrics", Signal: core.CommandError, Err: err, Output: err.Error()}
}
func (evalTestFailureCmd) Undo(core.Result) core.Result {
	return core.NoopUndo("fail_before_metrics")
}
