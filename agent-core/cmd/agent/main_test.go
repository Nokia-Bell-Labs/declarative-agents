// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/lifecycle"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
	toolrest "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/rest"
)

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
		"generator": {},
		"planner":   {},
		"evaluator": {},
		"bench":     {},
		"jurist":    {},
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

	require.True(t, byName["planning"].selectedBy(map[string]bool{"execute_task": true}))
	require.True(t, byName["evaluation"].selectedBy(map[string]bool{"run_point": true}))
	require.True(t, byName["bench"].selectedBy(map[string]bool{"launch_eval": true}))
	require.True(t, byName["spec_validation"].selectedBy(map[string]bool{"validate_specs": true}))
	require.True(t, byName["lifecycle"].selectedBy(map[string]bool{"checkpoint_history": true}))
	require.True(t, byName["lifecycle"].selectedBy(map[string]bool{"checkpoint_rollback": true}))
	require.True(t, byName["documentation"].selectedBy(map[string]bool{"serve_documentation": true}))
	require.False(t, byName["planning"].selectedBy(map[string]bool{"launch_eval": true}))
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
		"file_read", "file_write", "file_edit", "file_find", "file_list",
		"invoke_llm", "parse_response", "report_parse_error", "reset_history",
		"nudge_reread", "done", "suspend", "checkpoint_history",
		"checkpoint_rollback", "validate", "self_invoke",
		"extract_task", "extract_all", "assemble_prompt", "parse_plan",
		"create_issue", "execute_task", "check_result",
		"parse_suite_config", "discover_suite_samples", "expand_eval_grid",
		"init_eval_session", "report_suite_summary", "next_point", "run_point",
		"report_session", "run_agent", "run_oracle_check", "collect_trace_tokens",
		"check_agent_version", "summarize_point_results", "collect_metrics",
		"dump_config", "serve_ui", "launch_eval", "load_corpus", "validate_specs",
		"format_report", "serve_documentation",
	} {
		require.True(t, covered[init], "catalog should cover init %q", init)
	}
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
	} {
		require.Nil(t, rootCmd.PersistentFlags().Lookup(flag), "flag %q must not be public", flag)
	}
	for _, flag := range []string{"profile", "state-store-dir", "resume-checkpoint", "resume-signal", "directory", "request"} {
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
		"flagInput":               true,
	})
}

func TestRootCommandHelpShowsProfileOnlyRuntimeFlags(t *testing.T) {
	t.Parallel()

	usage := rootCmd.UsageString()

	for _, text := range []string{"--machine", "--tools", "--tools-declaration", "--tool-config-dir", "--profiles-dir", "--input"} {
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

	require.Contains(t, string(source), "shutdown:     shutdown.Request")
	require.NotContains(t, string(source), "shutdown:     func() {}")
}

func requireMainWiresMonitorRecorder(t *testing.T) {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(repoRootFromTest(t), "cmd", "agent", "main.go"))
	require.NoError(t, err)
	require.Contains(t, string(source), "MonitorRecorder: monitorRuntime.Recorder")
}

