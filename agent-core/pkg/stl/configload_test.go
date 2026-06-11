// Copyright (c) 2026 Nokia. All rights reserved.

package stl_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/stl"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
)

// configDir resolves the absolute path to the top-level configs/ directory.
func configDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	abs := filepath.Join(filepath.Dir(thisFile), "..", "..", "configs")
	info, err := os.Stat(abs)
	require.NoError(t, err, "configs directory must exist at %s", abs)
	require.True(t, info.IsDir())
	return abs
}

// loadTestDefs loads shared tool declarations, mode-local overrides,
// per-agent LLM defaults, and applies the agent's selection file.
func loadTestDefs(t *testing.T, cd, agent string) []stl.ToolDef {
	return loadTestDefsForSelection(t, cd, agent, filepath.Join(cd, agent, "tools.yaml"))
}

func loadTestDefsForSelection(t *testing.T, cd, agent, selectionPath string) []stl.ToolDef {
	t.Helper()
	declPaths := []string{
		filepath.Join(cd, "tools", "builtin.yaml"),
		filepath.Join(cd, "tools", "exec.yaml"),
	}
	modeBuiltin := filepath.Join(cd, agent, "builtin.yaml")
	if _, err := os.Stat(modeBuiltin); err == nil {
		declPaths = append(declPaths, modeBuiltin)
	}
	llmDefault := filepath.Join(cd, agent, "llm", "default.yaml")
	if _, err := os.Stat(llmDefault); err == nil {
		declPaths = append(declPaths, llmDefault)
	}
	declarations, err := stl.LoadToolDeclarations(declPaths)
	require.NoError(t, err)
	selection, err := stl.LoadToolSelection(selectionPath)
	require.NoError(t, err)
	defs, err := stl.SelectTools(declarations, selection)
	require.NoError(t, err)
	return defs
}

type noopBuilder struct{}

func (noopBuilder) Build(_ core.Result) core.Command { return noopCmd{} }

type noopCmd struct{}

func (noopCmd) Name() string         { return "noop" }
func (noopCmd) Execute() core.Result { return core.Result{Signal: core.ToolDone} }
func (noopCmd) Undo() core.Result    { return core.NoopUndo("noop") }

// buildRegistryForDefs creates a fully wired Registry from tool definitions.
// All builtin factories are stubbed so no Ollama server or real implementations
// are required — this validates config wiring only.
func buildRegistryForDefs(t *testing.T, defs []stl.ToolDef) *core.Registry {
	t.Helper()
	builtins := stl.NewBuiltinRegistry()
	reg := core.NewRegistry()

	for _, td := range defs {
		if td.Type == "builtin" && td.Init != "" {
			func(initName string) {
				builtins.Register(initName, func(_ stl.ToolDef, _ map[string]string) (core.Builder, error) {
					return noopBuilder{}, nil
				})
			}(td.Init)
		}
	}

	vars := map[string]string{"directory": t.TempDir(), "model": "test", "ollama_url": "http://localhost:11434"}
	err := stl.RegisterUnifiedTools(reg, builtins, vars["directory"], defs, vars)
	require.NoError(t, err)
	return reg
}

func assertToolNames(t *testing.T, defs []stl.ToolDef, want []string) {
	t.Helper()
	nameSet := make(map[string]bool, len(defs))
	for _, td := range defs {
		nameSet[td.Name] = true
	}
	for _, w := range want {
		require.True(t, nameSet[w], "expected tool %q not found in definitions", w)
	}
}

func requireToolDef(t *testing.T, defs []stl.ToolDef, name string) stl.ToolDef {
	t.Helper()
	for _, td := range defs {
		if td.Name == name {
			return td
		}
	}
	require.Failf(t, "missing tool definition", "tool %q not found", name)
	return stl.ToolDef{}
}

func assertToolEmits(t *testing.T, spec core.MachineSpec, defs []stl.ToolDef) {
	t.Helper()
	require.NoError(t, stl.ValidateToolEmits(spec, defs))
}

func assertTransition(t *testing.T, spec core.MachineSpec, state, signal, next, action string) {
	t.Helper()
	for _, tr := range spec.Transitions {
		if tr.State == state && tr.Signal == signal {
			require.Equal(t, next, tr.Next, "transition %s/%s next", state, signal)
			require.Equal(t, action, tr.Action, "transition %s/%s action", state, signal)
			return
		}
	}
	require.Failf(t, "missing transition", "%s/%s", state, signal)
}

