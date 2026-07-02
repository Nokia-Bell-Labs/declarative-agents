// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/metric"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/validation"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

var (
	flagProfile          string
	flagCoreRoot         string
	flagOTelLog          string
	flagOTelParent       string
	flagDirectory        string
	flagVerboseTrace     bool
	flagRequest          string
	flagOutput           string
	flagStateStoreDir    string
	flagResumeCheckpoint string
	flagResumeSignal     string
)

const (
	monitorLaunchCommandName = "launch_monitor_rest"
	monitorServerName        = "monitor"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:          "agent",
	Short:        "Unified agentic-loop binary",
	SilenceUsage: true,
	RunE:         run,
}

func init() {
	f := rootCmd.PersistentFlags()
	f.StringVar(&flagProfile, "profile", "", "path to agent profile YAML")
	f.StringVar(&flagCoreRoot, "core-root", "", "maps /opt/agent-core paths in the profile to this directory (development checkout)")
	f.StringVar(&flagOTelLog, "otel-log-file", "", "path to OTel trace output file")
	f.StringVar(&flagOTelParent, "otel-parent-span", "", "W3C traceparent for parent span")
	f.StringVar(&flagDirectory, "directory", "", "workspace directory")
	f.BoolVar(&flagVerboseTrace, "verbose-trace", false, "record LLM input/output in traces")
	f.StringVar(&flagRequest, "request", "", "request data file")
	f.StringVar(&flagOutput, "output", "", "output directory for eval results (default: eval-results)")
	f.StringVar(&flagStateStoreDir, "state-store-dir", "", "directory for lifecycle checkpoints")
	f.StringVar(&flagResumeCheckpoint, "resume-checkpoint", "", "checkpoint ID to resume from")
	f.StringVar(&flagResumeSignal, "resume-signal", string(core.Approved), "signal to feed the state machine when resuming")

	rootCmd.Version = "v0.0.0-dev"
}

type agentState struct {
	parser       llm.ResponseParser
	conversation *llm.Conversation
	tracker      *validation.ToolTracker
	registry     *core.Registry
	tracer       tracing.Tracer
	model        string
	providerName string
	parseRetries *toollm.ParseErrorRetryTracker
	maxDuration  time.Duration
	maxTokens    int
	verbose      bool
	ctx          context.Context
	directory    string
	request      string
	output       string
	stateStore   core.StateStore
	monitor      toolrest.MonitorState
	restDefs     toolrest.Collection
	shutdown     func()
}

type deferredShutdown struct {
	requested atomic.Bool
	cancel    context.CancelFunc
}

func newDeferredShutdown(cancel context.CancelFunc) *deferredShutdown {
	return &deferredShutdown{cancel: cancel}
}

func (s *deferredShutdown) Request() {
	s.requested.Store(true)
}

func (s *deferredShutdown) Apply() {
	if s.requested.Load() && s.cancel != nil {
		s.cancel()
	}
}