func TestProfileStartupLoadsActiveProfiles(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	root := repoRootFromTest(t)
	profiles := []string{
		"agents/generator/profile.yaml",
		"agents/evaluator/profile.yaml",
		"agents/bench/profile.yaml",
		"agents/jurist/profile.yaml",
		"agents/lifecycle/history/profile.yaml",
		"agents/lifecycle/rollback/profile.yaml",
		"agents/lifecycle/approval/profile.yaml",
		"agents/knowledge-manager/documentation-curator/profile.yaml",
	}
	for _, rel := range profiles {
		t.Run(rel, func(t *testing.T) {
			clearAgentFlags()
			flagProfile = filepath.Join(root, rel)

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

func TestMonitorRuntimeOptInProfileSetsLoopRecorder(t *testing.T) {
	t.Parallel()
	machine := monitorRuntimeMachine()

	optIn := newMonitorRuntime(machine, nil, toolrest.Collection{}, nil)
	require.NotNil(t, optIn.Store)
	require.NotNil(t, optIn.Recorder)

	params := core.LoopParams{MonitorRecorder: optIn.Recorder}
	require.NotNil(t, params.MonitorRecorder)

	disabled := newMonitorRuntime(core.MachineSpec{}, nil, toolrest.Collection{}, nil)
	require.Nil(t, disabled.Store)
	require.Nil(t, disabled.Recorder)
}

func TestMonitorRuntimeRecordsDispatchMetricsInStore(t *testing.T) {
	t.Parallel()
	runtime := newMonitorRuntime(monitorRuntimeMachine(), nil, toolrest.Collection{}, nil)

	result := runMonitorRuntimeLoop(t, runtime)

	require.Equal(t, core.StatusSucceeded, result.Status)
	snapshot := runtime.Store.Snapshot()
	requireMonitorSample(t, snapshot.RecentSamples, "dispatch_count")
	requireMonitorSample(t, snapshot.RecentSamples, "dispatch_success")
}

func TestMonitorRuntimeUsesTelemetryMeter(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	runtime := newMonitorRuntime(monitorRuntimeMachine(), nil, toolrest.Collection{}, provider.Meter("agent"))

	_ = runMonitorRuntimeLoop(t, runtime)

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))
	requireMetricData(t, data, "dispatch_count")
}

func TestMonitorReleaseProfileProof(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })
	requireMainWiresMonitorRecorder(t)

	proof := monitorReleaseProof(t)
	resultCh := make(chan loopResult, 1)
	go func() {
		result, err := core.Loop(proof.params, context.Background())
		resultCh <- loopResult{result: result, err: err}
	}()
	waitForProofMonitorRoute(t, "http://127.0.0.1:18083/monitor/state")
	postProofMonitorExit(t, "http://127.0.0.1:18083/monitor/control/exit")
	outcome := receiveLoopResult(t, resultCh)
	require.NoError(t, outcome.err)
	require.Equal(t, core.State("Done"), outcome.result.FinalState)
	require.Equal(t, core.StatusSucceeded, outcome.result.Status)

	snapshot := proof.monitor.Store.Snapshot()
	requireMonitorSample(t, snapshot.RecentSamples, "dispatch_count")
	requireMonitorSampleAttribute(t, snapshot.RecentSamples, "dispatch_duration", "profile", "monitor")
	requireMonitorSampleAttribute(t, snapshot.RecentSamples, "dispatch_duration", "route_group", "monitor")

	var data metricdata.ResourceMetrics
	require.NoError(t, proof.metricReader.Collect(context.Background(), &data))
	requireMetricData(t, data, "dispatch_count")

	state, baseURL := launchProofMonitorREST(t, proof)
	defer func() { _, _ = state.Stop("monitor") }()
	metrics := proofRequestBody(t, baseURL+"/monitor/metrics")
	require.Contains(t, metrics, "dispatch_count")
	require.Contains(t, metrics, "route_group")
	require.Contains(t, proofRequestBody(t, baseURL+"/monitor/openapi"), "/monitor/metrics")
	require.Contains(t, proofRequestBody(t, baseURL+"/monitor/events/stream"), "event: metric_sample")
}

func TestMonitorCLIProfileServesUntilControlExit(t *testing.T) {
	root := repoRootFromTest(t)
	cmd, stdout, stderr := startMonitorAgentProcess(t, root)
	resultCh := waitForProcess(t, cmd)

	waitForProofMonitorRoute(t, "http://127.0.0.1:18083/monitor/state")
	stateBody := proofRequestBody(t, "http://127.0.0.1:18083/monitor/state")
	require.Contains(t, stateBody, `"state"`)
	require.Contains(t, stateBody, `"run_id"`)
	require.NotContains(t, stateBody, `"State"`)
	require.NotContains(t, stateBody, `"RunID"`)
	require.Contains(t, proofRequestBody(t, "http://127.0.0.1:18083/monitor/metrics"), "dispatch_count")
	requireProcessStillRunning(t, resultCh)
	postProofMonitorExit(t, "http://127.0.0.1:18083/monitor/control/exit")
	requireProcessSucceeded(t, resultCh, stdout, stderr)
}