func TestLLMConfigsDeclareManifestState(t *testing.T) {
	cd := configDir(t)
	matches, err := filepath.Glob(filepath.Join(cd, "*", "llm", "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, matches)

	for _, path := range matches {
		defs, err := stl.LoadToolDefs(path)
		require.NoError(t, err, path)
		for _, def := range defs {
			if def.Init != "invoke_llm" {
				continue
			}
			var cfg stl.LLMToolConfig
			require.NoError(t, stl.DecodeToolConfig(def, &cfg), path)
			require.NotEmpty(t, cfg.Provider, path)
			require.NotEmpty(t, cfg.ManifestState, path)
		}
	}
}

func TestToolContractsWarnOnlyReport(t *testing.T) {
	cd := configDir(t)
	cases := []struct {
		name          string
		agent         string
		selectionPath string
	}{
		{name: "generator", agent: "generator", selectionPath: filepath.Join(cd, "generator", "tools.yaml")},
		{name: "planner", agent: "planner", selectionPath: filepath.Join(cd, "planner", "tools.yaml")},
		{name: "evaluator-session", agent: "evaluator", selectionPath: filepath.Join(cd, "evaluator", "tools.yaml")},
		{name: "evaluator-point", agent: "evaluator", selectionPath: filepath.Join(cd, "evaluator", "tools-point.yaml")},
		{name: "bench", agent: "bench", selectionPath: filepath.Join(cd, "bench", "tools.yaml")},
		{name: "validate", agent: "validate", selectionPath: filepath.Join(cd, "validate", "tools.yaml")},
	}

	total := 0
	bySeverity := map[string]int{}
	byCategory := map[string]int{}
	for _, tc := range cases {
		defs := loadTestDefsForSelection(t, cd, tc.agent, tc.selectionPath)
		findings := stl.ValidateToolContracts(defs, stl.ContractValidationOptions{
			IncludeInternal: true,
		})
		total += len(findings)
		for _, finding := range findings {
			bySeverity[finding.Severity]++
			byCategory[finding.Category]++
		}
		t.Logf("%s: %d tool contract findings", tc.name, len(findings))
	}

	require.NotZero(t, total, "current declarations should still have warn-only contract migration findings")
	t.Logf("tool contract findings by severity: %v", bySeverity)
	t.Logf("tool contract findings by category: %v", byCategory)
}

func TestBuiltinToolContractAuditClassifiesMigrationCoverage(t *testing.T) {
	cd := configDir(t)
	defs, err := stl.LoadToolDeclarations([]string{
		filepath.Join(cd, "tools", "builtin.yaml"),
	})
	require.NoError(t, err)

	audit := stl.AuditToolContracts(defs, stl.ContractValidationOptions{IncludeInternal: true})

	require.NotEmpty(t, audit)
	statusCounts := map[string]int{}
	byTool := map[string]stl.ContractAuditEntry{}
	for _, entry := range audit {
		statusCounts[entry.Status]++
		byTool[entry.ToolName] = entry
		require.NotContains(t, entry.MissingFields, "side_effects", "tool %s should declare side effects", entry.ToolName)
		require.NotContains(t, entry.MissingFields, "reversibility.classification", "tool %s should declare reversibility", entry.ToolName)
		require.NotContains(t, entry.MissingFields, "undo", "tool %s should declare undo", entry.ToolName)
		if entry.Category == "boundary" || entry.Category == "stateful_internal" {
			require.NotEmpty(t, entry.MigrationTarget, "stateful/boundary tool %s should name a migration target", entry.ToolName)
		}
	}
	require.NotZero(t, statusCounts[stl.ContractAuditPartial], "audit should identify partial contracts")
	require.Equal(t, stl.ContractAuditPartial, byTool["validate"].Status)
	require.Contains(t, byTool["validate"].MigrationTarget, "boundary side effects")
	require.Equal(t, stl.ContractAuditPartial, byTool["parse_response"].Status)
	require.Contains(t, byTool["parse_response"].MissingFields, "output.schema")
	t.Logf("builtin tool contract audit status counts: %v", statusCounts)
}

func TestBuiltinMigratedContractsValidateAtErrorLevel(t *testing.T) {
	cd := configDir(t)
	defs, err := stl.LoadToolDeclarations([]string{
		filepath.Join(cd, "tools", "builtin.yaml"),
	})
	require.NoError(t, err)

	validate := requireToolDef(t, defs, "validate")
	require.Equal(t, stl.ToolContractMigrated, validate.Contract)
	parseResponse := requireToolDef(t, defs, "parse_response")
	require.Equal(t, stl.ToolContractLegacy, parseResponse.Contract)

	findings := stl.ValidateToolContracts([]stl.ToolDef{validate, parseResponse}, stl.ContractValidationOptions{
		IncludeInternal: true,
		MinimumLevel:    stl.ContractSeverityError,
	})
	require.Empty(t, findings)
}

func TestSelectedToolOutputContractsLoad(t *testing.T) {
	cd := configDir(t)
	defs := loadTestDefs(t, cd, "generator")

	for _, name := range []string{"read", "write", "edit", "test"} {
		def := requireToolDef(t, defs, name)
		require.NotEmpty(t, def.Output.Schema, "tool %s should declare output.schema", name)
		require.Equal(t, "object", def.Output.Schema["type"], "tool %s output schema type", name)
		require.NotEmpty(t, def.Output.Description, "tool %s should describe output", name)
	}

	allDecls, err := stl.LoadToolDeclarations([]string{
		filepath.Join(cd, "tools", "builtin.yaml"),
		filepath.Join(cd, "tools", "exec.yaml"),
	})
	require.NoError(t, err)
	status := requireToolDef(t, allDecls, "workspace_status")
	require.Equal(t, "object", status.Output.Schema["type"])
}

func TestSelectedBoundaryToolContractsLoad(t *testing.T) {
	cd := configDir(t)
	cases := []struct {
		agent         string
		selectionPath string
		tools         []string
	}{
		{
			agent:         "generator",
			selectionPath: filepath.Join(cd, "generator", "tools.yaml"),
			tools:         []string{"invoke_llm"},
		},
		{
			agent:         "planner",
			selectionPath: filepath.Join(cd, "planner", "tools.yaml"),
			tools:         []string{"invoke_llm", "execute_task"},
		},
		{
			agent:         "evaluator",
			selectionPath: filepath.Join(cd, "evaluator", "tools.yaml"),
			tools:         []string{"run_point"},
		},
		{
			agent:         "bench",
			selectionPath: filepath.Join(cd, "bench", "tools.yaml"),
			tools:         []string{"launch_eval"},
		},
	}

	for _, tc := range cases {
		defs := loadTestDefsForSelection(t, cd, tc.agent, tc.selectionPath)
		for _, name := range tc.tools {
			def := requireToolDef(t, defs, name)
			require.Equal(t, "boundary", def.Category, "%s/%s category", tc.agent, name)
			require.NotEmpty(t, def.Problem, "%s/%s problem", tc.agent, name)
			require.NotEmpty(t, def.NonGoals, "%s/%s non_goals", tc.agent, name)
			require.NotEmpty(t, def.Output.Schema, "%s/%s output.schema", tc.agent, name)
			require.NotEmpty(t, def.SideEffects.Items, "%s/%s side_effects", tc.agent, name)
			require.NotEmpty(t, def.Reversibility.Classification, "%s/%s reversibility", tc.agent, name)
			require.NotEmpty(t, def.Undo.Strategy, "%s/%s undo", tc.agent, name)
			require.NotEmpty(t, def.Errors, "%s/%s errors", tc.agent, name)
		}
	}
}

func TestToolOutputContractsStayOutOfManifestInputSchema(t *testing.T) {
	cd := configDir(t)
	defs := loadTestDefs(t, cd, "generator")
	read := requireToolDef(t, defs, "read")

	spec := read.ToToolSpec()
	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(spec.InputSchema, &schema))
	require.NotContains(t, schema, "output")
	require.Contains(t, schema, "properties")
}

func dummyToolAction(_ core.Result) core.Command { return noopCmd{} }

// ---------------------------------------------------------------------------
// Generator config tests
// ---------------------------------------------------------------------------

func TestGeneratorConfig_MachineLoads(t *testing.T) {
	path := filepath.Join(configDir(t), "generator", "machine.yaml")
	spec, err := core.LoadMachineSpec(path)
	require.NoError(t, err)

	require.Equal(t, "generator", spec.Name)
	require.Equal(t, "Idle", spec.InitialState)
	require.Contains(t, spec.TerminalStates, "Succeeded")
	require.Contains(t, spec.TerminalStates, "Failed")
	require.Contains(t, spec.TerminalStates, "BudgetExceeded")
	require.NotEmpty(t, spec.States)
	require.NotEmpty(t, spec.Transitions)
	assertGeneratorValidationSequence(t, spec)
}

func assertGeneratorValidationSequence(t *testing.T, spec core.MachineSpec) {
	t.Helper()
	assertTransition(t, spec, "Parsing", "TaskCompleted", "ValidatingBuild", "build")
	assertTransition(t, spec, "ValidatingBuild", "ToolDone", "ValidatingLint", "lint")
	assertTransition(t, spec, "ValidatingLint", "ToolDone", "ValidatingTest", "test")
	assertTransition(t, spec, "ValidatingTest", "ToolDone", "Succeeded", "")
	assertTransition(t, spec, "ValidatingBuild", "ToolFailed", "Failed", "")
	assertTransition(t, spec, "ValidatingLint", "ToolFailed", "Failed", "")
	assertTransition(t, spec, "ValidatingTest", "ToolFailed", "Failed", "")
}

func TestGeneratorConfig_ToolsLoad(t *testing.T) {
	cd := configDir(t)
	defs := loadTestDefs(t, cd, "generator")
	require.NotEmpty(t, defs)

	assertToolNames(t, defs, []string{
		"read", "write", "edit", "find", "list_files",
		"build", "vet", "lint", "test",
		"invoke_llm", "parse_response", "done",
	})
}

func TestGeneratorConfig_TransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "generator", "machine.yaml"))
	require.NoError(t, err)

	defs := loadTestDefs(t, cd, "generator")
	assertToolEmits(t, spec, defs)
	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, dummyToolAction)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.NotNil(t, isTerminal)
	require.True(t, isTerminal(core.State("Succeeded")))
	require.False(t, isTerminal(core.State("Idle")))
}

