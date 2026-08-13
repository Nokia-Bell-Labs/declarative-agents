// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"context"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/stretchr/testify/require"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestSeedRequestSuppliesUniversalRequestPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "request.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"saga_id":"trace-1"}`), 0o644))
	var params core.LoopParams

	require.NoError(t, seedRequest(&params, path))

	require.Equal(t, core.Seed, params.InitialSignal)
	require.Equal(t, core.Seed, params.InitialResult.Signal)
	require.JSONEq(t, `{"saga_id":"trace-1"}`, params.InitialResult.Output)
}

func TestSeedRequestWithoutPathLeavesBootstrapUnchanged(t *testing.T) {
	params := core.LoopParams{
		InitialSignal: core.Signal("Existing"),
		InitialResult: core.Result{Signal: core.Signal("Existing"), Output: "existing"},
	}

	require.NoError(t, seedRequest(&params, " \t"))

	require.Equal(t, core.Signal("Existing"), params.InitialSignal)
	require.Equal(t, "existing", params.InitialResult.Output)
}

func TestSeedRequestRejectsUnreadableFile(t *testing.T) {
	var params core.LoopParams

	err := seedRequest(&params, filepath.Join(t.TempDir(), "missing-request.yaml"))

	require.ErrorContains(t, err, "read --request file")
	require.Empty(t, params.InitialSignal)
	require.Empty(t, params.InitialResult.Output)
}

func TestResumeLoadOverridesRequestSeed(t *testing.T) {
	checkpoint := &core.InMemoryCheckpoint{}
	require.NoError(t, checkpoint.Save(core.Position{CurrentState: "Start"}, nil))
	params := terminalLoopParams()
	params.Checkpoint = checkpoint
	params.InitialSignal = core.Seed
	params.InitialResult = core.Result{Signal: core.Seed, Output: "new request bytes"}
	params.Table = core.TransitionTable{
		{State: "Start", Signal: core.Seed}:     {NextState: "WrongSeedPath"},
		{State: "Start", Signal: core.Approved}: {NextState: "Resumed"},
	}
	params.IsTerminal = func(state core.State) bool {
		return state == "WrongSeedPath" || state == "Resumed"
	}
	params.Hooks.TerminalStatus = func(core.State) core.RunStatus { return core.StatusSucceeded }

	result, err := runOrResume(runtimeConfig{
		ResumeCheckpoint: "run-1", ResumeSignal: string(core.Approved),
	}, resumeDeps{Params: params, State: &agentState{}, Ctx: context.Background()})

	require.NoError(t, err)
	require.Equal(t, core.State("Resumed"), result.FinalState)
	require.Equal(t, core.StatusSucceeded, result.Status)
}

func TestCLIResultReporterDoesNotInventUndeclaredSummary(t *testing.T) {
	got := cliResultReporter(core.RunResult{}, core.Result{
		Signal: core.Signal("ResponseReady"), Output: `{"verdict":"pass"}`,
	})

	require.Empty(t, got.Summary)
}

func TestCLIResultReporterBoundsDeclaredMachineSummary(t *testing.T) {
	machine := &core.MachineSpec{SummarySignal: "ResponseReady"}
	got := cliResultReporterForMachine(machine)(
		core.RunResult{Summary: strings.Repeat("x", terminalSummaryMaxBytes+100)},
		core.Result{Signal: core.Signal("ResponseReady")},
	)

	require.Len(t, got.Summary, terminalSummaryMaxBytes)
	require.True(t, strings.HasSuffix(got.Summary, terminalSummaryTruncated))
}

func TestRunPreparedPrintsSummaryFinalStateAndMappedExit(t *testing.T) {
	originalExitCode := runExitCode
	t.Cleanup(func() { runExitCode = originalExitCode })
	builder := staticSignalBuilder{
		name: "respond", signal: core.ToolDone, output: `{"answer":"done"}`,
	}
	params := terminalLoopParams()
	params.Hooks.OnResult = cliResultReporter
	params.Hooks.TaskCompletedSignal = core.ToolDone
	params.Hooks.TerminalStatus = func(core.State) core.RunStatus { return core.StatusSucceeded }
	params.Table = core.TransitionTable{
		{State: "Start", Signal: core.Seed}: {
			NextState: "Finished",
			Action: func(result core.Result) core.Command {
				return builder.Build(result)
			},
		},
	}
	_, cancel := context.WithCancel(context.Background())
	prepared := preparedRun{
		Config: runtimeConfig{}, Params: params, State: &agentState{},
		Ctx: context.Background(), Cancel: cancel, Shutdown: newDeferredShutdown(cancel),
	}
	var stderr string

	stdout, err := captureStdout(t, func() error {
		var runErr error
		stderr, runErr = captureStderr(t, func() error { return runPrepared(prepared) })
		return runErr
	})

	require.NoError(t, err)
	require.Equal(t, "{\"answer\":\"done\"}\n", stdout)
	require.Contains(t, stderr, "terminal state: succeeded\n")
	require.Contains(t, stderr, "final machine state: Finished\n")
	require.Equal(t, ExitSucceeded, runExitCode)
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runErr := fn()
	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, readErr := buf.ReadFrom(r)
	require.NoError(t, readErr)
	require.NoError(t, r.Close())
	return buf.String(), runErr
}