type loopResult struct {
	result core.RunResult
	err    error
}

func TestControlProfileExitReachesSucceededBeforeDeferredShutdown(t *testing.T) {
	t.Parallel()
	var cancelled bool
	shutdown := newDeferredShutdown(func() { cancelled = true })

	result := runExitMachine(t, exitMachineCase{
		machinePath: "agents/control/machine.yaml",
		launch:      "launch_agent_control",
		await:       "await_agent_control",
		terminal:    "Succeeded",
		shutdown:    shutdown,
	})

	require.Equal(t, core.StatusSucceeded, result.Status)
	require.Equal(t, core.State("Succeeded"), result.FinalState)
	requireExitEvent(t, result)
	require.False(t, cancelled, "shutdown must wait until after Loop returns")
	shutdown.Apply()
	require.True(t, cancelled)
}

func TestDocumentationCuratorExitReachesDoneBeforeDeferredShutdown(t *testing.T) {
	t.Parallel()
	var cancelled bool
	shutdown := newDeferredShutdown(func() { cancelled = true })

	result := runExitMachine(t, exitMachineCase{
		machinePath:  "agents/knowledge-manager/documentation-curator/machine.yaml",
		launch:       "serve_documentation",
		secondLaunch: "launch_curator_control",
		await:        "await_curator_control",
		terminal:     "Done",
		shutdown:     shutdown,
	})

	require.Equal(t, core.StatusSucceeded, result.Status)
	require.Equal(t, core.State("Done"), result.FinalState)
	requireExitEvent(t, result)
	require.False(t, cancelled, "shutdown must wait until after Loop returns")
	shutdown.Apply()
	require.True(t, cancelled)
}

func TestApprovalLifecycleProfileSuspendsAndResumesApproved(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	profilePath := filepath.Join(repoRootFromTest(t), "agents", "lifecycle", "approval", "profile.yaml")
	storeDir := t.TempDir()

	clearAgentFlags()
	flagProfile = profilePath
	flagStateStoreDir = storeDir
	firstStderr, err := captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.NoError(t, err)
	require.Contains(t, firstStderr, "terminal state: suspended")

	store := core.NewFileStore(storeDir)
	keys, err := store.List(context.Background(), "checkpoint/")
	require.NoError(t, err)
	require.Len(t, keys, 1)
	checkpointID := strings.TrimPrefix(keys[0], "checkpoint/")

	clearAgentFlags()
	flagProfile = profilePath
	flagStateStoreDir = storeDir
	flagResumeCheckpoint = checkpointID
	flagResumeSignal = string(core.Approved)
	secondStderr, err := captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.NoError(t, err)
	require.Contains(t, secondStderr, "terminal state: succeeded")
}

func TestApprovalLifecycleProfileUsesWorkspaceLocalStateStore(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	profilePath := filepath.Join(repoRootFromTest(t), "agents", "lifecycle", "approval", "profile.yaml")
	workspace := t.TempDir()

	clearAgentFlags()
	flagProfile = profilePath
	flagDirectory = workspace
	firstStderr, err := captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.NoError(t, err)
	require.Contains(t, firstStderr, "terminal state: suspended")

	store := core.NewFileStore(filepath.Join(workspace, defaultStateStoreDirName))
	keys, err := store.List(context.Background(), "checkpoint/")
	require.NoError(t, err)
	require.Len(t, keys, 1)
	checkpointID := strings.TrimPrefix(keys[0], "checkpoint/")

	clearAgentFlags()
	flagProfile = profilePath
	flagDirectory = workspace
	flagResumeCheckpoint = checkpointID
	flagResumeSignal = string(core.Approved)
	secondStderr, err := captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.NoError(t, err)
	require.Contains(t, secondStderr, "terminal state: succeeded")
}