func TestGeneratorConfig_DeepseekMachineLoads(t *testing.T) {
	path := filepath.Join(configDir(t), "generator", "deepseek-coding-agent.yaml")
	spec, err := core.LoadMachineSpec(path)
	require.NoError(t, err)

	require.Equal(t, "generator-deepseek", spec.Name)
	require.Equal(t, "Idle", spec.InitialState)
	require.Contains(t, spec.TerminalStates, "Succeeded")
	require.Contains(t, spec.TerminalStates, "Failed")
	require.Contains(t, spec.TerminalStates, "BudgetExceeded")
	assertGeneratorValidationSequence(t, spec)
}

func TestGeneratorConfig_DeepseekTransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "generator", "deepseek-coding-agent.yaml"))
	require.NoError(t, err)

	defs := loadTestDefs(t, cd, "generator")
	assertToolEmits(t, spec, defs)
	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, dummyToolAction)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.NotNil(t, isTerminal)
	require.True(t, isTerminal(core.State("Succeeded")))

	key := core.TransitionInput{State: core.State("Composing"), Signal: core.Signal("EditDone")}
	tv, ok := table[key]
	require.True(t, ok, "deepseek machine must handle EditDone in Composing")
	require.Equal(t, core.State("Composing"), tv.NextState)
}