func TestMainRuntimeDoesNotBranchOnAgentModeNames(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}
	path := filepath.Join(filepath.Dir(currentFile), "main.go")
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	modeNames := map[string]struct{}{
		"executor": {},
		"planner":  {},
		"critic":   {},
		"bench":    {},
		"jurist":   {},
	}
	isModeLiteral := func(expr ast.Expr) (string, bool) {
		lit, ok := expr.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatalf("unquote %s: %v", lit.Value, err)
		}
		_, isMode := modeNames[value]
		return value, isMode
	}

	ast.Inspect(parsed, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BinaryExpr:
			if node.Op != token.EQL && node.Op != token.NEQ {
				return true
			}
			if value, ok := isModeLiteral(node.X); ok {
				t.Fatalf("cmd/agent must not branch on agent mode literal %q at %s; select behavior through machine/tools YAML", value, fset.Position(node.Pos()))
			}
			if value, ok := isModeLiteral(node.Y); ok {
				t.Fatalf("cmd/agent must not branch on agent mode literal %q at %s; select behavior through machine/tools YAML", value, fset.Position(node.Pos()))
			}
		case *ast.CaseClause:
			for _, expr := range node.List {
				if value, ok := isModeLiteral(expr); ok {
					t.Fatalf("cmd/agent must not switch on agent mode literal %q at %s; selected tool init gates are the allowed bootstrap boundary", value, fset.Position(expr.Pos()))
				}
			}
		}
		return true
	})
}

func TestBuiltinFactoryCatalogSelectsEntriesByInit(t *testing.T) {
	t.Parallel()

	catalog := builtinFactoryCatalog(&agentState{})
	byName := make(map[string]builtinFactoryCatalogEntry, len(catalog))
	for _, entry := range catalog {
		byName[entry.Name] = entry
	}

	require.True(t, byName["planning"].selectedBy(map[string]bool{"mark_nodes_executing": true}))
	require.True(t, byName["evaluation"].selectedBy(map[string]bool{"run_point": true}))
	require.True(t, byName["evaluation"].selectedBy(map[string]bool{"list_evaluation_sessions": true}))
	require.True(t, byName["spec_validation"].selectedBy(map[string]bool{"validate_specs": true}))
	require.True(t, byName["lifecycle"].selectedBy(map[string]bool{"checkpoint_history": true}))
	require.True(t, byName["lifecycle"].selectedBy(map[string]bool{"checkpoint_rollback": true}))
	require.True(t, byName["rest"].selectedBy(map[string]bool{"rest_server_launch": true}))
	require.True(t, byName["rest"].selectedBy(map[string]bool{"rest_server_stop": true}))
	require.False(t, byName["planning"].selectedBy(map[string]bool{"list_evaluation_sessions": true}))
}

func TestBuiltinFactoryCatalogCoversSelectedActiveInits(t *testing.T) {
	t.Parallel()

	catalog := builtinFactoryCatalog(&agentState{})
	covered := make(map[string]bool)
	for _, entry := range catalog {
		for _, init := range entry.Inits {
			covered[init] = true
		}
	}

	for _, init := range []string{
		"file_read", "file_write", "file_edit", "file_find",
		"invoke_llm", "parse_response", "report_parse_error", "reset_history",
		"nudge_reread", "done", "suspend", "checkpoint_history",
		"checkpoint_rollback", "self_invoke",
		"extract_task", "select_all_ready", "seed_passthrough_plan", "mark_nodes_planning", "project_planner_context", "capture_planner_failure", "parse_plan", "compose",
		"format_issue", "record_tracker_issue", "mark_nodes_executing", "format_task_file", "mark_task_done", "mark_task_failed",
		"remaining_work",
		"parse_suite_config", "discover_suite_samples", "expand_eval_grid",
		"init_eval_session", "report_suite_summary", "materialize_eval_points", "run_point",
		"report_session", "run_agent", "record_oracle_result", "collect_trace_tokens",
		"check_agent_version", "summarize_point_results", "collect_metrics",
		"record_agent_commit", "dump_config", "list_evaluation_sessions",
		"analyze_evaluation_session", "list_evaluation_points", "read_evaluation_trace",
		"load_corpus", "validate_specs",
		"format_report", "rest_server_launch", "rest_server_stop",
	} {
		require.True(t, covered[init], "catalog should cover init %q", init)
	}
	require.False(t, covered["validate"], "retired validate aggregator must not have a builtin factory")
}