func TestStateStoreDirOverridesWorkspaceLocalDefault(t *testing.T) {
	cfg := runtimeConfig{
		Directory:     filepath.Join("workspace"),
		StateStoreDir: filepath.Join("operator", "state"),
	}

	require.Equal(t, filepath.Join("operator", "state"), resolveStateStoreRoot(cfg))
}

func TestResumeCheckpointRequiresResolvableStateStore(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	clearAgentFlags()
	flagProfile = filepath.Join(repoRootFromTest(t), "agents", "lifecycle", "approval", "profile.yaml")
	flagResumeCheckpoint = "missing"

	_, err := captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.ErrorContains(t, err, "--resume-checkpoint requires --directory or --state-store-dir")
}

type exitMachineCase struct {
	machinePath  string
	launch       string
	secondLaunch string
	await        string
	terminal     string
	shutdown     *deferredShutdown
}

type staticSignalBuilder struct {
	name      string
	signal    core.Signal
	output    string
	afterExit core.Signal
}

type staticSignalCmd struct {
	name   string
	signal core.Signal
	output string
}

func assertMainDeclsAbsent(t *testing.T, forbidden map[string]bool) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(filepath.Dir(currentFile), "main.go"), nil, 0)
	require.NoError(t, err)
	for _, decl := range parsed.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			require.False(t, forbidden[d.Name.Name], "main.go must not declare %s", d.Name.Name)
		case *ast.GenDecl:
			assertGenDeclNamesAbsent(t, d, forbidden)
		}
	}
}

func runExitMachine(t *testing.T, tc exitMachineCase) core.RunResult {
	t.Helper()
	machine, err := core.LoadMachineSpec(filepath.Join(repoRootFromTest(t), tc.machinePath))
	require.NoError(t, err)
	reg := core.NewRegistry()
	launchStopSignal := core.Signal("")
	if tc.secondLaunch != "" {
		launchStopSignal = "ServerStopped"
	}
	registerStaticSignal(reg, tc.launch, "ServerLaunched", "{}", launchStopSignal)
	if tc.secondLaunch != "" {
		registerStaticSignal(reg, tc.secondLaunch, "ServerLaunched", "{}", "")
	}
	registerStaticSignal(reg, tc.await, "ExitRequested", exitEventOutput(), "")
	reg.Register(core.ToolSpec{Name: "exit_agent"}, lifecycle.ExitBuilder{
		Config:   lifecycle.ExitConfig{Status: "success"},
		Shutdown: tc.shutdown.Request,
	})
	result, err := core.Loop(core.LoopParams{
		MachineSpec: &machine, Registry: reg, Trace: tracing.NoopTracer{},
	}, context.Background())
	require.NoError(t, err)
	return result
}

func registerStaticSignal(reg *core.Registry, name string, signal core.Signal, output string, afterExit core.Signal) {
	reg.Register(core.ToolSpec{Name: name}, staticSignalBuilder{
		name: name, signal: signal, output: output, afterExit: afterExit,
	})
}

func monitorRuntimeMachine() core.MachineSpec {
	return core.MachineSpec{
		Name:         "monitor-runtime-test",
		MetricLabels: core.MetricLabels{"profile": "monitor"},
		InitialState: "Idle",
		States: core.StateSpecs{
			{Name: "Idle"}, {Name: "Working"}, {Name: "Done"},
		},
		TerminalStates: []string{"Done"},
		Transitions: []core.TransitionSpec{
			{State: "Idle", Signal: "Seed", Next: "Working", Action: "run"},
			{State: "Working", Signal: "ToolDone", Next: "Done"},
		},
	}
}

func runMonitorRuntimeLoop(t *testing.T, runtime monitorRuntime) core.RunResult {
	t.Helper()
	reg := core.NewRegistry()
	registerStaticSignal(reg, "run", core.ToolDone, "ok", "")
	machine := monitorRuntimeMachine()
	result, err := core.Loop(core.LoopParams{
		MachineSpec:     &machine,
		AgentName:       "agent",
		Registry:        reg,
		Trace:           tracing.NoopTracer{},
		MonitorRecorder: runtime.Recorder,
	}, context.Background())
	require.NoError(t, err)
	return result
}