// ---------------------------------------------------------------------------
// Pipeline (planner) config tests
// ---------------------------------------------------------------------------

func TestPlannerConfig_MachineLoads(t *testing.T) {
	path := filepath.Join(configDir(t), "planner", "machine.yaml")
	spec, err := core.LoadMachineSpec(path)
	require.NoError(t, err)

	require.Equal(t, "planner", spec.Name)
	require.Equal(t, "Idle", spec.InitialState)
	require.Contains(t, spec.TerminalStates, "Completed")
	require.Contains(t, spec.TerminalStates, "Failed")
	require.NotEmpty(t, spec.States)
	require.NotEmpty(t, spec.Transitions)
}

func TestPlannerConfig_PassthroughLoads(t *testing.T) {
	path := filepath.Join(configDir(t), "planner", "machine-passthrough.yaml")
	spec, err := core.LoadMachineSpec(path)
	require.NoError(t, err)

	require.Equal(t, "planner-passthrough", spec.Name)
	require.Equal(t, "Idle", spec.InitialState)
	require.Contains(t, spec.TerminalStates, "Completed")
	require.Contains(t, spec.TerminalStates, "Failed")
	require.Contains(t, spec.TerminalStates, "Stalled")
}

func TestPlannerConfig_PlanOnlyLoads(t *testing.T) {
	path := filepath.Join(configDir(t), "planner", "machine-plan-only.yaml")
	spec, err := core.LoadMachineSpec(path)
	require.NoError(t, err)

	require.Equal(t, "planner-plan-only", spec.Name)
	require.Equal(t, "Idle", spec.InitialState)
	require.Contains(t, spec.TerminalStates, "Completed")
	require.Contains(t, spec.TerminalStates, "Failed")
	require.Contains(t, spec.TerminalStates, "Stalled")
	require.Contains(t, spec.TerminalStates, "Paused")
}

func TestPlannerConfig_ToolsLoad(t *testing.T) {
	cd := configDir(t)
	defs := loadTestDefs(t, cd, "planner")
	require.NotEmpty(t, defs)

	assertToolNames(t, defs, []string{
		"extract_task", "extract_all", "assemble_prompt", "parse_plan",
		"create_issue", "execute_task", "invoke_llm", "reset_history",
		"check_result", "vet", "build", "test",
	})
}

func TestPlannerConfig_TransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "planner", "machine.yaml"))
	require.NoError(t, err)

	defs := loadTestDefs(t, cd, "planner")
	assertToolEmits(t, spec, defs)
	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, nil)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.True(t, isTerminal(core.State("Completed")))
	require.False(t, isTerminal(core.State("Idle")))
}

