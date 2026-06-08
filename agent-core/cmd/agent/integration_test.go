// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/stl"
)

// configDir resolves the absolute path to the configs/ directory relative to this test file.
func configDir(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../configs")
	require.NoError(t, err)
	info, err := os.Stat(abs)
	require.NoError(t, err, "configs directory must exist")
	require.True(t, info.IsDir())
	return abs
}

// noopBuilder is a trivial Builder used to satisfy registry wiring without executing anything.
type noopBuilder struct{}

func (noopBuilder) Build(_ core.Result) core.Command { return noopCmd{} }

type noopCmd struct{}

func (noopCmd) Name() string        { return "noop" }
func (noopCmd) Execute() core.Result { return core.Result{Signal: core.ToolDone} }

// stubFactory returns a BuiltinFactory that always produces a noopBuilder.
func stubFactory() stl.BuiltinFactory {
	return func(_ stl.ToolDef, _ map[string]string) (core.Builder, error) {
		return noopBuilder{}, nil
	}
}

// registerAllAsStubs registers stub factories for every init name found in
// the tool definitions, skipping any already present in the builtin registry.
func registerAllAsStubs(br *stl.BuiltinRegistry, defs []stl.ToolDef) {
	registered := make(map[string]bool)
	for _, n := range br.Names() {
		registered[n] = true
	}
	for _, td := range defs {
		if td.Type == "builtin" && td.Init != "" && !registered[td.Init] {
			br.Register(td.Init, stubFactory())
			registered[td.Init] = true
		}
	}
}

// buildRegistryForDefs creates a fully wired Registry from tool definitions,
// using real builtin factories where available and stubs for the rest.
func buildRegistryForDefs(t *testing.T, defs []stl.ToolDef) *core.Registry {
	t.Helper()
	builtins := stl.NewBuiltinRegistry()
	registerBuiltinFactories(builtins)
	registerAllAsStubs(builtins, defs)

	reg := core.NewRegistry()
	vars := map[string]string{"directory": t.TempDir(), "model": "test", "ollama_url": "http://localhost:11434"}
	err := stl.RegisterUnifiedTools(reg, builtins, vars["directory"], defs, vars)
	require.NoError(t, err)
	return reg
}

// dummyToolAction is a stand-in ActionFunc for the $tool dynamic dispatch transitions.
func dummyToolAction(_ core.Result) core.Command { return noopCmd{} }

// assertToolNames checks that every name in want appears in the loaded tool definitions.
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

// ---------------------------------------------------------------------------
// Generate config tests
// ---------------------------------------------------------------------------

func TestGenerateConfig_MachineLoads(t *testing.T) {
	path := filepath.Join(configDir(t), "generate", "machine.yaml")
	spec, err := core.LoadMachineSpec(path)
	require.NoError(t, err)

	require.Equal(t, "generate", spec.Name)
	require.Equal(t, "Idle", spec.InitialState)
	require.Contains(t, spec.TerminalStates, "Succeeded")
	require.Contains(t, spec.TerminalStates, "Failed")
	require.Contains(t, spec.TerminalStates, "BudgetExceeded")
	require.NotEmpty(t, spec.States)
	require.NotEmpty(t, spec.Transitions)
}

func TestGenerateConfig_ToolsLoad(t *testing.T) {
	path := filepath.Join(configDir(t), "generate", "tools.yaml")
	defs, err := stl.LoadToolDefs(path)
	require.NoError(t, err)
	require.NotEmpty(t, defs)

	assertToolNames(t, defs, []string{
		"read", "write", "edit", "find", "list_files",
		"build", "vet", "lint", "test",
		"invoke_llm", "parse_response", "validate", "done",
	})
}

func TestGenerateConfig_TransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "generate", "machine.yaml"))
	require.NoError(t, err)

	defs, err := stl.LoadToolDefs(filepath.Join(cd, "generate", "tools.yaml"))
	require.NoError(t, err)

	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, dummyToolAction)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.NotNil(t, isTerminal)
	require.True(t, isTerminal(core.State("Succeeded")))
	require.False(t, isTerminal(core.State("Idle")))
}

// ---------------------------------------------------------------------------
// Pipeline config tests
// ---------------------------------------------------------------------------

func TestPipelineConfig_MachineLoads(t *testing.T) {
	path := filepath.Join(configDir(t), "pipeline", "machine.yaml")
	spec, err := core.LoadMachineSpec(path)
	require.NoError(t, err)

	require.Equal(t, "pipeline", spec.Name)
	require.Equal(t, "Idle", spec.InitialState)
	require.Contains(t, spec.TerminalStates, "Completed")
	require.Contains(t, spec.TerminalStates, "Failed")
	require.NotEmpty(t, spec.States)
	require.NotEmpty(t, spec.Transitions)
}

func TestPipelineConfig_PassthroughLoads(t *testing.T) {
	path := filepath.Join(configDir(t), "pipeline", "machine-passthrough.yaml")
	spec, err := core.LoadMachineSpec(path)
	require.NoError(t, err)

	require.Equal(t, "pipeline-passthrough", spec.Name)
	require.Equal(t, "Idle", spec.InitialState)
	require.Contains(t, spec.TerminalStates, "Completed")
	require.Contains(t, spec.TerminalStates, "Failed")
	require.Contains(t, spec.TerminalStates, "Stalled")
}