type monitorProof struct {
	params       core.LoopParams
	monitor      monitorRuntime
	monitorState toolrest.MonitorState
	restDefs     toolrest.Collection
	launchDef    catalog.ToolDef
	metricReader *sdkmetric.ManualReader
}

func monitorReleaseProof(t *testing.T) monitorProof {
	t.Helper()
	clearAgentFlags()
	root := repoRootFromTest(t)
	flagProfile = filepath.Join(root, "agents", "monitor", "profile.yaml")
	flagDirectory = root

	cfg, err := loadRuntimeConfig()
	require.NoError(t, err)
	defs, err := loadProfileToolDefs(cfg)
	require.NoError(t, err)
	restDefs, err := toolrest.LoadDefinitions(cfg.RestDefinitions, cfg.RestConfigDirs)
	require.NoError(t, err)
	machine, err := core.LoadMachineSpec(cfg.Machine)
	require.NoError(t, err)
	require.NoError(t, catalog.ValidateToolEmits(machine, defs))

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	mon := newMonitorRuntime(machine, defs, restDefs, provider.Meter("agent"))
	require.NotNil(t, mon.Store)
	require.NotNil(t, mon.Recorder)

	reg := core.NewRegistry()
	builtins := toolregistry.NewBuiltinRegistry()
	st := monitorProofAgentState(cfg, reg, mon, &machine, defs, restDefs)
	registerBuiltinFactories(builtins, st, selectedBuiltinInits(defs))
	vars := map[string]string{"directory": cfg.Directory, "request": cfg.Request}
	require.NoError(t, toolregistry.RegisterUnifiedTools(reg, builtins, cfg.Directory, defs, vars, execBuilder))

	return monitorProof{
		params: core.LoopParams{
			MachineSpec:     &machine,
			AgentName:       "agent",
			Registry:        reg,
			Trace:           tracing.NoopTracer{},
			Budget:          machine.BudgetSpec.ToBudget(core.Budget{MaxIterations: 2}),
			MonitorRecorder: mon.Recorder,
		},
		monitor:      mon,
		monitorState: monitorState(mon.Store, &machine, defs),
		restDefs:     restDefs,
		launchDef:    requireToolDef(t, defs, "launch_monitor_rest"),
		metricReader: reader,
	}
}

func monitorProofAgentState(
	cfg runtimeConfig,
	reg *core.Registry,
	mon monitorRuntime,
	machine *core.MachineSpec,
	defs []catalog.ToolDef,
	restDefs toolrest.Collection,
) *agentState {
	return &agentState{
		registry:   reg,
		tracer:     tracing.NoopTracer{},
		ctx:        context.Background(),
		directory:  cfg.Directory,
		request:    cfg.Request,
		monitor:    monitorState(mon.Store, machine, defs),
		restDefs:   restDefs,
		shutdown:   func() {},
		stateStore: nil,
	}
}

func launchProofMonitorREST(t *testing.T, proof monitorProof) (*toolrest.ServerState, string) {
	t.Helper()
	state := toolrest.NewServerState()
	br := toolregistry.NewBuiltinRegistry()
	toolrest.RegisterFactories(br, toolrest.FactoryDeps{
		Definitions:        proof.restDefs,
		ServerState:        state,
		Monitor:            proof.monitorState,
		CredentialResolver: toolrest.EmptyCredentialResolver{},
	})
	factory, ok := br.Resolve(toolrest.InitServerLaunch)
	require.True(t, ok)
	builder, err := factory(proof.launchDef, nil)
	require.NoError(t, err)
	result := builder.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("ServerLaunched"), result.Signal, result.Output)
	return state, "http://" + proofLaunchAddress(t, result.Output)
}

func proofLaunchAddress(t *testing.T, output string) string {
	t.Helper()
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(output), &decoded))
	address, ok := decoded["address"].(string)
	require.True(t, ok, "launch output should include address")
	return address
}

func proofRequestBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	var body bytes.Buffer
	_, err = body.ReadFrom(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, body.String())
	return body.String()
}