func run(cmd *cobra.Command, args []string) error {
	if f := cmd.Flags().Lookup("core-root"); f != nil && f.Changed && strings.TrimSpace(flagCoreRoot) != "" {
		spec.SetAgentCoreInstallRoot(flagCoreRoot)
	}
	prepared, err := prepareRun(cmd)
	if err != nil {
		return err
	}
	defer prepared.Close()
	result, err := runOrResume(prepared.Config, resumeDeps{
		Params: prepared.Params,
		State:  prepared.State,
		Ctx:    prepared.Ctx,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "terminal state: %s\n", result.Status)
	prepared.Shutdown.Apply()
	return nil
}

type preparedRun struct {
	Config            runtimeConfig
	Params            core.LoopParams
	State             *agentState
	Ctx               context.Context
	Cancel            context.CancelFunc
	Shutdown          *deferredShutdown
	shutdownTelemetry func()
}

type runResources struct {
	Config            runtimeConfig
	Tracer            tracing.Tracer
	Meter             metric.Meter
	Definitions       []catalog.ToolDef
	RestDefinitions   toolrest.Collection
	Machine           core.MachineSpec
	shutdownTelemetry func()
}

func (r preparedRun) Close() {
	if r.Cancel != nil {
		r.Cancel()
	}
	if r.shutdownTelemetry != nil {
		r.shutdownTelemetry()
	}
}

func prepareRun(cmd *cobra.Command) (preparedRun, error) {
	resources, err := loadRunResources()
	if err != nil {
		return preparedRun{}, err
	}
	return buildPreparedRun(cmd, resources)
}

func loadRunResources() (runResources, error) {
	cfg, err := loadRuntimeConfig()
	if err != nil {
		return runResources{}, err
	}
	tracer, meter, shutdownTelemetry, err := initRunTelemetry(cfg)
	if err != nil {
		return runResources{}, err
	}
	defs, restDefs, err := loadRuntimeDefinitions(cfg)
	if err != nil {
		shutdownTelemetry()
		return runResources{}, err
	}
	machineSpec, err := core.LoadMachineSpec(cfg.Machine)
	if err != nil {
		shutdownTelemetry()
		return runResources{}, fmt.Errorf("load machine spec for budget: %w", err)
	}
	if err := catalog.ValidateToolEmits(machineSpec, defs); err != nil {
		shutdownTelemetry()
		return runResources{}, err
	}
	return runResources{
		Config: cfg, Tracer: tracer, Meter: meter, Definitions: defs,
		RestDefinitions: restDefs, Machine: machineSpec,
		shutdownTelemetry: shutdownTelemetry,
	}, nil
}

func buildPreparedRun(cmd *cobra.Command, resources runResources) (preparedRun, error) {
	cfg := resources.Config
	stateStore := resolveStateStore(resources.Config)
	loopCtx, loopCancel := context.WithCancel(commandContext(cmd))
	shutdown := newDeferredShutdown(loopCancel)
	monitorRuntime := newMonitorRuntime(resources.Machine, resources.Definitions, resources.RestDefinitions, resources.Meter)
	selectedInits := selectedBuiltinInits(resources.Definitions)
	reg := core.NewRegistry()
	builtins := toolregistry.NewBuiltinRegistry()
	policy := parseErrorPolicy(resources.Machine, selectedInits)
	st := newAgentState(cfg, agentStateDeps{
		Registry:     reg,
		Tracer:       resources.Tracer,
		StateStore:   stateStore,
		Ctx:          loopCtx,
		Monitor:      monitorState(monitorRuntime.Store, &resources.Machine, resources.Definitions),
		RestDefs:     resources.RestDefinitions,
		shutdown:     shutdown.Request,
		ParseRetries: policy.Retries,
	})

	registerBuiltinFactories(builtins, st, selectedInits)
	if err := registerRuntimeTools(reg, builtins, cfg, resources.Definitions); err != nil {
		loopCancel()
		resources.shutdownTelemetry()
		return preparedRun{}, fmt.Errorf("register tools: %w", err)
	}
	params := loopParams(cfg, loopParamDeps{
		Machine: resources.Machine, State: st, Registry: reg, Tracer: resources.Tracer,
		StateStore: stateStore, MonitorRecorder: monitorRuntime.Recorder,
		AfterDispatch: policy.AfterDispatch,
	})
	return preparedRun{
		Config: cfg, Params: params, State: st, Ctx: loopCtx,
		Cancel: loopCancel, Shutdown: shutdown, shutdownTelemetry: resources.shutdownTelemetry,
	}, nil
}

func initRunTelemetry(cfg runtimeConfig) (tracing.Tracer, metric.Meter, func(), error) {
	if cfg.OTelLog == "" {
		return tracing.NoopTracer{}, nil, func() {}, nil
	}
	parentCtx, _ := telemetry.ParseParentSpan(cfg.OTelParent)
	exporter := telemetry.ExporterConfig{FilePath: cfg.OTelLog}
	t, shutdown, err := telemetry.NewRoot("agent", "agent.run", exporter, parentCtx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("otel init: %w", err)
	}
	return telemetry.TraceAdapter{T: t}, t.Meter(), shutdown, nil
}

func loadRuntimeDefinitions(cfg runtimeConfig) ([]catalog.ToolDef, toolrest.Collection, error) {
	defs, err := loadProfileToolDefs(cfg)
	if err != nil {
		return nil, toolrest.Collection{}, err
	}
	restDefs, err := toolrest.LoadDefinitions(cfg.RestDefinitions, cfg.RestConfigDirs)
	if err != nil {
		return nil, toolrest.Collection{}, fmt.Errorf("load REST definitions: %w", err)
	}
	return defs, restDefs, nil
}

type parsePolicy struct {
	Retries       *toollm.ParseErrorRetryTracker
	AfterDispatch func(core.Command, core.Result) core.Signal
}

func parseErrorPolicy(machine core.MachineSpec, selected map[string]bool) parsePolicy {
	limit := parseErrorLimit(machine)
	if limit == 0 {
		return parsePolicy{}
	}
	if selected["report_parse_error"] {
		return parsePolicy{Retries: &toollm.ParseErrorRetryTracker{MaxConsecutive: limit}}
	}
	return parsePolicy{AfterDispatch: toollm.ParseErrorPolicy(limit)}
}

func parseErrorLimit(machine core.MachineSpec) int {
	if machine.BudgetSpec == nil {
		return 0
	}
	return machine.BudgetSpec.MaxConsecutiveParseErrors
}

type agentStateDeps struct {
	Registry     *core.Registry
	Tracer       tracing.Tracer
	StateStore   core.StateStore
	Ctx          context.Context
	Monitor      toolrest.MonitorState
	RestDefs     toolrest.Collection
	shutdown     func()
	ParseRetries *toollm.ParseErrorRetryTracker
}

func newAgentState(cfg runtimeConfig, deps agentStateDeps) *agentState {
	return &agentState{
		conversation: llm.NewConversation(nil, "", llm.ChatOptions{}),
		tracker:      validation.NewToolTracker(),
		registry:     deps.Registry,
		tracer:       deps.Tracer,
		parseRetries: deps.ParseRetries,
		verbose:      cfg.VerboseTrace,
		ctx:          deps.Ctx,
		directory:    cfg.Directory,
		request:      cfg.Request,
		output:       cfg.Output,
		stateStore:   deps.StateStore,
		monitor:      deps.Monitor,
		restDefs:     deps.RestDefs,
		shutdown:     deps.shutdown,
	}
}

func registerRuntimeTools(reg *core.Registry, builtins *toolregistry.BuiltinRegistry, cfg runtimeConfig, defs []catalog.ToolDef) error {
	vars := map[string]string{
		"directory": cfg.Directory,
		"request":   cfg.Request,
	}
	return toolregistry.RegisterUnifiedTools(reg, builtins, cfg.Directory, defs, vars, execBuilder)
}

type loopParamDeps struct {
	Machine         core.MachineSpec
	State           *agentState
	Registry        *core.Registry
	Tracer          tracing.Tracer
	StateStore      core.StateStore
	MonitorRecorder monitor.RuntimeRecorder
	AfterDispatch   func(core.Command, core.Result) core.Signal
}

func loopParams(cfg runtimeConfig, deps loopParamDeps) core.LoopParams {
	toolAction := toolregistry.BuildDynamicToolAction(toolregistry.DynamicToolActionDeps{
		Registry: deps.Registry,
		Tracker:  deps.State.tracker,
		Tracer:   deps.Tracer,
		Verbose:  cfg.VerboseTrace,
	})
	return core.LoopParams{
		MachineFile:     cfg.Machine,
		MachineSpec:     &deps.Machine,
		AgentName:       "agent",
		ModelName:       deps.State.model,
		ProviderName:    deps.State.providerName,
		Trace:           deps.Tracer,
		Budget:          runBudget(deps.Machine, deps.State),
		ToolAction:      toolAction,
		Registry:        deps.Registry,
		Directory:       cfg.Directory,
		StateStore:      deps.StateStore,
		MonitorRecorder: deps.MonitorRecorder,
		Hooks: core.LoopHooks{
			AfterDispatch:        deps.AfterDispatch,
			OnResult:             monitorLaunchReporter,
			SnapshotConversation: deps.State.snapshotConversation,
		},
	}
}

func runBudget(machine core.MachineSpec, st *agentState) core.Budget {
	budget := machine.BudgetSpec.ToBudget(defaultRunBudget())
	if st.maxDuration > 0 {
		budget.MaxDuration = st.maxDuration
	}
	if st.maxTokens > 0 {
		budget.MaxTokens = st.maxTokens
	}
	return budget
}

func defaultRunBudget() core.Budget {
	return core.Budget{MaxIterations: 100}
}

func monitorLaunchReporter(rr core.RunResult, res core.Result) core.RunResult {
	if res.CommandName != monitorLaunchCommandName || res.Signal != core.Signal("ServerLaunched") {
		return rr
	}
	if address := monitorLaunchAddress(res.Output); address != "" {
		fmt.Fprintf(os.Stderr, "monitor address: %s\n", address)
	}
	return rr
}

func monitorLaunchAddress(output string) string {
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		return ""
	}
	server, _ := decoded["server"].(string)
	address, _ := decoded["address"].(string)
	if server != monitorServerName {
		return ""
	}
	return address
}

