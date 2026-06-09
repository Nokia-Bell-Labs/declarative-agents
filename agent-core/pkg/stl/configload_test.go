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

// loadTestDefs loads shared tool declarations + per-agent LLM defaults and
// applies the agent's selection file.
func loadTestDefs(t *testing.T, cd, agent string) []stl.ToolDef {
	t.Helper()
	declPaths := []string{
		filepath.Join(cd, "tools", "builtin.yaml"),
		filepath.Join(cd, "tools", "exec.yaml"),
	}
	llmDefault := filepath.Join(cd, agent, "llm", "default.yaml")
	if _, err := os.Stat(llmDefault); err == nil {
		declPaths = append(declPaths, llmDefault)
	}
	declarations, err := stl.LoadToolDeclarations(declPaths)
	require.NoError(t, err)
	selection, err := stl.LoadToolSelection(filepath.Join(cd, agent, "tools.yaml"))
	require.NoError(t, err)
	defs, err := stl.SelectTools(declarations, selection)
	require.NoError(t, err)
	return defs
}

type noopBuilder struct{}

func (noopBuilder) Build(_ core.Result) core.Command { return noopCmd{} }

type noopCmd struct{}

func (noopCmd) Name() string        { return "noop" }
func (noopCmd) Execute() core.Result { return core.Result{Signal: core.ToolDone} }

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
}

func TestGeneratorConfig_ToolsLoad(t *testing.T) {
	cd := configDir(t)
	defs := loadTestDefs(t, cd, "generator")
	require.NotEmpty(t, defs)

	assertToolNames(t, defs, []string{
		"read", "write", "edit", "find", "list_files",
		"build", "vet", "lint", "test",
		"invoke_llm", "parse_response", "validate", "done",
	})
}

func TestGeneratorConfig_TransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "generator", "machine.yaml"))
	require.NoError(t, err)

	defs := loadTestDefs(t, cd, "generator")
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
}

func TestGeneratorConfig_DeepseekTransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "generator", "deepseek-coding-agent.yaml"))
	require.NoError(t, err)

	defs := loadTestDefs(t, cd, "generator")
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
		"stage_all", "workspace_status", "commit", "rev_parse",
		"diff_stat", "log_oneline", "issue_create", "issue_close", "issue_list",
	})
}

func TestPlannerConfig_TransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "planner", "machine.yaml"))
	require.NoError(t, err)

	defs := loadTestDefs(t, cd, "planner")
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
	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, nil)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.True(t, isTerminal(core.State("Completed")))
	require.True(t, isTerminal(core.State("Paused")))
}

// ---------------------------------------------------------------------------
// Evaluator config tests
// ---------------------------------------------------------------------------

func TestEvaluatorConfig_ToolsLoad(t *testing.T) {
	cd := configDir(t)
	defs := loadTestDefs(t, cd, "evaluator")
	require.NotEmpty(t, defs)

	assertToolNames(t, defs, []string{
		"prepare_workspace",
		"run_agent", "check_results", "collect_metrics",
	})
}

func TestEvaluatorConfig_TransitionTable(t *testing.T) {
	cd := configDir(t)
	spec, err := core.LoadMachineSpec(filepath.Join(cd, "evaluator", "machine.yaml"))
	require.NoError(t, err)

	defs := loadTestDefs(t, cd, "evaluator")
	reg := buildRegistryForDefs(t, defs)

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, nil)
	require.NoError(t, err)
	require.NotEmpty(t, table)
	require.True(t, isTerminal(core.State("Done")))
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

func (s *scriptedCmd) Name() string { return "invoke_llm" }
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

func (s *stubPassCmd) Name() string { return s.name }
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

func (f *failCmd) Name() string { return "fail" }
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

	// Override validate to use real ValidateBuilder with stubbed tools
	builtins.Override("validate", func(_ stl.ToolDef, _ map[string]string) (core.Builder, error) {
		return &stl.ValidateBuilder{
			Tracker:  tracker,
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