func TestRootCommandHasNoLifecycleSubcommands(t *testing.T) {
	t.Parallel()

	for _, cmd := range rootCmd.Commands() {
		require.NotContains(t, []string{"history", "rollback"}, cmd.Name())
	}
	assertMainDeclsAbsent(t, map[string]bool{
		"historyCmd":     true,
		"rollbackCmd":    true,
		"runHistory":     true,
		"runRollback":    true,
		"lifecycleStore": true,
	})
}

func TestRootCommandHasNoLifecycleOnlyFlags(t *testing.T) {
	t.Parallel()

	for _, flag := range []string{
		"checkpoint", "to-iteration", "machine", "tools",
		"tools-declaration", "tool-config-dir", "profiles-dir", "input",
		"validate-test-evidence", "run-test-evidence",
	} {
		require.Nil(t, rootCmd.PersistentFlags().Lookup(flag), "flag %q must not be public", flag)
	}
	for _, flag := range []string{"profile", "dolt-dsn", "resume-checkpoint", "resume-signal", "directory", "request"} {
		require.NotNil(t, rootCmd.PersistentFlags().Lookup(flag), "universal flag %q should remain", flag)
	}
	assertMainDeclsAbsent(t, map[string]bool{
		"flagHistoryCheckpoint":   true,
		"flagRollbackCheckpoint":  true,
		"flagRollbackToIteration": true,
		"flagMachine":             true,
		"flagTools":               true,
		"flagToolDeclarations":    true,
		"flagToolConfigDirs":      true,
		"flagProfilesDir":         true,
		"flagValidateEvidence":    true,
		"flagRunEvidence":         true,
		"validateTestEvidence":    true,
		"runTestEvidence":         true,
		"flagInput":               true,
	})
}

func TestRootCommandHelpShowsProfileOnlyRuntimeFlags(t *testing.T) {
	t.Parallel()

	usage := rootCmd.UsageString()

	for _, text := range []string{"--machine", "--tools", "--tools-declaration", "--tool-config-dir", "--profiles-dir", "--input", "--validate-test-evidence", "--run-test-evidence"} {
		require.NotContains(t, usage, text)
	}
	for _, text := range []string{"--profile", "--request", "--output", "--directory"} {
		require.Contains(t, usage, text)
	}
}

func TestMainWiresExitAgentToDeferredShutdown(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(filepath.Join(repoRootFromTest(t), "cmd", "agent", "main.go"))
	require.NoError(t, err)

	require.Regexp(t, `shutdown:\s+shutdown\.Request`, string(source))
	require.NotRegexp(t, `shutdown:\s+func\(\) \{\}`, string(source))
}

func TestProfileStartupLoadsCoreRuntimeFixtures(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	profileRoot := profileRootFromTest(t)
	profiles := []string{
		"control/profile.yaml",
		"lifecycle/profile.yaml",
		"monitor/profile.yaml",
		"audit/profile.yaml",
		"audit/audit-profile.yaml",
	}
	for _, rel := range profiles {
		t.Run(rel, func(t *testing.T) {
			clearAgentFlags()
			flagProfile = filepath.Join(profileRoot, filepath.FromSlash(rel))

			cfg, err := loadRuntimeConfig()
			require.NoError(t, err)
			defs, err := loadProfileToolDefs(cfg)
			require.NoError(t, err)
			spec, err := core.LoadMachineSpec(cfg.Machine)
			require.NoError(t, err)
			require.NoError(t, catalog.ValidateToolEmits(spec, defs))
		})
	}
}

func TestValidateConfigValidProfileExitsZero(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	clearAgentFlags()
	flagProfile = profilePathFromTest(t, "monitor/profile.yaml")
	flagValidateConfig = true

	stderr, err := captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.NoError(t, err)
	require.Contains(t, stderr, "config valid")
	// Validate mode must not enter the run loop or bind servers.
	require.NotContains(t, stderr, "\nterminal state:")
}

