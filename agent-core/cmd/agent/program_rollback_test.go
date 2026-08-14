// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/filesystem"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestFreshRollbackRegistryTracesRepresentativeFamilies(t *testing.T) {
	t.Run("successful receipt walk", func(t *testing.T) {
		fixture := newMixedFamilyRollbackFixture(t, nil)
		result, state, trace := fixture.rollback(t, true)

		require.Equal(t, core.ToolDone, result.Signal, result.Output)
		report := decodeMixedRollbackReport(t, result.Output)
		require.Equal(t, float64(5), report.Reverted)
		require.ElementsMatch(t, []string{"exit_alias"}, report.Skipped)
		require.ElementsMatch(t,
			[]string{"invoke_executor", "await_release", "checkpoint_alias"},
			pendingCommands(report.Pending),
		)
		require.Empty(t, report.Failed)
		require.NotContains(t, report.Detail, "no builder registered")
		require.NotContains(t, report.Detail, "does not implement Reverser")
		require.NotContains(t, report.Detail, "no receipt")

		require.Equal(t, 2, state.parseRetries.Snapshot(), "aliased LLM retry state must be restored")
		require.NotNil(t, state.validation)
		require.True(t, state.validation.HasErrors, "validation state must be restored from its durable reference")
		require.Nil(t, state.validation.Corpus)
		require.Equal(t, "restored", fixture.remoteValue())
		requireRollbackTraceCommands(t, trace, "rollback.entry_reversed",
			"decode_response", "load_validation", "launch_api", "set_remote", "read_remote")
		requireRollbackTraceCommands(t, trace, "rollback.entry_compensation_required",
			"invoke_executor", "await_release", "checkpoint_alias")
		requireRollbackTraceCommands(t, trace, "rollback.entry_skipped", "exit_alias")
	})

	t.Run("named malformed receipts remain partial failures", func(t *testing.T) {
		fixture := newMixedFamilyRollbackFixture(t, map[string]bool{
			"decode_response": true,
			"load_validation": true,
		})
		result, _, trace := fixture.rollback(t, false)

		require.Equal(t, core.CommandError, result.Signal, result.Output)
		report := decodeMixedRollbackReport(t, result.Output)
		require.ElementsMatch(t, []string{"decode_response", "load_validation"}, failedCommands(report.Failed))
		require.ElementsMatch(t,
			[]string{"invoke_executor", "await_release", "checkpoint_alias"},
			pendingCommands(report.Pending),
		)
		require.ElementsMatch(t, []string{"exit_alias"}, report.Skipped)
		require.Equal(t, "restored", fixture.remoteValue(),
			"a malformed family receipt must not stop later REST compensation")
		require.NotContains(t, report.Detail, "no builder registered")
		require.NotContains(t, report.Detail, "does not implement Reverser")
		require.NotContains(t, report.Detail, "no receipt")
		requireRollbackTraceCommands(t, trace, "rollback.entry_undo_failed",
			"decode_response", "load_validation")
	})
}

