// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/stl"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
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

// buildRegistryForDefs creates a fully wired Registry from tool definitions,
// using real builtin factories for file/done tools and stubs for everything else.
// In integration tests we don't have an Ollama server or pipeline/eval implementations,
// so we use stubs for those tools to verify config wiring only.
func buildRegistryForDefs(t *testing.T, defs []stl.ToolDef) *core.Registry {
	t.Helper()
	builtins := stl.NewBuiltinRegistry()
	reg := core.NewRegistry()

	st := &agentState{
		registry:      reg,
		tracer:        tracing.NoopTracer{},
		tracker:       stl.NewToolTracker(),
		conversation:  llm.NewConversation(nil, "", llm.ChatOptions{}),
		conversations: stl.NewConversationStore(),
		directory:     t.TempDir(),
	}
	registerBuiltinFactories(builtins, st)

	// Override factories that would error (no Ollama, no pipeline/eval impl)
	// with stubs for config-wiring tests.
	overrideWithStubs(builtins, defs)

	vars := map[string]string{"directory": st.directory, "model": "test", "ollama_url": "http://localhost:11434"}
	err := stl.RegisterUnifiedTools(reg, builtins, vars["directory"], defs, vars)
	require.NoError(t, err)
	return reg
}

// overrideWithStubs replaces registered factories with stubs for any
// builtin tool whose factory would error in a test environment (no LLM,
// no pipeline/eval implementations). Uses BuiltinRegistry.Override.
func overrideWithStubs(br *stl.BuiltinRegistry, defs []stl.ToolDef) {
	for _, td := range defs {
		if td.Type == "builtin" && td.Init != "" {
			br.Override(td.Init, stubFactory())
		}
	}
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
// End-to-end loop tests with real generate configs and scripted LLM
// ---------------------------------------------------------------------------

// scriptedLLMBuilder returns canned LLM responses in sequence,
// emulating what a real LLM would produce without needing Ollama.
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

func (s *scriptedCmd) Name() string { return "invoke_llm" }
func (s *scriptedCmd) Execute() core.Result {
	return core.Result{
		Output:      s.output,
		Signal:      core.Signal("LLMResponded"),
		CommandName: "invoke_llm",
	}
}

// stubPassBuilder always builds a Command that returns ToolDone.
type stubPassBuilder struct{ name string }

func (s *stubPassBuilder) Build(_ core.Result) core.Command {
	return &stubPassCmd{name: s.name}
}

type stubPassCmd struct{ name string }

func (s *stubPassCmd) Name() string { return s.name }
func (s *stubPassCmd) Execute() core.Result {
	return core.Result{
		Signal:      core.ToolDone,
		Output:      s.name + " passed",
		CommandName: s.name,
	}
}

// setupFixtureWorkspace creates a minimal Go CLI workspace for testing.
func setupFixtureWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	appDir := root + "/cmd/app"
	require.NoError(t, os.MkdirAll(appDir, 0o755))
	require.NoError(t, os.WriteFile(root+"/go.mod", []byte("module fixture\n\ngo 1.23\n"), 0o644))
	require.NoError(t, os.WriteFile(appDir+"/root.go", []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"old help\")\n}\n"), 0o644))
	return root
}

// buildE2EParams loads the generate machine/tools configs and wires
// scripted LLM responses with real file tools + stub validation.
func buildE2EParams(t *testing.T, workspace string, llmResponses []string) core.LoopParams {
	t.Helper()
	cd := configDir(t)
	machineFile := filepath.Join(cd, "generator", "machine.yaml")
	toolsFile := filepath.Join(cd, "generator", "tools.yaml")

	defs, err := stl.LoadToolDefs(toolsFile)
	require.NoError(t, err)

	builtins := stl.NewBuiltinRegistry()
	reg := core.NewRegistry()
	tr := tracing.NoopTracer{}

	st := &agentState{
		registry:      reg,
		tracer:        tr,
		tracker:       stl.NewToolTracker(),
		conversation:  llm.NewConversation(nil, "", llm.ChatOptions{}),
		conversations: stl.NewConversationStore(),
		directory:     workspace,
	}
	registerBuiltinFactories(builtins, st)

	// Replace invoke_llm with scripted responses
	scripted := &scriptedLLMBuilder{responses: llmResponses}
	builtins.Override("invoke_llm", func(_ stl.ToolDef, _ map[string]string) (core.Builder, error) {
		return scripted, nil
	})

	builtins.Override("validate", func(_ stl.ToolDef, _ map[string]string) (core.Builder, error) {
		return &stl.ValidateBuilder{
			Tracker:  st.tracker,
			Registry: reg,
			Tracer:   tr,
		}, nil
	})

	vars := map[string]string{"directory": workspace, "model": "test", "ollama_url": "http://localhost:11434"}
	require.NoError(t, stl.RegisterUnifiedTools(reg, builtins, workspace, defs, vars))

	// Override build/lint/test exec tools with stubs so validate passes
	for _, name := range []string{"build", "lint", "test"} {
		reg.Override(core.ToolSpec{Name: name}, &stubPassBuilder{name: name})
	}

	toolAction := buildToolAction(st, reg)

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

// ---------------------------------------------------------------------------
// Generate config tests
// ---------------------------------------------------------------------------

func TestGenerateConfig_MachineLoads(t *testing.T) {
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
}

func TestGenerateConfig_ToolsLoad(t *testing.T) {
	path := filepath.Join(configDir(t), "generator", "tools.yaml")
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
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "generator", "machine.yaml"))
	require.NoError(t, err)

	defs, err := stl.LoadToolDefs(filepath.Join(cd, "generator", "tools.yaml"))
	require.NoError(t, err)

	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, dummyToolAction)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.NotNil(t, isTerminal)
	require.True(t, isTerminal(core.State("Succeeded")))
	require.False(t, isTerminal(core.State("Idle")))
}