func (st *agentState) snapshotConversation() (json.RawMessage, error) {
	return json.Marshal(st.conversation.Snapshot())
}

type resumeDeps struct {
	Params core.LoopParams
	State  *agentState
	Ctx    context.Context
}

func runOrResume(cfg runtimeConfig, deps resumeDeps) (core.RunResult, error) {
	if cfg.ResumeCheckpoint == "" {
		result, err := core.Loop(deps.Params, deps.Ctx)
		if err != nil {
			return core.RunResult{}, fmt.Errorf("loop: %w", err)
		}
		return result, nil
	}
	return resumeRun(cfg, deps)
}

func resumeRun(cfg runtimeConfig, deps resumeDeps) (core.RunResult, error) {
	if deps.State.stateStore == nil {
		return core.RunResult{}, fmt.Errorf("--resume-checkpoint requires --directory or --state-store-dir")
	}
	result, err := core.ResumeFromCheckpoint(core.ResumeOptions{
		Store:               deps.State.stateStore,
		CheckpointID:        cfg.ResumeCheckpoint,
		Params:              deps.Params,
		ResumeSignal:        core.Signal(cfg.ResumeSignal),
		RestoreConversation: deps.State.restoreConversation,
		Ctx:                 deps.Ctx,
	})
	if err != nil {
		return core.RunResult{}, fmt.Errorf("resume: %w", err)
	}
	return result.Run, nil
}

func (st *agentState) restoreConversation(data json.RawMessage) error {
	var messages []llm.Message
	if err := json.Unmarshal(data, &messages); err != nil {
		return err
	}
	st.conversation.Restore(messages)
	return nil
}