func TestPipelineConfig_PlanOnlyLoads(t *testing.T) {
	path := filepath.Join(configDir(t), "pipeline", "machine-plan-only.yaml")
	spec, err := core.LoadMachineSpec(path)
	require.NoError(t, err)

	require.Equal(t, "pipeline-plan-only", spec.Name)
	require.Equal(t, "Idle", spec.InitialState)
	require.Contains(t, spec.TerminalStates, "Completed")
	require.Contains(t, spec.TerminalStates, "Failed")
	require.Contains(t, spec.TerminalStates, "Stalled")
	require.Contains(t, spec.TerminalStates, "Paused")
}

func TestPipelineConfig_ToolsLoad(t *testing.T) {
	path := filepath.Join(configDir(t), "pipeline", "tools.yaml")
	defs, err := stl.LoadToolDefs(path)
	require.NoError(t, err)
	require.NotEmpty(t, defs)

	assertToolNames(t, defs, []string{
		"extract_task", "extract_all", "assemble_prompt", "parse_plan",
		"create_issue", "execute_task", "invoke_llm", "reset_history",
		"check_result", "vet", "build", "test",
		"stage_all", "workspace_status", "commit", "rev_parse",
		"diff_stat", "log_oneline", "issue_create", "issue_close", "issue_list",
	})
}

func TestPipelineConfig_TransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "pipeline", "machine.yaml"))
	require.NoError(t, err)

	defs, err := stl.LoadToolDefs(filepath.Join(cd, "pipeline", "tools.yaml"))
	require.NoError(t, err)

	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, nil)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.True(t, isTerminal(core.State("Completed")))
	require.False(t, isTerminal(core.State("Idle")))
}

func TestPipelineConfig_PassthroughTransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "pipeline", "machine-passthrough.yaml"))
	require.NoError(t, err)

	defs, err := stl.LoadToolDefs(filepath.Join(cd, "pipeline", "tools.yaml"))
	require.NoError(t, err)

	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, nil)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.True(t, isTerminal(core.State("Completed")))
}

func TestPipelineConfig_PlanOnlyTransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "pipeline", "machine-plan-only.yaml"))
	require.NoError(t, err)

	defs, err := stl.LoadToolDefs(filepath.Join(cd, "pipeline", "tools.yaml"))
	require.NoError(t, err)

	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, nil)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.True(t, isTerminal(core.State("Completed")))
	require.True(t, isTerminal(core.State("Paused")))
}

// ---------------------------------------------------------------------------
// Eval config tests
// ---------------------------------------------------------------------------

func TestEvalConfig_GenerateSpecLoads(t *testing.T) {
	path := filepath.Join(configDir(t), "eval", "generate-spec.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var gen core.GenerateSpec
	err = yaml.Unmarshal(data, &gen)
	require.NoError(t, err)

	require.Equal(t, "eval", gen.Name)
	require.NotEmpty(t, gen.Points)
	require.Equal(t, "summarize", gen.DoneAction)

	point := gen.Points[0]
	require.Equal(t, []string{"prepare_workspace", "run_agent", "check_results", "collect_metrics"}, point.Steps)
	require.NotEmpty(t, point.Vars)
}

func TestEvalConfig_GenerateLinearMachine(t *testing.T) {
	path := filepath.Join(configDir(t), "eval", "generate-spec.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var gen core.GenerateSpec
	err = yaml.Unmarshal(data, &gen)
	require.NoError(t, err)

	spec := core.GenerateLinearMachine(gen)

	require.Equal(t, "eval", spec.Name)
	require.Equal(t, "Point_0_Step_0", spec.InitialState)
	require.Contains(t, spec.TerminalStates, "Done")
	require.Contains(t, spec.States, "Point_0_Step_0")
	require.Contains(t, spec.States, "Point_0_Step_1")
	require.Contains(t, spec.States, "Point_0_Step_2")
	require.Contains(t, spec.States, "Point_0_Step_3")
	require.Contains(t, spec.States, "Summarize")
	require.Contains(t, spec.States, "Done")
	require.NotEmpty(t, spec.Transitions)

	// The generated machine must be self-consistent (ParseMachineSpec validates it).
	out, err := core.MarshalMachineSpec(spec)
	require.NoError(t, err)
	_, err = core.ParseMachineSpec(out)
	require.NoError(t, err)
}

func TestEvalConfig_ToolsLoad(t *testing.T) {
	path := filepath.Join(configDir(t), "eval", "tools.yaml")
	defs, err := stl.LoadToolDefs(path)
	require.NoError(t, err)
	require.NotEmpty(t, defs)

	assertToolNames(t, defs, []string{
		"prepare_workspace", "run_agent", "check_results",
		"collect_metrics", "summarize",
	})
}

func TestEvalConfig_TransitionTable(t *testing.T) {
	cd := configDir(t)

	specData, err := os.ReadFile(filepath.Join(cd, "eval", "generate-spec.yaml"))
	require.NoError(t, err)

	var gen core.GenerateSpec
	err = yaml.Unmarshal(specData, &gen)
	require.NoError(t, err)

	machineSpec := core.GenerateLinearMachine(gen)

	defs, err := stl.LoadToolDefs(filepath.Join(cd, "eval", "tools.yaml"))
	require.NoError(t, err)

	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(machineSpec, reg, nil)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.True(t, isTerminal(core.State("Done")))
	require.False(t, isTerminal(core.State("Point_0_Step_0")))
}