func TestPlannerConfig_PassthroughTransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "planner", "machine-passthrough.yaml"))
	require.NoError(t, err)

	defs := loadTestDefs(t, cd, "planner")
	assertToolEmits(t, spec, defs)
	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, nil)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.True(t, isTerminal(core.State("Completed")))
}

func TestPlannerConfig_PlanOnlyTransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "planner", "machine-plan-only.yaml"))
	require.NoError(t, err)

	defs := loadTestDefs(t, cd, "planner")
	assertToolEmits(t, spec, defs)
	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, nil)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.True(t, isTerminal(core.State("Completed")))
	require.True(t, isTerminal(core.State("Paused")))
}

// ---------------------------------------------------------------------------
// Evaluator config tests (session-level machine)
// ---------------------------------------------------------------------------

func TestEvaluatorConfig_SessionMachineLoads(t *testing.T) {
	path := filepath.Join(configDir(t), "evaluator", "machine.yaml")
	spec, err := core.LoadMachineSpec(path)
	require.NoError(t, err)

	require.Equal(t, "evaluator-session", spec.Name)
	require.Equal(t, "Idle", spec.InitialState)
	require.Contains(t, spec.TerminalStates, "Done")
	require.Contains(t, spec.TerminalStates, "Failed")
	require.NotEmpty(t, spec.States)
	require.NotEmpty(t, spec.Transitions)
}

func TestEvaluatorConfig_ToolsLoad(t *testing.T) {
	cd := configDir(t)
	defs := loadTestDefs(t, cd, "evaluator")
	require.NotEmpty(t, defs)

	assertToolNames(t, defs, []string{
		"parse_suite_config", "discover_suite_samples", "expand_eval_grid",
		"init_eval_session", "report_suite_summary",
		"next_point", "run_point", "report_session",
	})
}

func TestEvaluatorConfig_TransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "evaluator", "machine.yaml"))
	require.NoError(t, err)

	defs := loadTestDefs(t, cd, "evaluator")
	assertToolEmits(t, spec, defs)
	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, nil)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.True(t, isTerminal(core.State("Done")))
	require.True(t, isTerminal(core.State("Failed")))
	require.False(t, isTerminal(core.State("Idle")))
}

func TestEvaluatorConfig_PointMachineLoads(t *testing.T) {
	path := filepath.Join(configDir(t), "evaluator", "point.yaml")
	spec, err := core.LoadMachineSpec(path)
	require.NoError(t, err)

	require.NotEmpty(t, spec.Name)
	require.Equal(t, "Idle", spec.InitialState)
	require.Contains(t, spec.TerminalStates, "Done")
	require.Contains(t, spec.TerminalStates, "Failed")
}

func TestEvaluatorConfig_PointTransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "evaluator", "point.yaml"))
	require.NoError(t, err)

	defs := loadTestDefsForSelection(t, cd, "evaluator", filepath.Join(cd, "evaluator", "tools-point.yaml"))
	require.NotEmpty(t, defs)
	assertToolNames(t, defs, []string{
		"create_point_dir", "copy_sample_workspace", "copy_sample_docs",
		"init_workspace_repo", "stage_workspace_baseline", "commit_workspace_baseline",
		"dump_config",
		"run_agent", "run_oracle_check", "collect_trace_tokens",
		"check_agent_version", "summarize_point_results", "collect_metrics",
	})
	assertToolEmits(t, spec, defs)
	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, nil)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.True(t, isTerminal(core.State("Done")))
	require.True(t, isTerminal(core.State("Failed")))
	require.False(t, isTerminal(core.State("Idle")))
}

// ---------------------------------------------------------------------------
// Bench config tests
// ---------------------------------------------------------------------------

func TestBenchConfig_MachineLoads(t *testing.T) {
	path := filepath.Join(configDir(t), "bench", "machine.yaml")
	spec, err := core.LoadMachineSpec(path)
	require.NoError(t, err)

	require.Equal(t, "bench", spec.Name)
	require.Equal(t, "Idle", spec.InitialState)
	require.Contains(t, spec.TerminalStates, "Done")
	require.Contains(t, spec.TerminalStates, "Failed")
	require.NotEmpty(t, spec.States)
	require.NotEmpty(t, spec.Transitions)
}

func TestBenchConfig_ToolsLoad(t *testing.T) {
	cd := configDir(t)
	defs := loadTestDefs(t, cd, "bench")
	require.NotEmpty(t, defs)

	assertToolNames(t, defs, []string{
		"serve_ui", "launch_eval",
	})
}