func TestGenerateConfig_DeepseekMachineLoads(t *testing.T) {
	path := filepath.Join(configDir(t), "generator", "deepseek-coding-agent.yaml")
	spec, err := core.LoadMachineSpec(path)
	require.NoError(t, err)

	require.Equal(t, "generator-deepseek", spec.Name)
	require.Equal(t, "Idle", spec.InitialState)
	require.Contains(t, spec.TerminalStates, "Succeeded")
	require.Contains(t, spec.TerminalStates, "Failed")
	require.Contains(t, spec.TerminalStates, "BudgetExceeded")
}

func TestGenerateConfig_DeepseekTransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "generator", "deepseek-coding-agent.yaml"))
	require.NoError(t, err)

	defs, err := stl.LoadToolDefs(filepath.Join(cd, "generator", "tools.yaml"))
	require.NoError(t, err)

	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, dummyToolAction)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.NotNil(t, isTerminal)
	require.True(t, isTerminal(core.State("Succeeded")))

	// Deepseek machine should have nudge_reread wired for EditDone
	key := core.TransitionInput{State: core.State("Composing"), Signal: core.Signal("EditDone")}
	tv, ok := table[key]
	require.True(t, ok, "deepseek machine must handle EditDone in Composing")
	require.Equal(t, core.State("Composing"), tv.NextState)
}

// ---------------------------------------------------------------------------
// Pipeline config tests
// ---------------------------------------------------------------------------

func TestPipelineConfig_MachineLoads(t *testing.T) {
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

func TestPipelineConfig_PassthroughLoads(t *testing.T) {
	path := filepath.Join(configDir(t), "planner", "machine-passthrough.yaml")
	spec, err := core.LoadMachineSpec(path)
	require.NoError(t, err)

	require.Equal(t, "planner-passthrough", spec.Name)
	require.Equal(t, "Idle", spec.InitialState)
	require.Contains(t, spec.TerminalStates, "Completed")
	require.Contains(t, spec.TerminalStates, "Failed")
	require.Contains(t, spec.TerminalStates, "Stalled")
}

func TestPipelineConfig_PlanOnlyLoads(t *testing.T) {
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

func TestPipelineConfig_ToolsLoad(t *testing.T) {
	path := filepath.Join(configDir(t), "planner", "tools.yaml")
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
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "planner", "machine.yaml"))
	require.NoError(t, err)

	defs, err := stl.LoadToolDefs(filepath.Join(cd, "planner", "tools.yaml"))
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
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "planner", "machine-passthrough.yaml"))
	require.NoError(t, err)

	defs, err := stl.LoadToolDefs(filepath.Join(cd, "planner", "tools.yaml"))
	require.NoError(t, err)

	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, nil)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.True(t, isTerminal(core.State("Completed")))
}

func TestPipelineConfig_PlanOnlyTransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "planner", "machine-plan-only.yaml"))
	require.NoError(t, err)

	defs, err := stl.LoadToolDefs(filepath.Join(cd, "planner", "tools.yaml"))
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
	path := filepath.Join(configDir(t), "evaluator", "generate-spec.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var gen core.GenerateSpec
	err = yaml.Unmarshal(data, &gen)
	require.NoError(t, err)

	require.Equal(t, "evaluator", gen.Name)
	require.NotEmpty(t, gen.Points)
	require.Equal(t, "summarize", gen.DoneAction)

	point := gen.Points[0]
	require.Equal(t, []string{"prepare_workspace", "run_agent", "check_results", "collect_metrics"}, point.Steps)
	require.NotEmpty(t, point.Vars)
}

func TestEvalConfig_GenerateLinearMachine(t *testing.T) {
	path := filepath.Join(configDir(t), "evaluator", "generate-spec.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var gen core.GenerateSpec
	err = yaml.Unmarshal(data, &gen)
	require.NoError(t, err)

	spec := core.GenerateLinearMachine(gen)

	require.Equal(t, "evaluator", spec.Name)
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
	path := filepath.Join(configDir(t), "evaluator", "tools.yaml")
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

	specData, err := os.ReadFile(filepath.Join(cd, "evaluator", "generate-spec.yaml"))
	require.NoError(t, err)

	var gen core.GenerateSpec
	err = yaml.Unmarshal(specData, &gen)
	require.NoError(t, err)

	machineSpec := core.GenerateLinearMachine(gen)

	defs, err := stl.LoadToolDefs(filepath.Join(cd, "evaluator", "tools.yaml"))
	require.NoError(t, err)

	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(machineSpec, reg, nil)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.True(t, isTerminal(core.State("Done")))
	require.False(t, isTerminal(core.State("Point_0_Step_0")))
}
