// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/execute"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/control"
)

type mixedFamilyRollbackFixture struct {
	profile       string
	declarations  string
	workspace     string
	reverter      *programReverter
	resources     runResources
	remoteCurrent *string
}

type mixedRollbackReport struct {
	Reverted float64  `json:"reverted_entries"`
	Skipped  []string `json:"skipped_irreversible"`
	Pending  []struct {
		Command string `json:"command"`
	} `json:"pending_compensation"`
	Failed []struct {
		Command string `json:"command"`
	} `json:"failed_entries"`
	Detail string `json:"detail"`
}

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

func mixedFamilyDeclarations() string {
	return `tools:
  - name: decode_response
    type: builtin
    init: parse_response
    visibility: internal
    side_effects: [{kind: state_mutation, target: parse_retry_counter, state: reset}]
    reversibility: {classification: reversible}
    undo: {strategy: parse_retry_counter_restore}
  - name: load_validation
    type: builtin
    init: load_corpus
    visibility: internal
    side_effects: [{kind: state_mutation, target: validation_state, state: corpus_loaded}]
    reversibility: {classification: reversible}
    undo: {strategy: session_state_restore}
    config: {corpus_optional: true}
  - name: invoke_executor
    type: builtin
    init: self_invoke
    visibility: internal
    side_effects: [{kind: child_process, target: child_agent, state: started}]
    reversibility: {classification: compensatable}
    undo:
      strategy: child_agent_workspace_restore
      description: restore child workspace and inspect trace artifacts
      requires: [child_workspace_ref, child_trace]
    config: {profile: agents/executor/profile.yaml}
  - name: await_release
    type: builtin
    init: suspend
    visibility: internal
    side_effects: [{kind: state_mutation, target: lifecycle_checkpoint, state: suspended}]
    reversibility: {classification: compensatable}
    undo:
      strategy: compensating_action
      description: resume, reject, or roll back
      requires: [operator_decision, resume_signal_or_rollback_target]
    config: {label: release, reason: approve release, require_checkpoint: true}
  - name: checkpoint_alias
    type: builtin
    init: checkpoint_rollback
    visibility: internal
    side_effects: [{kind: state_mutation, target: checkpoint_backend, state: reverted}]
    reversibility: {classification: compensatable}
    undo:
      strategy: compensating_action
      description: choose the checkpoint to resume
      requires: [operator_decision, resume_checkpoint_or_rollback_target]
    config: {to_iteration: 1}
  - name: exit_alias
    type: builtin
    init: exit_agent
    visibility: internal
    side_effects: [{kind: state_mutation, target: lifecycle, state: exited}]
    reversibility: {classification: irreversible, requires_confirmation: true}
    undo: {strategy: irreversible}
    config: {reason: complete, status: success}
  - name: launch_api
    type: builtin
    init: rest_server_launch
    visibility: internal
    emits: [ServerLaunched, CommandError]
    side_effects: [{kind: network_listen, target: fixture_api, state: listener_started}]
    reversibility: {classification: compensatable}
    undo: {strategy: compensating_action}
    config: {rest_ref: fixture}
  - name: set_remote
    type: builtin
    init: rest_client_set
    visibility: internal
    emits: [RESTResourceWritten, CommandError]
    side_effects: [{kind: external_api, target: fixture.item, state: item_updated}]
    reversibility: {classification: compensatable}
    undo: {strategy: compensating_action}
    config: {rest_ref: fixture, resource: item, operation: set}
  - name: read_remote
    type: builtin
    init: rest_client_get
    visibility: internal
    emits: [RESTResourceRead, CommandError]
    side_effects: [{kind: external_api, target: fixture.item, state: read_only}]
    reversibility: {classification: reversible}
    undo: {strategy: noop}
    config: {rest_ref: fixture, resource: item, operation: get}
`
}

func mixedFamilyRESTDefinition(baseURL string) string {
	return fmt.Sprintf(`rest:
  version: v1
  clients:
    fixture:
      base_url: %s
      resources:
        item:
          path: /items/{id}
          operations:
            get:
              method: GET
              params:
                path: {id: {type: string}}
              success: {status: [200], signal: RESTResourceRead}
              side_effects: [{kind: external_api, target: fixture.item, state: read_only}]
              reversibility: {classification: reversible, undo: noop}
            set:
              method: PATCH
              params:
                path: {id: {type: string}}
                body_schema:
                  type: object
                  properties: {value: {type: string}}
                  required: [value]
              body: {value: "{{ params.value }}"}
              success: {status: [200], signal: RESTResourceWritten}
              side_effects: [{kind: external_api, target: fixture.item, state: item_updated}]
              reversibility: {classification: compensatable, undo: set}
              compensation:
                operation: set
                parameters: {value: restored}
  servers:
    fixture:
      address: 127.0.0.1:0
      endpoints:
        health:
          method: GET
          path: /health
          binding: health
`, baseURL)
}