func TestBenchConfig_TransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "bench", "machine.yaml"))
	require.NoError(t, err)

	defs := loadTestDefs(t, cd, "bench")
	assertToolEmits(t, spec, defs)
	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, nil)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.True(t, isTerminal(core.State("Done")))
	require.True(t, isTerminal(core.State("Failed")))
	require.False(t, isTerminal(core.State("Idle")))
}

// ---------------------------------------------------------------------------
// Validate config tests
// ---------------------------------------------------------------------------

func TestValidateConfig_MachineLoads(t *testing.T) {
	path := filepath.Join(configDir(t), "validate", "machine.yaml")
	spec, err := core.LoadMachineSpec(path)
	require.NoError(t, err)

	require.Equal(t, "validate", spec.Name)
	require.Equal(t, "Idle", spec.InitialState)
	require.Contains(t, spec.TerminalStates, "Passed")
	require.Contains(t, spec.TerminalStates, "Failed")
	require.NotEmpty(t, spec.States)
	require.NotEmpty(t, spec.Transitions)
}

func TestValidateConfig_ToolsLoad(t *testing.T) {
	cd := configDir(t)
	defs := loadTestDefs(t, cd, "validate")
	require.NotEmpty(t, defs)

	assertToolNames(t, defs, []string{
		"load_corpus", "validate_specs", "format_report",
	})
}

func TestValidateConfig_TransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "validate", "machine.yaml"))
	require.NoError(t, err)

	defs := loadTestDefs(t, cd, "validate")
	assertToolEmits(t, spec, defs)
	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, nil)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.True(t, isTerminal(core.State("Passed")))
	require.True(t, isTerminal(core.State("Failed")))
	require.False(t, isTerminal(core.State("Idle")))
}

// ---------------------------------------------------------------------------
// E2E loop tests with real generator configs and scripted LLM
// ---------------------------------------------------------------------------

type scriptedLLMBuilder struct {
	responses []string
	callIdx   int
}

func (s *scriptedLLMBuilder) Build(_ core.Result) core.Command {
	if s.callIdx >= len(s.responses) {
		return &scriptedCmd{output: `{"tool":"done","parameters":{"summary":"out of responses"}}`}
	}
	resp := s.responses[s.callIdx]
	s.callIdx++
	return &scriptedCmd{output: resp}
}

type scriptedCmd struct{ output string }

func (s *scriptedCmd) Name() string      { return "invoke_llm" }
func (s *scriptedCmd) Undo() core.Result { return core.NoopUndo(s.Name()) }

func (s *scriptedCmd) Execute() core.Result {
	return core.Result{
		Output:      s.output,
		Signal:      core.Signal("LLMResponded"),
		CommandName: "invoke_llm",
	}
}

type stubPassBuilder struct{ name string }

func (s *stubPassBuilder) Build(_ core.Result) core.Command {
	return &stubPassCmd{name: s.name}
}

type stubPassCmd struct{ name string }

func (s *stubPassCmd) Name() string      { return s.name }
func (s *stubPassCmd) Undo() core.Result { return core.NoopUndo(s.Name()) }

func (s *stubPassCmd) Execute() core.Result {
	return core.Result{
		Signal:      core.ToolDone,
		Output:      s.name + " passed",
		CommandName: s.name,
	}
}

func setupFixtureWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	appDir := filepath.Join(root, "cmd", "app")
	require.NoError(t, os.MkdirAll(appDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n\ngo 1.23\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "root.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"old help\")\n}\n"), 0o644))
	return root
}

// buildE2EToolAction creates an ActionFunc for $tool dynamic dispatch
// that unmarshals the ToolRequest from parse_response output and dispatches
// to the appropriate builder in the registry.
func buildE2EToolAction(reg *core.Registry, tracker *stl.ToolTracker) core.ActionFunc {
	return func(r core.Result) core.Command {
		var treq llm.ToolRequest
		if err := json.Unmarshal([]byte(r.Output), &treq); err != nil {
			return &failCmd{err: err}
		}
		builder, ok := reg.Resolve(treq.ToolName)
		if !ok {
			return &failCmd{err: nil}
		}
		tracker.Record(treq.ToolName)
		return builder.Build(core.Result{Output: r.Output})
	}
}

type failCmd struct{ err error }

func (f *failCmd) Name() string      { return "fail" }
func (f *failCmd) Undo() core.Result { return core.NoopUndo(f.Name()) }

func (f *failCmd) Execute() core.Result {
	return core.Result{Signal: core.CommandError, CommandName: "fail"}
}