func TestValidateConfigInvalidRestExitsNonZero(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	monitorDir := filepath.Dir(profilePathFromTest(t, "monitor/profile.yaml"))
	dir := t.TempDir()
	badRest := filepath.Join(dir, "rest.yaml")
	require.NoError(t, os.WriteFile(badRest,
		[]byte("rest:\n  version: v1\n  auth:\n    broken:\n      type: totally-unsupported\n"), 0o644))
	profile := filepath.Join(dir, "profile.yaml")
	require.NoError(t, os.WriteFile(profile, []byte(fmt.Sprintf(
		"name: badrest\nmachine: %s\ntools:\n  - %s\ntool_declarations:\n  - %s\nrest_definitions:\n  - %s\n",
		filepath.Join(monitorDir, "machine.yaml"),
		filepath.Join(monitorDir, "tools.yaml"),
		filepath.Join(monitorDir, "declarations.yaml"),
		badRest)), 0o644))

	clearAgentFlags()
	flagProfile = profile
	flagValidateConfig = true

	_, err := captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported type")
}

func TestValidateConfigRejectsImplicitParseRetryPolicy(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	monitorDir := filepath.Dir(profilePathFromTest(t, "monitor/profile.yaml"))
	machineData, err := os.ReadFile(filepath.Join(monitorDir, "machine.yaml"))
	require.NoError(t, err)
	machineData = []byte(strings.Replace(
		string(machineData),
		"  max_iterations: 6",
		"  max_iterations: 6\n  max_consecutive_parse_errors: 2",
		1,
	))

	dir := t.TempDir()
	machine := filepath.Join(dir, "machine.yaml")
	require.NoError(t, os.WriteFile(machine, machineData, 0o644))
	profile := filepath.Join(dir, "profile.yaml")
	require.NoError(t, os.WriteFile(profile, []byte(fmt.Sprintf(
		"name: implicit-parse-retry\nmachine: %s\ntools:\n  - %s\ntool_declarations:\n  - %s\nrest_definitions:\n  - %s\n",
		machine,
		filepath.Join(monitorDir, "tools.yaml"),
		filepath.Join(monitorDir, "declarations.yaml"),
		filepath.Join(monitorDir, "rest.yaml"),
	)), 0o644))

	clearAgentFlags()
	flagProfile = profile
	flagValidateConfig = true

	_, err = captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse retry wiring validation")
	require.Contains(t, err.Error(), `select "report_parse_error"`)
	require.Contains(t, err.Error(), "remove the parse retry budget")
}

func TestValidateConfigInvalidReceiptContractExitsNonZero(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	monitorDir := filepath.Dir(profilePathFromTest(t, "monitor/profile.yaml"))
	// Corrupt one selected monitor tool to the inconsistent form GH-494
	// targets: reversible with a state-mutating effect but a noop undo.
	// --validate-config must reject it (srd025 R3.5; GH-494).
	realDecls, err := os.ReadFile(filepath.Join(monitorDir, "declarations.yaml"))
	require.NoError(t, err)
	corrupted := strings.Replace(string(realDecls),
		"      classification: reversible\n      undo: queue_event_restore",
		"      classification: reversible\n      undo: noop", 1)
	corrupted = strings.Replace(corrupted,
		"    strategy: queue_event_restore",
		"    strategy: noop", 1)
	require.NotEqual(t, string(realDecls), corrupted, "expected a reversible queue-restore tool to corrupt")

	dir := t.TempDir()
	badDecls := filepath.Join(dir, "declarations.yaml")
	require.NoError(t, os.WriteFile(badDecls, []byte(corrupted), 0o644))
	profile := filepath.Join(dir, "profile.yaml")
	require.NoError(t, os.WriteFile(profile, []byte(fmt.Sprintf(
		"name: badreceipt\nmachine: %s\ntools:\n  - %s\ntool_declarations:\n  - %s\nrest_definitions:\n  - %s\n",
		filepath.Join(monitorDir, "machine.yaml"),
		filepath.Join(monitorDir, "tools.yaml"),
		badDecls,
		filepath.Join(monitorDir, "rest.yaml"))), 0o644))

	clearAgentFlags()
	flagProfile = profile
	flagValidateConfig = true

	_, err = captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "receipt-contract validation failed")
	require.Contains(t, err.Error(), "no receipt-consuming undo")
}

func TestCommandFailureMessageReportsCommandErrorDetail(t *testing.T) {
	t.Parallel()

	message := commandFailureMessage(core.Result{
		CommandName: "load_corpus",
		Signal:      core.CommandError,
		Output:      "load corpus failed: parse SRD docs/specs/software-requirements/srd025-rollback-lifecycle.yaml: yaml: line 54",
	})

	require.Contains(t, message, "load_corpus failed")
	require.Contains(t, message, "srd025-rollback-lifecycle.yaml")
}
