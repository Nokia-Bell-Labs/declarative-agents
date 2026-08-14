// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/execute"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/control"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/filesystem"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestFreshRollbackRegistryRestoresOriginatingFileTool(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	target := filepath.Join(workspace, "artifact.txt")
	require.NoError(t, os.WriteFile(target, []byte("before"), 0o600))
	writeResult := executeRollbackFixtureWrite(t, workspace)

	profile := writeFileRollbackProgram(t)
	targetConfig, err := runtimeConfigForProfile(profile, runtimeConfig{Directory: workspace})
	require.NoError(t, err)
	ref, err := buildProgramRef(targetConfig)
	require.NoError(t, err)
	reverter := &programReverter{}
	require.NoError(t, reverter.Save(core.Position{
		Snapshot: core.AgentSnapshot{Program: ref},
	}, core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "write", Signal: core.ToolDone, Receipt: writeResult.Receipt},
	}))

	resources := rollbackFixtureResources(workspace)
	resources, err = augmentRollbackResources(resources, reverter)
	require.NoError(t, err)
	result := executeRebuiltRollback(t, resources, reverter)
	require.Equal(t, core.ToolDone, result.Signal, result.Output)
	content, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "before", string(content))
}

func TestFreshRollbackRegistryReportsAliasedSelfInvokeReceipt(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	traceDir := t.TempDir()
	child := (&control.SelfInvokeBuilder{
		ToolName: "invoke_executor",
		Config: execute.Config{
			Binary: "true", Profile: "agents/executor/profile.yaml", OTelDir: traceDir,
		},
		WorkspacePath: workspace,
		Ctx:           context.Background(),
	}).Build(core.Result{Output: `{"parameters":{"run_id":"child-run-42"}}`}).Execute()
	require.Equal(t, core.ToolDone, child.Signal)
	require.Equal(t, "invoke_executor", child.CommandName)
	require.NotEmpty(t, child.Receipt)

	profile := selfInvokeRollbackProgram(t)
	targetConfig, err := runtimeConfigForProfile(profile, runtimeConfig{Directory: workspace})
	require.NoError(t, err)
	ref, err := buildProgramRef(targetConfig)
	require.NoError(t, err)
	reverter := &programReverter{}
	require.NoError(t, reverter.Save(core.Position{
		Snapshot: core.AgentSnapshot{Program: ref},
	}, core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{
			Iteration: 2, CommandName: "invoke_executor",
			Signal: core.ToolDone, Receipt: child.Receipt,
		},
	}))

	resources := rollbackFixtureResources(workspace)
	resources, err = augmentRollbackResources(resources, reverter)
	require.NoError(t, err)
	result := executeRebuiltRollback(t, resources, reverter)
	require.Equal(t, core.ToolDone, result.Signal, result.Output)
	var report struct {
		Pending []struct {
			Command     string         `json:"command"`
			Description string         `json:"description"`
			Requires    []string       `json:"requires"`
			Data        map[string]any `json:"data"`
		} `json:"pending_compensation"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &report))
	require.Len(t, report.Pending, 1)
	pending := report.Pending[0]
	require.Equal(t, "invoke_executor", pending.Command)
	require.Contains(t, pending.Description, "restore the child workspace")
	require.Equal(t, []string{"child_workspace_ref", "child_trace"}, pending.Requires)
	require.Equal(t, "invoke_executor", pending.Data["command_name"])
	require.Equal(t, "agents/executor/profile.yaml", pending.Data["child_profile"])
	require.Equal(t, "child-run-42", pending.Data["child_run_id"])
	require.Equal(t, workspace, pending.Data["child_workspace_path"])
	require.Equal(t, []any{traceDir + "/child-child-run-42.otel.json"}, pending.Data["artifact_paths"])
}

func executeRollbackFixtureWrite(t *testing.T, workspace string) core.Result {
	t.Helper()
	builder := &filesystem.WriteBuilder{Root: workspace}
	result := builder.Build(core.Result{
		Output: `{"parameters":{"path":"artifact.txt","content":"after"}}`,
	}).Execute()
	require.Equal(t, core.ToolDone, result.Signal)
	require.NotEmpty(t, result.Receipt)
	return result
}

func writeFileRollbackProgram(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "machine.yaml"), "name: origin\n")
	writeTestFile(t, filepath.Join(dir, "tools.yaml"), "tools: [write]\n")
	writeTestFile(t, filepath.Join(dir, "declarations.yaml"), `tools:
  - name: write
    type: builtin
    init: file_write
    visibility: internal
    reversibility: {classification: reversible}
    undo: {strategy: file_snapshot_restore, description: restore prior file}
`)
	profile := filepath.Join(dir, "profile.yaml")
	writeTestFile(t, profile, `name: origin
machine: machine.yaml
tools: [tools.yaml]
tool_declarations: [declarations.yaml]
`)
	return profile
}

func selfInvokeRollbackProgram(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "machine.yaml"), "name: origin\n")
	writeTestFile(t, filepath.Join(dir, "tools.yaml"), "tools: [invoke_executor]\n")
	writeTestFile(t, filepath.Join(dir, "declarations.yaml"), `tools:
  - name: invoke_executor
    type: builtin
    init: self_invoke
    visibility: internal
    reversibility: {classification: compensatable}
    undo:
      strategy: child_agent_workspace_restore
      description: restore child workspace and inspect trace artifacts
      requires: [child_workspace_ref, child_trace]
    config:
      profile: agents/executor/profile.yaml
`)
	profile := filepath.Join(dir, "profile.yaml")
	writeTestFile(t, profile, `name: origin
machine: machine.yaml
tools: [tools.yaml]
tool_declarations: [declarations.yaml]
`)
	return profile
}

func rollbackFixtureResources(workspace string) runResources {
	toIteration := 1
	return runResources{
		Config: runtimeConfig{Directory: workspace},
		Definitions: []catalog.ToolDef{{
			Name: "checkpoint_rollback", Type: "builtin", Init: "checkpoint_rollback",
			Config: map[string]any{"to_iteration": toIteration},
		}},
	}
}

func executeRebuiltRollback(
	t *testing.T, resources runResources, reverter core.CheckpointReverter,
) core.Result {
	t.Helper()
	registry := core.NewRegistry()
	builtins := toolregistry.NewBuiltinRegistry()
	state := newAgentState(resources.Config, agentStateDeps{
		Registry: registry, Tracer: tracing.NoopTracer{},
		Checkpoint: reverter, LifecycleCheckpoint: reverter,
		Ctx: context.Background(),
	})
	registerBuiltinFactories(builtins, state, selectedBuiltinInits(resources.Definitions))
	require.NoError(t, registerRuntimeTools(
		registry, builtins, resources.Config, core.MachineSpec{}, resources.Definitions,
	))
	builder, ok := registry.Resolve("checkpoint_rollback")
	require.True(t, ok)
	return builder.Build(core.Result{}).Execute()
}

type programReverter struct {
	position  core.Position
	execution core.Execution
}

func (r *programReverter) Save(position core.Position, execution core.Execution) error {
	r.position = position
	r.execution = append(core.Execution(nil), execution...)
	return nil
}

func (r *programReverter) Load() (core.Position, core.Execution, error) {
	return r.position, append(core.Execution(nil), r.execution...), nil
}

func (r *programReverter) Revert(_ string, step int) error {
	r.execution = append(core.Execution(nil), r.execution[:step+1]...)
	return nil
}

var _ core.CheckpointReverter = (*programReverter)(nil)