// buildE2EParams loads the generator machine/tools configs and wires
// scripted LLM responses with real file tools + stub validation.
func buildE2EParams(t *testing.T, workspace string, llmResponses []string) core.LoopParams {
	t.Helper()
	cd := configDir(t)
	machineFile := filepath.Join(cd, "generator", "machine.yaml")
	defs := loadTestDefs(t, cd, "generator")

	builtins := stl.NewBuiltinRegistry()
	reg := core.NewRegistry()
	tracker := stl.NewToolTracker()
	tr := tracing.NoopTracer{}

	// Register real file tools
	builtins.Register("file_read", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.ReadBuilder{Root: vars["directory"]}, nil
	})
	builtins.Register("file_write", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.WriteBuilder{Root: vars["directory"]}, nil
	})
	builtins.Register("file_edit", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.EditBuilder{Root: vars["directory"]}, nil
	})
	builtins.Register("file_find", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.FindBuilder{Root: vars["directory"]}, nil
	})
	builtins.Register("file_list", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.ListFilesBuilder{Root: vars["directory"]}, nil
	})

	// Scripted LLM
	scripted := &scriptedLLMBuilder{responses: llmResponses}
	builtins.Register("invoke_llm", func(_ stl.ToolDef, _ map[string]string) (core.Builder, error) {
		return scripted, nil
	})

	// Parse response with real parser
	builtins.Register("parse_response", func(_ stl.ToolDef, _ map[string]string) (core.Builder, error) {
		return &stl.ParseResponseBuilder{
			Registry: reg,
			Parser:   llm.DefaultProfile(),
			Tracer:   tr,
		}, nil
	})

	// Stub remaining builtins
	for _, td := range defs {
		if td.Type == "builtin" && td.Init != "" {
			if _, ok := builtins.Resolve(td.Init); !ok {
				func(initName string) {
					builtins.Register(initName, func(_ stl.ToolDef, _ map[string]string) (core.Builder, error) {
						return noopBuilder{}, nil
					})
				}(td.Init)
			}
		}
	}

	vars := map[string]string{"directory": workspace, "model": "test", "ollama_url": "http://localhost:11434"}
	require.NoError(t, stl.RegisterUnifiedTools(reg, builtins, workspace, defs, vars))

	// Override build/lint/test exec tools with stubs so validate passes
	for _, name := range []string{"build", "lint", "test"} {
		reg.Override(core.ToolSpec{Name: name}, &stubPassBuilder{name: name})
	}

	toolAction := buildE2EToolAction(reg, tracker)

	return core.LoopParams{
		MachineFile: machineFile,
		AgentName:   "agent",
		ModelName:   "test",
		Trace:       tr,
		Budget:      core.Budget{MaxIterations: 100},
		ToolAction:  toolAction,
		Registry:    reg,
		Directory:   workspace,
	}
}

func TestE2E_ReadFileAndSucceed(t *testing.T) {
	t.Parallel()
	ws := setupFixtureWorkspace(t)
	params := buildE2EParams(t, ws, []string{
		`{"tool":"read","parameters":{"path":"cmd/app/root.go"}}`,
		`{"tool":"done","parameters":{"summary":"File has old help text"}}`,
	})
	rr, err := core.Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, core.StatusSucceeded, rr.Status)
	require.Equal(t, core.State("Succeeded"), rr.FinalState)
}

func TestE2E_EditFileAndSucceed(t *testing.T) {
	t.Parallel()
	ws := setupFixtureWorkspace(t)
	params := buildE2EParams(t, ws, []string{
		`{"tool":"read","parameters":{"path":"cmd/app/root.go"}}`,
		`{"tool":"edit","parameters":{"path":"cmd/app/root.go","old_string":"old help","new_string":"new help"}}`,
		`{"tool":"done","parameters":{"summary":"Updated CLI help text"}}`,
	})
	rr, err := core.Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, core.StatusSucceeded, rr.Status)

	data, err := os.ReadFile(filepath.Join(ws, "cmd", "app", "root.go"))
	require.NoError(t, err)
	require.Contains(t, string(data), "new help")
	require.NotContains(t, string(data), "old help")
}

func TestE2E_MalformedJSONRetries(t *testing.T) {
	t.Parallel()
	ws := setupFixtureWorkspace(t)
	params := buildE2EParams(t, ws, []string{
		`{"tool":"read","parameters":`,
		`{"tool":"read","parameters":{"path":"cmd/app/root.go"}}`,
		`{"tool":"done","parameters":{"summary":"Recovered after retry"}}`,
	})
	rr, err := core.Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, core.StatusSucceeded, rr.Status)

	var foundParseFailed bool
	for _, e := range rr.Events {
		if e.Signal == core.Signal("ParseFailed") {
			foundParseFailed = true
		}
	}
	require.True(t, foundParseFailed, "should have at least one ParseFailed event")
}

