// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type shippedMachine struct {
	InitialState   string              `yaml:"initial_state"`
	TerminalStates []string            `yaml:"terminal_states"`
	Transitions    []shippedTransition `yaml:"transitions"`
}

type shippedTransition struct {
	Name   string `yaml:"-"`
	State  string `yaml:"state"`
	Signal string `yaml:"signal"`
	Next   string `yaml:"next"`
	Action string `yaml:"action"`
}

type shippedTools struct {
	Tools []string `yaml:"tools"`
}

type shippedProfile struct {
	Machine          string   `yaml:"machine"`
	Tools            []string `yaml:"tools"`
	ToolDeclarations []string `yaml:"tool_declarations"`
}

func TestExecutorConfig_MachineLoads(t *testing.T) {
	root := requireAgentProfilesRoot(t)
	for _, tc := range []struct {
		name    string
		profile string
		llm     string
	}{
		{name: "default", profile: "profile.yaml", llm: "llm/default.yaml"},
		{name: "qwen 35b", profile: "profile-qwen35b.yaml", llm: "llm/default.yaml"},
		{name: "qwen 27b", profile: "profile-qwen27b.yaml", llm: "llm/qwen27b.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var profile shippedProfile
			readShippedYAML(t, filepath.Join(root, "agents", "executor", tc.profile), &profile)
			require.Equal(t, "machine.yaml", profile.Machine)
			require.Contains(t, profile.Tools, "tools.yaml")
			require.Contains(t, profile.ToolDeclarations, tc.llm)
		})
	}
	machine := readShippedMachine(t, root, "executor", "machine.yaml")
	require.Equal(t, "Idle", machine.InitialState)
	require.ElementsMatch(t, []string{"Succeeded", "Failed", "BudgetExceeded"}, machine.TerminalStates)
}

func TestExecutorConfig_ToolsLoad(t *testing.T) {
	tools := readShippedTools(t, requireAgentProfilesRoot(t), "executor", "tools.yaml")
	require.ElementsMatch(t, []string{
		"read", "write", "edit", "find", "list_files",
		"invoke_llm", "parse_response", "report_parse_error", "reset_history",
		"nudge_reread", "done", "build", "vet", "lint", "test",
	}, tools)
}

func TestExecutorConfig_TransitionTable(t *testing.T) {
	machine := readShippedMachine(t, requireAgentProfilesRoot(t), "executor", "machine.yaml")
	requireShippedTransition(t, machine, "Parsing", "TaskCompleted", "ValidatingBuild", "build")
	requireShippedTransition(t, machine, "ValidatingBuild", "ToolDone", "ValidatingLint", "lint")
	requireShippedTransition(t, machine, "ValidatingLint", "ToolDone", "ValidatingTest", "test")
	requireShippedTransition(t, machine, "ValidatingTest", "ToolDone", "Succeeded", "")
}

func TestCriticConfig_SessionMachineLoads(t *testing.T) {
	root := requireAgentProfilesRoot(t)
	var profile shippedProfile
	readShippedYAML(t, filepath.Join(root, "agents", "critic", "profile.yaml"), &profile)
	require.Equal(t, "machine.yaml", profile.Machine)
	require.Contains(t, profile.Tools, "tools.yaml")
	machine := readShippedMachine(t, root, "critic", "machine.yaml")
	require.Equal(t, "Idle", machine.InitialState)
	require.ElementsMatch(t, []string{"Done", "Failed"}, machine.TerminalStates)
}

func TestCriticConfig_ToolsLoad(t *testing.T) {
	tools := readShippedTools(t, requireAgentProfilesRoot(t), "critic", "tools.yaml")
	require.Equal(t, []string{
		"parse_suite_config", "discover_suite_samples", "expand_eval_grid",
		"init_eval_session", "report_suite_summary", "next_point", "run_point", "report_session",
	}, tools)
}

func TestCriticConfig_TransitionTable(t *testing.T) {
	machine := readShippedMachine(t, requireAgentProfilesRoot(t), "critic", "machine.yaml")
	for _, tc := range []shippedTransition{
		{Name: "parse suite", State: "Idle", Signal: "Seed", Next: "ParsingSuiteConfig", Action: "parse_suite_config"},
		{Name: "advance point", State: "ReportingSuiteSummary", Signal: "SuiteLoaded", Next: "AdvancingPoint", Action: "next_point"},
		{Name: "run point", State: "AdvancingPoint", Signal: "PointReady", Next: "RunningPoint", Action: "run_point"},
		{Name: "report session", State: "AdvancingPoint", Signal: "AllPointsDone", Next: "Reporting", Action: "report_session"},
		{Name: "terminal", State: "Reporting", Signal: "SessionReported", Next: "Done"},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			requireShippedTransition(t, machine, tc.State, tc.Signal, tc.Next, tc.Action)
		})
	}
}