func newMixedFamilyRollbackFixture(
	t *testing.T,
	malformed map[string]bool,
) mixedFamilyRollbackFixture {
	t.Helper()
	remoteCurrent := "before"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !strings.HasPrefix(req.URL.Path, "/items/") {
			http.NotFound(w, req)
			return
		}
		if req.Method == http.MethodPatch {
			var body struct {
				Value string `json:"value"`
			}
			require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
			remoteCurrent = body.Value
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]string{
			"id": "1", "value": remoteCurrent,
		}))
	}))
	t.Cleanup(upstream.Close)

	dir := t.TempDir()
	declarations := mixedFamilyDeclarations()
	writeTestFile(t, filepath.Join(dir, "machine.yaml"), "name: mixed-origin\n")
	writeTestFile(t, filepath.Join(dir, "tools.yaml"), `tools:
  - decode_response
  - load_validation
  - invoke_executor
  - await_release
  - checkpoint_alias
  - exit_alias
  - launch_api
  - set_remote
  - read_remote
`)
	declarationsPath := filepath.Join(dir, "declarations.yaml")
	writeTestFile(t, declarationsPath, declarations)
	writeTestFile(t, filepath.Join(dir, "rest.yaml"), mixedFamilyRESTDefinition(upstream.URL))
	profile := filepath.Join(dir, "profile.yaml")
	writeTestFile(t, profile, `name: mixed-origin
machine: machine.yaml
tools: [tools.yaml]
tool_declarations: [declarations.yaml]
rest_definitions: [rest.yaml]
`)

	workspace := filepath.Join("..", "..", "pkg", "spec", "testdata", "valid")
	cfg, err := runtimeConfigForProfile(profile, runtimeConfig{
		Directory: workspace, ChildAgentBinary: "true",
	})
	require.NoError(t, err)
	ref, err := buildProgramRef(cfg)
	require.NoError(t, err)
	program, err := loadReferencedProgram(ref, cfg)
	require.NoError(t, err)
	reverter := &programReverter{}
	resources := runResources{
		Config: cfg, Definitions: program.Definitions, RestDefinitions: program.RestDefinitions,
		Program: ref,
	}
	registry, state := buildRollbackRegistry(t, resources, reverter, tracing.NoopTracer{})
	state.parseRetries.ReportParseError()
	state.parseRetries.ReportParseError()
	require.NotNil(t, state.validation)
	state.validation.HasErrors = true
	state.validation.CorpusOptional = true
	priorDomain, err := state.snapshotDomain()
	require.NoError(t, err)
	require.NoError(t, reverter.Save(core.Position{
		Snapshot: core.AgentSnapshot{Program: ref, Domain: priorDomain},
	}, core.Execution{{Iteration: 1, CommandName: "seed", Signal: core.ToolDone}}))

	results := []core.Result{
		executeRollbackFixtureCommand(t, registry, "decode_response",
			core.Result{Output: `{"tool":"done","parameters":{"summary":"decoded"}}`}),
		executeRollbackFixtureCommand(t, registry, "load_validation", core.Result{}),
		executeRollbackFixtureCommand(t, registry, "invoke_executor",
			core.Result{Output: `{"parameters":{"run_id":"child-mixed-42"}}`}),
		executeRollbackFixtureCommand(t, registry, "await_release", core.Result{}),
		executeRollbackFixtureCommand(t, registry, "checkpoint_alias", core.Result{}),
		executeRollbackFixtureCommand(t, registry, "exit_alias", core.Result{}),
		executeRollbackFixtureCommand(t, registry, "launch_api", core.Result{}),
		executeRollbackFixtureCommand(t, registry, "set_remote",
			core.Result{Output: `{"parameters":{"id":"1","value":"after"}}`}),
		executeRollbackFixtureCommand(t, registry, "read_remote",
			core.Result{Output: `{"parameters":{"id":"1"}}`}),
	}
	for i := range results {
		require.NotEqual(t, core.CommandError, results[i].Signal, results[i].Output)
		if malformed[results[i].CommandName] {
			results[i].Receipt = "{"
		}
	}
	require.Equal(t, "after", remoteCurrent)

	// A fresh process has no ownership of the old listener. Stop the originating
	// process's listener before reconstruction, then let the rebuilt server
	// reverser validate the persisted receipt and report it already compensated.
	launchBuilder, ok := registry.Resolve("launch_api")
	require.True(t, ok)
	launchReverser, ok := launchBuilder.(core.Reverser)
	require.True(t, ok)
	stopped := launchReverser.BuildReverser().Undo(results[6])
	require.Equal(t, core.Signal("ServerStopped"), stopped.Signal, stopped.Output)

	execution := core.Execution{{Iteration: 1, CommandName: "seed", Signal: core.ToolDone}}
	for i, result := range results {
		execution = append(execution, core.Entry{
			Iteration: i + 2, CommandName: result.CommandName,
			Signal: result.Signal, Receipt: result.Receipt,
		})
	}
	currentDomain, err := state.snapshotDomain()
	require.NoError(t, err)
	require.NoError(t, reverter.Save(core.Position{
		Snapshot: core.AgentSnapshot{Program: ref, Domain: currentDomain},
	}, execution))

	return mixedFamilyRollbackFixture{
		profile: profile, declarations: declarations, workspace: workspace,
		reverter: reverter, resources: resources, remoteCurrent: &remoteCurrent,
	}
}

func (f mixedFamilyRollbackFixture) rollback(
	t *testing.T,
	verifyDigest bool,
) (core.Result, *agentState, *tracing.RecordingTracer) {
	t.Helper()
	resources := rollbackFixtureResources(f.workspace)
	resources.Config.ChildAgentBinary = "true"
	if verifyDigest {
		declarationsPath := filepath.Join(filepath.Dir(f.profile), "declarations.yaml")
		writeTestFile(t, declarationsPath, f.declarations+"# digest drift\n")
		_, err := augmentRollbackResources(resources, f.reverter)
		require.ErrorContains(t, err, "target program")
		require.ErrorContains(t, err, "changed")
		writeTestFile(t, declarationsPath, f.declarations)
	}
	var err error
	resources, err = augmentRollbackResources(resources, f.reverter)
	require.NoError(t, err, "the persisted program digest must verify before reconstruction")

	trace := tracing.NewRecordingTracer()
	registry, state := buildRollbackRegistry(t, resources, f.reverter, trace)
	require.NoError(t, catalog.ValidateReceiptFamilies(
		mixedReceiptFamilies(t, resources.Definitions), registry,
	))
	builder, ok := registry.Resolve("checkpoint_rollback")
	require.True(t, ok)
	result := builder.Build(core.Result{}).Execute()
	return result, state, trace
}

func (f mixedFamilyRollbackFixture) remoteValue() string {
	return *f.remoteCurrent
}