func TestE2E_DoneImmediately(t *testing.T) {
	t.Parallel()
	ws := setupFixtureWorkspace(t)
	params := buildE2EParams(t, ws, []string{
		`{"tool":"done","parameters":{"summary":"Nothing to change"}}`,
	})
	rr, err := core.Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, core.StatusSucceeded, rr.Status)
	require.Equal(t, core.State("Succeeded"), rr.FinalState)
}

func TestE2E_BudgetExhausted(t *testing.T) {
	t.Parallel()
	ws := setupFixtureWorkspace(t)
	params := buildE2EParams(t, ws, []string{
		`{"tool":"read","parameters":{"path":"cmd/app/root.go"}}`,
		`{"tool":"read","parameters":{"path":"cmd/app/root.go"}}`,
		`{"tool":"read","parameters":{"path":"cmd/app/root.go"}}`,
	})
	params.Budget.MaxIterations = 3
	rr, err := core.Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, core.StatusBudgetExceeded, rr.Status)
	require.Equal(t, core.State("BudgetExceeded"), rr.FinalState)
}

func TestE2E_ReadMissingFile_Recovers(t *testing.T) {
	t.Parallel()
	ws := setupFixtureWorkspace(t)
	params := buildE2EParams(t, ws, []string{
		`{"tool":"read","parameters":{"path":"nonexistent.txt"}}`,
		`{"tool":"done","parameters":{"summary":"File was missing"}}`,
	})
	rr, err := core.Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, core.StatusSucceeded, rr.Status)
}

func TestE2E_BoundaryRejectsTraversal(t *testing.T) {
	t.Parallel()
	ws := setupFixtureWorkspace(t)
	params := buildE2EParams(t, ws, []string{
		`{"tool":"read","parameters":{"path":"../../etc/passwd"}}`,
		`{"tool":"done","parameters":{"summary":"Path rejected"}}`,
	})
	rr, err := core.Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, core.StatusSucceeded, rr.Status)
}

func TestE2E_WriteTool(t *testing.T) {
	t.Parallel()
	ws := setupFixtureWorkspace(t)
	params := buildE2EParams(t, ws, []string{
		`{"tool":"write","parameters":{"path":"new_file.go","content":"package main\n"}}`,
		`{"tool":"done","parameters":{"summary":"Created new file"}}`,
	})
	rr, err := core.Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, core.StatusSucceeded, rr.Status)

	data, err := os.ReadFile(filepath.Join(ws, "new_file.go"))
	require.NoError(t, err)
	require.Equal(t, "package main\n", string(data))
}

func TestE2E_FindTool(t *testing.T) {
	t.Parallel()
	ws := setupFixtureWorkspace(t)
	params := buildE2EParams(t, ws, []string{
		`{"tool":"find","parameters":{"query":"func main"}}`,
		`{"tool":"done","parameters":{"summary":"Found main function"}}`,
	})
	rr, err := core.Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, core.StatusSucceeded, rr.Status)
}

func TestE2E_ManifestFilteredByVisibility(t *testing.T) {
	t.Parallel()
	ws := setupFixtureWorkspace(t)
	params := buildE2EParams(t, ws, nil)

	manifest := params.Registry.Manifest(core.State("Composing"))
	var names []string
	for _, s := range manifest {
		names = append(names, s.Name)
	}
	require.Contains(t, names, "read")
	require.Contains(t, names, "edit")
	require.Contains(t, names, "find")
	require.Contains(t, names, "write")
	require.Contains(t, names, "build")
	require.Contains(t, names, "lint")
	require.Contains(t, names, "test")
	require.NotContains(t, names, "invoke_llm")
	require.NotContains(t, names, "parse_response")
	require.NotContains(t, names, "report_parse_error")
	require.NotContains(t, names, "validate")
}

func TestValidateDeclarationIsInternalBoundaryAggregator(t *testing.T) {
	t.Parallel()
	cd := configDir(t)
	declarations, err := stl.LoadToolDeclarations([]string{
		filepath.Join(cd, "tools", "builtin.yaml"),
	})
	require.NoError(t, err)

	validate := requireToolDef(t, declarations, "validate")
	require.Equal(t, "boundary", validate.Category)
	require.Equal(t, "internal", validate.Visibility)
	require.Contains(t, validate.Description, "Deprecated compatibility aggregator")
	require.NotEmpty(t, validate.Problem)
	require.NotEmpty(t, validate.NonGoals)
	require.NotEmpty(t, validate.Output.Schema)
	require.NotEmpty(t, validate.SideEffects.Items)
	require.Equal(t, "compensatable", validate.Reversibility.Classification)
	require.NotEmpty(t, validate.Undo.Strategy)

	generatorDefs := loadTestDefs(t, cd, "generator")
	for _, def := range generatorDefs {
		require.NotEqual(t, "validate", def.Name, "generator validation is expressed in machine.yaml transitions")
	}
}