func waitForProofMonitorRoute(t *testing.T, url string) {
	t.Helper()
	client := http.Client{Timeout: 100 * time.Millisecond}
	require.Eventually(t, func() bool {
		resp, err := client.Get(url)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 2*time.Second, 10*time.Millisecond)
}

func postProofMonitorExit(t *testing.T, url string) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(`{"reason":"test"}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
}

func receiveLoopResult(t *testing.T, resultCh <-chan loopResult) loopResult {
	t.Helper()
	select {
	case outcome := <-resultCh:
		return outcome
	case <-time.After(2 * time.Second):
		require.FailNow(t, "monitor loop did not exit after control request")
	}
	return loopResult{}
}

func startMonitorAgentProcess(t *testing.T, root string) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	cmd := exec.Command("go", "run", "./cmd/agent", "--profile", "agents/monitor/profile.yaml", "--directory", root)
	cmd.Dir = root
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	return cmd, &stdout, &stderr
}

func waitForProcess(t *testing.T, cmd *exec.Cmd) <-chan error {
	t.Helper()
	resultCh := make(chan error, 1)
	go func() { resultCh <- cmd.Wait() }()
	return resultCh
}

func requireProcessStillRunning(t *testing.T, resultCh <-chan error) {
	t.Helper()
	select {
	case err := <-resultCh:
		require.Failf(t, "monitor process exited early", "err=%v", err)
	default:
	}
}

func requireProcessSucceeded(t *testing.T, resultCh <-chan error, stdout, stderr *bytes.Buffer) {
	t.Helper()
	select {
	case err := <-resultCh:
		require.NoError(t, err, "stdout=%s stderr=%s", stdout.String(), stderr.String())
	case <-time.After(5 * time.Second):
		require.FailNow(t, "monitor process did not exit after control request")
	}
	require.Contains(t, stderr.String(), "terminal state: succeeded")
}

func requireToolDef(t *testing.T, defs []catalog.ToolDef, name string) catalog.ToolDef {
	t.Helper()
	for _, def := range defs {
		if def.Name == name {
			return def
		}
	}
	require.Failf(t, "missing tool definition", "tool %q not found", name)
	return catalog.ToolDef{}
}

func requireMonitorSample(t *testing.T, samples []monitor.MetricSample, name string) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name == name {
			return
		}
	}
	require.Failf(t, "missing monitor sample", "sample %q not found in %#v", name, samples)
}

func requireMonitorSampleAttribute(t *testing.T, samples []monitor.MetricSample, name, key, value string) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name == name && sample.Attributes[key] == value {
			return
		}
	}
	require.Failf(t, "missing monitor sample attribute", "sample %q missing %s=%s in %#v", name, key, value, samples)
}

func requireMetricData(t *testing.T, data metricdata.ResourceMetrics, name string) {
	t.Helper()
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				return
			}
		}
	}
	require.Failf(t, "missing OTel metric", "metric %q not found in %#v", name, data)
}

func (b staticSignalBuilder) Build(previous core.Result) core.Command {
	if b.afterExit != "" && previous.Signal == core.Signal("AgentExited") {
		return staticSignalCmd{name: b.name, signal: b.afterExit, output: b.output}
	}
	return staticSignalCmd{name: b.name, signal: b.signal, output: b.output}
}

func (c staticSignalCmd) Name() string { return c.name }

func (c staticSignalCmd) Execute() core.Result {
	return core.Result{CommandName: c.name, Signal: c.signal, Output: c.output}
}

func (c staticSignalCmd) Undo() core.Result {
	return core.NoopUndo(c.name)
}

func exitEventOutput() string {
	return `{"payload":{"reason":"operator requested shutdown","status":"success"}}`
}

func requireExitEvent(t *testing.T, result core.RunResult) {
	t.Helper()
	require.NotEqual(t, core.StatusCancelled, result.Status)
	for _, event := range result.Events {
		if event.CommandName == "exit_agent" {
			require.Equal(t, core.Signal("AgentExited"), event.Signal)
			return
		}
	}
	require.Fail(t, "exit_agent event not recorded")
}

func assertGenDeclNamesAbsent(t *testing.T, decl *ast.GenDecl, forbidden map[string]bool) {
	t.Helper()
	for _, spec := range decl.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range value.Names {
			require.False(t, forbidden[name.Name], "main.go must not declare %s", name.Name)
		}
	}
}

type agentFlagSnapshot struct {
	profile          string
	otelLog          string
	otelParent       string
	directory        string
	verboseTrace     bool
	request          string
	output           string
	stateStoreDir    string
	resumeCheckpoint string
	resumeSignal     string
}

func snapshotAgentFlags() agentFlagSnapshot {
	return agentFlagSnapshot{
		profile:          flagProfile,
		otelLog:          flagOTelLog,
		otelParent:       flagOTelParent,
		directory:        flagDirectory,
		verboseTrace:     flagVerboseTrace,
		request:          flagRequest,
		output:           flagOutput,
		stateStoreDir:    flagStateStoreDir,
		resumeCheckpoint: flagResumeCheckpoint,
		resumeSignal:     flagResumeSignal,
	}
}

func restoreAgentFlags(s agentFlagSnapshot) {
	flagProfile = s.profile
	flagOTelLog = s.otelLog
	flagOTelParent = s.otelParent
	flagDirectory = s.directory
	flagVerboseTrace = s.verboseTrace
	flagRequest = s.request
	flagOutput = s.output
	flagStateStoreDir = s.stateStoreDir
	flagResumeCheckpoint = s.resumeCheckpoint
	flagResumeSignal = s.resumeSignal
}

func clearAgentFlags() {
	restoreAgentFlags(agentFlagSnapshot{resumeSignal: string(core.Approved)})
}

func repoRootFromTest(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
}

func captureStderr(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	defer func() { os.Stderr = old }()

	runErr := fn()
	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, readErr := buf.ReadFrom(r)
	require.NoError(t, readErr)
	require.NoError(t, r.Close())
	return buf.String(), runErr
}

func TestFormatCheckpointHistory(t *testing.T) {
	cp := sampleCheckpoint("cp-1", time.Unix(100, 0).UTC())

	out := core.FormatCheckpointHistory(cp)

	require.Contains(t, out, "checkpoint: cp-1")
	require.Contains(t, out, "iteration: 2")
	require.Contains(t, out, "state: Working")
	require.Contains(t, out, "1  read  Start -> Reading  signal=ToolDone  workspace=ref-1")
	require.Contains(t, out, "2  write  Reading -> Working  signal=EditDone  workspace=ref-2")
}

func TestResolveCheckpointIDLatest(t *testing.T) {
	ctx := context.Background()
	store := core.NewFileStore(t.TempDir())
	saveAgentCheckpoint(t, store, sampleCheckpoint("older", time.Unix(100, 0).UTC()))
	saveAgentCheckpoint(t, store, sampleCheckpoint("newer", time.Unix(200, 0).UTC()))

	id, err := core.ResolveLatestCheckpointID(ctx, store, "latest")

	require.NoError(t, err)
	require.Equal(t, "newer", id)
}

func TestRollbackCheckpointToIteration(t *testing.T) {
	cp := sampleCheckpoint("cp-1", time.Unix(100, 0).UTC())

	result, err := core.RollbackCheckpoint(cp, 1)

	require.NoError(t, err)
	require.Equal(t, "ref-1", result.WorkspaceRef)
	require.Equal(t, 1, result.Checkpoint.Iteration)
	require.Equal(t, 1, result.Checkpoint.AgentState.Iteration)
	require.Equal(t, core.State("Reading"), result.Checkpoint.AgentState.State)
	require.Equal(t, "ref-1", result.Checkpoint.WorkspaceRef)
	require.Len(t, result.Checkpoint.History, 1)
	require.JSONEq(t, `{"conversation_len":1}`, string(result.Checkpoint.DomainState))
	require.True(t, strings.HasPrefix(result.Checkpoint.ID, "rollback-cp-1-to-1-"))
}

func TestRollbackCheckpointToIterationRestoresConversationMemento(t *testing.T) {
	cp := sampleCheckpoint("cp-1", time.Unix(100, 0).UTC())
	cp.History[1].CommandName = "invoke_llm"
	cp.History[1].Undo = &core.UndoMemento{
		Version:     core.UndoMementoVersion,
		Kind:        core.UndoMementoReversible,
		CommandName: "invoke_llm",
		Payload:     json.RawMessage(`{"conversation":[{"role":"user","content":"before"}]}`),
	}

	result, err := core.RollbackCheckpoint(cp, 1)

	require.NoError(t, err)
	require.JSONEq(t, `[{"role":"user","content":"before"}]`, string(result.Checkpoint.ConversationLog))
}

func TestRollbackCheckpointToIterationRestoresPipelineDomainMemento(t *testing.T) {
	cp := sampleCheckpoint("cp-1", time.Unix(100, 0).UTC())
	cp.History[1].CommandName = "parse_plan"
	cp.History[1].Undo = &core.UndoMemento{
		Version:     core.UndoMementoVersion,
		Kind:        core.UndoMementoReversible,
		CommandName: "parse_plan",
		Payload:     json.RawMessage(`{"domain_state":{"retry_count":3,"issue_id":"old"}}`),
	}

	result, err := core.RollbackCheckpoint(cp, 1)

	require.NoError(t, err)
	require.JSONEq(t, `{"retry_count":3,"issue_id":"old"}`, string(result.Checkpoint.DomainState))
}

func TestRollbackCheckpointToIterationReportsMissingUndoMemento(t *testing.T) {
	cp := sampleCheckpoint("cp-1", time.Unix(100, 0).UTC())
	cp.History[1].Undo = nil

	_, err := core.RollbackCheckpoint(cp, 1)

	require.Contains(t, err.Error(), "rollback command restore")
	require.Contains(t, err.Error(), core.ErrUndoMementoMissing.Error())
	require.Contains(t, err.Error(), "write")
}

func TestRollbackCheckpointToIterationReportsIrreversibleUndoMemento(t *testing.T) {
	cp := sampleCheckpoint("cp-1", time.Unix(100, 0).UTC())
	irreversible := core.IrreversibleUndoMemento("write", "already published externally")
	cp.History[1].Undo = &irreversible

	_, err := core.RollbackCheckpoint(cp, 1)

	require.Contains(t, err.Error(), core.ErrUndoMementoIncompatible.Error())
	require.Contains(t, err.Error(), "irreversible")
	require.Contains(t, err.Error(), "already published externally")
}

func sampleCheckpoint(id string, ts time.Time) core.Checkpoint {
	return core.Checkpoint{
		ID:        id,
		Iteration: 2,
		Timestamp: ts,
		AgentState: core.AgentSnapshot{
			State:     "Working",
			Signal:    core.EditDone,
			Iteration: 2,
		},
		WorkspaceRef: "ref-2",
		DomainState:  json.RawMessage(`{"conversation_len":2}`),
		History: []core.HistoryDigest{
			{
				Iteration:    1,
				CommandName:  "read",
				FromState:    "Start",
				ToState:      "Reading",
				Signal:       core.ToolDone,
				WorkspaceRef: "ref-1",
			},
			{
				Iteration:   2,
				CommandName: "write",
				FromState:   "Reading",
				ToState:     "Working",
				Signal:      core.EditDone,
				Undo: &core.UndoMemento{
					Version:     core.UndoMementoVersion,
					Kind:        core.UndoMementoReversible,
					CommandName: "write",
					Payload:     json.RawMessage(`{"domain_state":{"conversation_len":1}}`),
				},
				WorkspaceRef: "ref-2",
			},
		},
	}
}

func saveAgentCheckpoint(t *testing.T, store core.StateStore, cp core.Checkpoint) {
	t.Helper()
	data, err := json.Marshal(cp)
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), "checkpoint/"+cp.ID, data))
}