func buildRollbackRegistry(
	t *testing.T,
	resources runResources,
	reverter core.CheckpointReverter,
	tracer tracing.Tracer,
) (*core.Registry, *agentState) {
	t.Helper()
	registry := core.NewRegistry()
	builtins := toolregistry.NewBuiltinRegistry()
	state := newAgentState(resources.Config, agentStateDeps{
		Registry: registry, Tracer: tracer, RunID: "run-mixed",
		Checkpoint: reverter, LifecycleCheckpoint: reverter,
		Ctx: context.Background(), RestDefs: resources.RestDefinitions,
		ParseRetries: &toollm.ParseErrorRetryTracker{MaxConsecutive: 3},
		shutdown:     func() {},
	})
	registerBuiltinFactories(builtins, state, selectedBuiltinInits(resources.Definitions))
	require.NoError(t, registerRuntimeTools(
		registry, builtins, resources.Config, core.MachineSpec{}, resources.Definitions,
	))
	return registry, state
}

func executeRollbackFixtureCommand(
	t *testing.T,
	registry *core.Registry,
	name string,
	previous core.Result,
) core.Result {
	t.Helper()
	builder, ok := registry.Resolve(name)
	require.True(t, ok, "missing originating builder %q", name)
	result := builder.Build(previous).Execute()
	require.Equal(t, name, result.CommandName, result.Output)
	return result
}

func mixedReceiptFamilies(t *testing.T, defs []catalog.ToolDef) []catalog.ReceiptFamily {
	t.Helper()
	return []catalog.ReceiptFamily{
		{Name: "llm", Definitions: []catalog.ToolDef{
			definitionByName(t, defs, "decode_response"),
		}},
		{Name: "validation", Definitions: []catalog.ToolDef{
			definitionByName(t, defs, "load_validation"),
		}},
		{Name: "child", Definitions: []catalog.ToolDef{
			definitionByName(t, defs, "invoke_executor"),
		}},
		{Name: "lifecycle", Definitions: []catalog.ToolDef{
			definitionByName(t, defs, "await_release"),
			definitionByName(t, defs, "checkpoint_alias"),
			definitionByName(t, defs, "exit_alias"),
		}},
		{Name: "rest", Definitions: []catalog.ToolDef{
			definitionByName(t, defs, "launch_api"),
			definitionByName(t, defs, "set_remote"),
			definitionByName(t, defs, "read_remote"),
		}},
	}
}

func decodeMixedRollbackReport(t *testing.T, output string) mixedRollbackReport {
	t.Helper()
	var report mixedRollbackReport
	require.NoError(t, json.Unmarshal([]byte(output), &report))
	return report
}

func pendingCommands(pending []struct {
	Command string `json:"command"`
}) []string {
	names := make([]string, len(pending))
	for i := range pending {
		names[i] = pending[i].Command
	}
	return names
}

func failedCommands(failed []struct {
	Command string `json:"command"`
}) []string {
	names := make([]string, len(failed))
	for i := range failed {
		names[i] = failed[i].Command
	}
	return names
}

func requireRollbackTraceCommands(
	t *testing.T,
	trace *tracing.RecordingTracer,
	event string,
	wants ...string,
) {
	t.Helper()
	var commands []string
	for _, recorded := range trace.Events {
		if recorded.Name == event {
			commands = append(commands, fmt.Sprint(recorded.Attrs["command"]))
		}
	}
	for _, want := range wants {
		require.Contains(t, commands, want, "event %s", event)
	}
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
	domainRef string
	domains   map[string][]byte
}

func (r *programReverter) Save(position core.Position, execution core.Execution) error {
	r.position = position
	r.execution = append(core.Execution(nil), execution...)
	if len(position.Snapshot.Domain) > 0 {
		if r.domains == nil {
			r.domains = make(map[string][]byte)
		}
		r.domainRef = fmt.Sprintf("fixture-domain-%d", len(execution))
		r.domains[r.domainRef] = append([]byte(nil), position.Snapshot.Domain...)
	}
	return nil
}

func (r *programReverter) Load() (core.Position, core.Execution, error) {
	return r.position, append(core.Execution(nil), r.execution...), nil
}

func (r *programReverter) Revert(_ string, step int) error {
	r.execution = append(core.Execution(nil), r.execution[:step+1]...)
	return nil
}

func (r *programReverter) DomainReference() (string, bool) {
	return r.domainRef, r.domainRef != ""
}

func (r *programReverter) ResolveDomainSnapshot(reference string) ([]byte, error) {
	domain, ok := r.domains[reference]
	if !ok {
		return nil, fmt.Errorf("fixture domain reference %q is unavailable", reference)
	}
	return append([]byte(nil), domain...), nil
}

var _ core.CheckpointReverter = (*programReverter)(nil)
var _ core.DomainReferenceProvider = (*programReverter)(nil)
var _ core.DomainSnapshotResolver = (*programReverter)(nil)