func TestCriticConfig_PointTransitionTable(t *testing.T) {
	root := requireAgentProfilesRoot(t)
	tools := readShippedTools(t, root, "critic", "tools-point.yaml")
	require.ElementsMatch(t, []string{
		"create_point_dir", "copy_sample_workspace", "copy_sample_docs",
		"init_workspace_repo", "stage_workspace_baseline", "commit_workspace_baseline",
		"dump_config", "run_agent", "run_oracle_check", "collect_trace_tokens",
		"check_agent_version", "summarize_point_results", "collect_metrics",
	}, tools)
	machine := readShippedMachine(t, root, "critic", "point.yaml")
	for _, tc := range []shippedTransition{
		{Name: "create point directory", State: "Idle", Signal: "Seed", Next: "CreatingPointDir", Action: "create_point_dir"},
		{Name: "copy workspace", State: "CreatingPointDir", Signal: "PointDirCreated", Next: "CopyingSampleWorkspace", Action: "copy_sample_workspace"},
		{Name: "initialize repository", State: "CopyingSampleDocs", Signal: "SampleDocsCopied", Next: "InitializingWorkspaceRepo", Action: "init_workspace_repo"},
		{Name: "stage baseline", State: "InitializingWorkspaceRepo", Signal: "WorkspaceRepoInitialized", Next: "StagingWorkspaceBaseline", Action: "stage_workspace_baseline"},
		{Name: "commit baseline", State: "StagingWorkspaceBaseline", Signal: "WorkspaceBaselineStaged", Next: "CommittingWorkspaceBaseline", Action: "commit_workspace_baseline"},
		{Name: "terminal", State: "CollectingMetrics", Signal: "MetricsCollected", Next: "Done"},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			requireShippedTransition(t, machine, tc.State, tc.Signal, tc.Next, tc.Action)
		})
	}
}

func TestBenchConfig_MachineLoads(t *testing.T) {
	root := requireAgentProfilesRoot(t)
	var profile shippedProfile
	readShippedYAML(t, filepath.Join(root, "agents", "bench", "profile.yaml"), &profile)
	require.Equal(t, "machine.yaml", profile.Machine)
	require.Contains(t, profile.Tools, "tools.yaml")
	machine := readShippedMachine(t, root, "bench", "machine.yaml")
	require.Equal(t, "Idle", machine.InitialState)
	require.ElementsMatch(t, []string{"Done", "Failed"}, machine.TerminalStates)
}

func TestBenchConfig_ToolsLoad(t *testing.T) {
	tools := readShippedTools(t, requireAgentProfilesRoot(t), "bench", "tools.yaml")
	require.Equal(t, []string{"serve_ui", "launch_eval"}, tools)
}

func TestBenchConfig_TransitionTable(t *testing.T) {
	machine := readShippedMachine(t, requireAgentProfilesRoot(t), "bench", "machine.yaml")
	for _, tc := range []shippedTransition{
		{Name: "serve UI", State: "Idle", Signal: "Seed", Next: "Serving", Action: "serve_ui"},
		{Name: "launch evaluation", State: "Serving", Signal: "ExperimentRequested", Next: "Launching", Action: "launch_eval"},
		{Name: "evaluation completed", State: "Launching", Signal: "EvalCompleted", Next: "Serving", Action: "serve_ui"},
		{Name: "evaluation failed", State: "Launching", Signal: "EvalFailed", Next: "Serving", Action: "serve_ui"},
		{Name: "shutdown", State: "Serving", Signal: "Shutdown", Next: "Done"},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			requireShippedTransition(t, machine, tc.State, tc.Signal, tc.Next, tc.Action)
		})
	}
}

func requireAgentProfilesRoot(t *testing.T) string {
	t.Helper()
	if explicit := os.Getenv("AGENT_PROFILES_ROOT"); explicit != "" {
		requireDirectory(t, explicit)
		return explicit
	}
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "agent-profiles"))
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Skipf("agent-profiles checkout unavailable; set AGENT_PROFILES_ROOT (resolved %s)", root)
	}
	return root
}

func requireDirectory(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, info.IsDir(), "%s is not a directory", path)
}

func readShippedMachine(t *testing.T, root, profile, name string) shippedMachine {
	t.Helper()
	var machine shippedMachine
	readShippedYAML(t, filepath.Join(root, "agents", profile, name), &machine)
	return machine
}

func readShippedTools(t *testing.T, root, profile, name string) []string {
	t.Helper()
	var tools shippedTools
	readShippedYAML(t, filepath.Join(root, "agents", profile, name), &tools)
	return tools.Tools
}

func readShippedYAML(t *testing.T, path string, target interface{}) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, target))
}

func requireShippedTransition(t *testing.T, machine shippedMachine, state, signal, next, action string) {
	t.Helper()
	for _, transition := range machine.Transitions {
		if transition.State == state && transition.Signal == signal &&
			transition.Next == next && transition.Action == action {
			return
		}
	}
	t.Fatalf("missing transition %s + %s -> %s / %s", state, signal, next, action)
}
