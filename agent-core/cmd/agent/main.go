// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
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
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/profileaudit"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

var (
	flagProfile          string
	flagCoreRoot         string
	flagOTelLog          string
	flagOTelOTLP         string
	flagOTelMetricOTLP   string
	flagOTelService      string
	flagOTelParent       string
	flagDirectory        string
	flagTelemetryCapture string
	flagVerboseTrace     bool
	flagRequest          string
	flagOutput           string
	flagDoltDSN          string
	flagResumeCheckpoint string
	flagResumeSignal     string
	flagChildAgent       string
	flagValidateConfig   bool
	telemetryCaptureSet  = func() bool { return false }
)

const (
	agentVersion             = "v0.0.0-dev"
	terminalSummaryMaxBytes  = 1 << 20
	terminalSummaryTruncated = "... [terminal summary truncated]"
)

// Exit codes. A caller that runs an agent as a child process reads its
// outcome here: zero means the machine reached a success terminal, and
// ExitMachineFailed means it reached a failure terminal. Those are different
// facts from ExitRunError, which means the binary could not complete a run at
// all (bad config, unreadable profile, transport failure), so a caller can
// tell a clean domain failure from a crash (srd018 R6).
const (
	ExitSucceeded     = 0
	ExitRunError      = 1
	ExitMachineFailed = 2
)

// runExitCode carries the terminal-status mapping from run() to main(), since
// a failed terminal is not a cobra error: the binary did its job, and the
// machine it interpreted reported failure.
var runExitCode = ExitSucceeded

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(ExitRunError)
	}
	os.Exit(runExitCode)
}

// exitCodeForStatus maps a run's terminal status to the process exit code.
// Suspended is a deliberate pause with a persisted checkpoint, not a failure,
// so it exits zero; a caller resumes it rather than treating it as broken.
func exitCodeForStatus(status core.RunStatus) int {
	switch status {
	case core.StatusSucceeded, core.StatusSuspended:
		return ExitSucceeded
	default:
		return ExitMachineFailed
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
	f.StringVar(&flagOTelOTLP, "otel-otlp-endpoint", "", "OTLP gRPC endpoint for OTel spans (host:port); enables the OTLP exporter (srd008)")
	f.StringVar(&flagOTelMetricOTLP, "otel-metric-otlp-endpoint", "", "optional OTLP gRPC endpoint for OTel metrics; defaults to --otel-otlp-endpoint (srd008)")
	f.StringVar(&flagOTelService, "otel-service-name", "agent", "OTel resource service.name for this agent, so a cross-agent trace distinguishes agents")
	f.StringVar(&flagOTelParent, "otel-parent-span", "", "W3C traceparent for parent span")
	f.StringVar(&flagDirectory, "directory", "", "workspace directory")
	f.StringVar(&flagTelemetryCapture, "telemetry-capture", string(toollm.CaptureOff), "telemetry content capture level: off, delta, or full")
	f.BoolVar(&flagVerboseTrace, "verbose-trace", false, "record full LLM input/output in traces (alias for --telemetry-capture=full)")
	telemetryCaptureSet = func() bool { return f.Changed("telemetry-capture") }
	f.StringVar(&flagRequest, "request", "", "request data file")
	f.StringVar(&flagOutput, "output", "", "output directory for runtime artifacts")
	f.StringVar(&flagDoltDSN, "dolt-dsn", "", "MySQL-wire DSN to a dolt sql-server for the persistent checkpoint backend (default: no persistence)")
	f.StringVar(&flagResumeCheckpoint, "resume-checkpoint", "", "checkpoint ID to resume from")
	f.StringVar(&flagResumeSignal, "resume-signal", "", "resume signal override (default: required machine resume_signal)")
	f.StringVar(&flagChildAgent, "child-agent-binary", "", "path to the child agent binary used by child-process words (default: agent, resolved from PATH)")
	f.BoolVar(&flagValidateConfig, "validate-config", false, "load and validate the profile, machine, and REST definitions, then exit 0 (valid) or 1 (invalid) without serving; for a rollout preflight (srd015 R2.2)")

	rootCmd.Version = agentVersion
}

type agentState struct {
	parser        llm.ResponseParser
	conversation  *llm.Conversation
	registry      *core.Registry
	tracer        tracing.Tracer
	coreRoot      string
	model         string
	providerName  string
	manifestState core.State
	parseRetries  *toollm.ParseErrorRetryTracker
	validation    *validation.SpecState
	// validationEnabled distinguishes runtime-selected validation words from
	// the factory-catalog probe that invokes every registrar to discover names.
	validationEnabled bool
	// isolateConversations gives each invoke_llm word its own conversation instead
	// of the shared one, so a request-scoped router word's tool call does not
	// pollute the answer word's history. Set on request-local machine_request state.
	isolateConversations bool
	maxDuration          time.Duration
	maxTokens            int
	captureLevel         toollm.CaptureLevel
	ctx                  context.Context
	directory            string
	request              string
	output               string
	childAgentBinary     string
	runID                string
	doltDSN              string
	checkpoint           core.Checkpoint
	// lifecycleCheckpoint is the backend the checkpoint_history/checkpoint_rollback
	// tools read and revert through. For the history and rollback families it is
	// pinned to the request's target run so the inspecting machine never persists
	// over the run it inspects; otherwise it equals checkpoint.
	lifecycleCheckpoint core.Checkpoint
	monitor             toolrest.MonitorState
	restDefs            toolrest.Collection
	signalSourceRunner  toolrest.SignalSourceRunner
	shutdown            func()
	reapServices        func()
}

// checkpointForOps returns the backend the checkpoint history/rollback tools
// operate through: the target-pinned lifecycle backend when set, else the
// loop's own checkpoint backend.
func (st *agentState) checkpointForOps() core.Checkpoint {
	if st.lifecycleCheckpoint != nil {
		return st.lifecycleCheckpoint
	}
	return st.checkpoint
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
		spec.SetAgentCoreInstallRoot(strings.TrimSpace(flagCoreRoot))
	}
	if flagValidateConfig {
		return validateConfig()
	}
	prepared, err := prepareRun(cmd)
	if err != nil {
		return err
	}
	return runPrepared(prepared)
}

func runPrepared(prepared preparedRun) (err error) {
	defer func() {
		err = errors.Join(err, prepared.Close())
	}()
	if hasRequestSignalSources(prepared.State.restDefs) {
		return serveRequestSignalSources(prepared)
	}
	result, err := runOrResume(prepared.Config, resumeDeps{
		Params: prepared.Params,
		State:  prepared.State,
		Ctx:    prepared.Ctx,
	})
	if err != nil {
		return err
	}
	if result.Summary != "" {
		_, _ = fmt.Fprintln(os.Stdout, result.Summary)
	}
	fmt.Fprintf(os.Stderr, "terminal state: %s\n", result.Status)
	fmt.Fprintf(os.Stderr, "final machine state: %s\n", result.FinalState)
	runExitCode = exitCodeForStatus(result.Status)
	prepared.Shutdown.Apply()
	return nil
}

type boundSignalSourceRunner struct {
	mu     sync.RWMutex
	runner toolrest.SignalSourceRunner
}

func (r *boundSignalSourceRunner) Bind(runner toolrest.SignalSourceRunner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runner = runner
}

func (r *boundSignalSourceRunner) RequestSignal(
	ctx context.Context,
	envelope core.SignalEnvelope,
) core.SignalAdmission {
	r.mu.RLock()
	runner := r.runner
	r.mu.RUnlock()
	if runner != nil {
		return runner.RequestSignal(ctx, envelope)
	}
	return core.SignalAdmission{
		Outcome: core.AdmissionRefusedConflict,
		Source:  envelope.Source, RequestID: envelope.RequestID, RunID: envelope.RunID,
		Signal: envelope.Signal, Stage: "runner_unavailable",
		Err: fmt.Errorf("request signal source runner is unavailable"),
	}
}

type requestSignalCheckpointProvider func(string) (openedCheckpoint, error)

type requestSignalCheckpointStore struct {
	cfg     runtimeConfig
	machine core.MachineSpec
	mu      sync.Mutex
	memory  map[string]*core.InMemoryCheckpoint
}

func (s *requestSignalCheckpointStore) Open(runID string) (openedCheckpoint, error) {
	if s.cfg.DoltDSN != "" {
		cp, err := openDoltCheckpoint(s.cfg.DoltDSN, runID, terminalPredicate(s.machine))
		if err != nil {
			return openedCheckpoint{}, fmt.Errorf("open request signal Dolt checkpoint: %w", err)
		}
		return openedCheckpoint{
			Checkpoint: cp, close: cp.Close, label: "request signal checkpoint " + runID,
		}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.memory == nil {
		s.memory = make(map[string]*core.InMemoryCheckpoint)
	}
	checkpoint := s.memory[runID]
	if checkpoint == nil {
		checkpoint = core.NewInMemoryCheckpoint(runID)
		s.memory[runID] = checkpoint
	}
	return openedCheckpoint{Checkpoint: checkpoint}, nil
}

type hostRequestSignalRunner struct {
	source      *core.LoopSignalSource
	params      core.LoopParams
	state       *agentState
	checkpoints requestSignalCheckpointProvider
	afterRun    func()
	activeMu    sync.Mutex
	active      map[string]struct{}
	stateMu     sync.Mutex
}

func (r *hostRequestSignalRunner) RequestSignal(
	ctx context.Context,
	envelope core.SignalEnvelope,
) core.SignalAdmission {
	if !r.begin(envelope.RunID) {
		return refusedSignalAdmission(envelope, "concurrent_conflict", nil)
	}
	defer r.end(envelope.RunID)
	checkpoint, err := r.checkpoints(envelope.RunID)
	if err != nil {
		return refusedSignalAdmission(envelope, "checkpoint_open_failed", err)
	}
	params := r.params
	params.Checkpoint = checkpoint.Checkpoint
	params.MonitorRecorder = requestSignalMonitorRecorder(r.state, envelope.RunID)
	params.Hooks.RestoreSnapshot = r.restoreSnapshot
	// Builders and snapshot hooks retain the host's conversation and domain
	// state. Serialize that mutable host state across different run IDs while
	// begin still refuses, rather than queues, a concurrent request for one run.
	r.stateMu.Lock()
	admission := r.source.Admit(ctx, envelope, params)
	r.stateMu.Unlock()
	closeRequestSignalCheckpoint(checkpoint, &admission)
	if r.afterRun != nil {
		r.afterRun()
	}
	return admission
}

func requestSignalMonitorRecorder(st *agentState, runID string) monitor.RuntimeRecorder {
	recorder := st.monitor.Recorder
	scoped, ok := recorder.(monitor.TrustedEnvelopeRecorder)
	if !ok || st.monitor.Machine == nil {
		return recorder
	}
	return scoped.WithTrustedEnvelope(monitorEnvelopePolicy(
		*st.monitor.Machine, st.monitor.Tools, runID,
	))
}

func refusedSignalAdmission(
	envelope core.SignalEnvelope,
	stage string,
	err error,
) core.SignalAdmission {
	return core.SignalAdmission{
		Outcome: core.AdmissionRefusedConflict,
		Source:  envelope.Source, RequestID: envelope.RequestID, RunID: envelope.RunID,
		Signal: envelope.Signal, Stage: stage, Err: err,
	}
}

func closeRequestSignalCheckpoint(
	checkpoint openedCheckpoint,
	admission *core.SignalAdmission,
) {
	if checkpoint.close != nil {
		if closeErr := checkpoint.close(); closeErr != nil {
			admission.Err = errors.Join(admission.Err, fmt.Errorf("close request signal checkpoint: %w", closeErr))
			admission.Stage = "checkpoint_close_failed"
			if admission.Accepted() {
				admission.RunStatus = core.StatusFailed
			}
		}
	}
}

func (r *hostRequestSignalRunner) begin(runID string) bool {
	r.activeMu.Lock()
	defer r.activeMu.Unlock()
	if r.active == nil {
		r.active = make(map[string]struct{})
	}
	if _, exists := r.active[runID]; exists {
		return false
	}
	r.active[runID] = struct{}{}
	return true
}

func (r *hostRequestSignalRunner) end(runID string) {
	r.activeMu.Lock()
	defer r.activeMu.Unlock()
	delete(r.active, runID)
}

func (r *hostRequestSignalRunner) restoreSnapshot(snapshot core.AgentSnapshot) error {
	if len(snapshot.Conversation) > 0 {
		if err := r.state.restoreConversation(snapshot.Conversation); err != nil {
			return fmt.Errorf("restore conversation: %w", err)
		}
	}
	if err := r.state.restoreDomain(snapshot.Domain); err != nil {
		return fmt.Errorf("restore domain: %w", err)
	}
	return nil
}

type requestSignalServers struct {
	state     *toolrest.ServerState
	names     []string
	addresses map[string]string
}

func hasRequestSignalSources(defs toolrest.Collection) bool {
	return len(requestSignalServerNames(defs)) > 0
}

func requestSignalServerNames(defs toolrest.Collection) []string {
	names := make([]string, 0)
	for name, server := range defs.Servers {
		for _, endpoint := range server.Endpoints {
			if endpoint.Binding == "signal_source" {
				names = append(names, name)
				break
			}
		}
	}
	sort.Strings(names)
	return names
}

func launchRequestSignalServers(
	st *agentState,
	runner toolrest.SignalSourceRunner,
) (*requestSignalServers, error) {
	servers := &requestSignalServers{
		state: toolrest.NewServerState(), addresses: make(map[string]string),
	}
	for _, name := range requestSignalServerNames(st.restDefs) {
		def, err := st.restDefs.ResolveServer(name)
		if err != nil {
			return nil, errors.Join(err, servers.Close())
		}
		def.MachineRequestRunner = profileMachineRequestRunner(st)
		def.SignalSourceRunner = runner
		def.Monitor = st.monitor
		def.RunID = st.runID
		def.Credentials = toolrest.EnvironmentCredentials{}
		output, err := servers.state.Launch(def)
		if err != nil {
			return nil, errors.Join(err, servers.Close())
		}
		servers.names = append(servers.names, name)
		servers.addresses[name], _ = output["address"].(string)
	}
	return servers, nil
}

func (s *requestSignalServers) Close() error {
	if s == nil || s.state == nil {
		return nil
	}
	var errs []error
	for i := len(s.names) - 1; i >= 0; i-- {
		if _, err := s.state.Stop(s.names[i]); err != nil {
			errs = append(errs, err)
		}
	}
	s.names = nil
	return errors.Join(errs...)
}

func serveRequestSignalSources(prepared preparedRun) error {
	store := &requestSignalCheckpointStore{cfg: prepared.Config, machine: *prepared.Params.MachineSpec}
	runner := &hostRequestSignalRunner{
		source: core.NewLoopSignalSource(), params: prepared.Params, state: prepared.State,
		checkpoints: store.Open, afterRun: prepared.Shutdown.Apply,
	}
	if bound, ok := prepared.State.signalSourceRunner.(*boundSignalSourceRunner); ok {
		bound.Bind(runner)
	}
	servers, err := launchRequestSignalServers(prepared.State, runner)
	if err != nil {
		return fmt.Errorf("launch request signal servers: %w", err)
	}
	<-prepared.Ctx.Done()
	return servers.Close()
}

// validateConfig loads the profile, machine spec, tool definitions, and REST
// definitions and runs the same validation the runtime performs at startup
// (selected machine actions, ValidateDefinition per REST file during load,
// ValidateToolEmits over the machine, and ValidateReceiptContracts over the
// selected ToolDefs), then returns without binding servers or running the loop.
// A nil return exits 0; a load or validation error propagates to cobra and exits
// 1.
//
// The chatbot deployment runs this as an init-container so an invalid rendered
// rest.yaml fails the rollout before the agent serves (srd015 R2.2). The
// agent-profiles and chatbot-mesh audit gates run it over every shipped profile
// as a boot smoke, so a profile the runtime would reject fails the audit rather
// than surfacing the first time an agent starts (GH-614).
func validateConfig() error {
	resources, err := loadRunResources()
	if err != nil {
		return err
	}
	resources.shutdownTelemetry()
	if err := validateDeclaredRequestSources(resources.Config, resources.Definitions); err != nil {
		return err
	}
	reportMachineDiagnostics(resources.Machine)
	fmt.Fprintf(os.Stderr, "config valid: profile %s (%d REST client(s), %d server(s))\n",
		flagProfile, len(resources.RestDefinitions.Clients), len(resources.RestDefinitions.Servers))
	return nil
}

func reportMachineDiagnostics(machine core.MachineSpec) {
	for _, diagnostic := range core.DiagnoseMachineSpec(machine) {
		fmt.Fprintf(
			os.Stderr, "warning: machine-diagnostic-%s: %s\n",
			diagnostic.Code, diagnostic.Message,
		)
	}
}

type preparedRun struct {
	Config            runtimeConfig
	Params            core.LoopParams
	State             *agentState
	Ctx               context.Context
	Cancel            context.CancelFunc
	Shutdown          *deferredShutdown
	checkpoints       checkpointResources
	shutdownTelemetry func()
	closed            bool
	closeErr          error
}

type runResources struct {
	Config            runtimeConfig
	Tracer            tracing.Tracer
	Meter             metric.Meter
	Definitions       []catalog.ToolDef
	RestDefinitions   toolrest.Collection
	Machine           core.MachineSpec
	Program           core.ProgramRef
	shutdownTelemetry func()
}

type checkpointResources struct {
	opened []openedCheckpoint
}

func (r *checkpointResources) Add(checkpoint openedCheckpoint) {
	if checkpoint.close != nil {
		r.opened = append(r.opened, checkpoint)
	}
}

func (r *checkpointResources) Close() error {
	var errs []error
	for i := len(r.opened) - 1; i >= 0; i-- {
		checkpoint := r.opened[i]
		r.opened[i].close = nil
		if err := checkpoint.close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", checkpoint.label, err))
		}
	}
	r.opened = nil
	return errors.Join(errs...)
}

func (r *preparedRun) Close() error {
	if r.closed {
		return r.closeErr
	}
	r.closed = true
	if r.Cancel != nil {
		r.Cancel()
	}
	if r.State != nil && r.State.reapServices != nil {
		r.State.reapServices()
	}
	r.closeErr = r.checkpoints.Close()
	if r.shutdownTelemetry != nil {
		r.shutdownTelemetry()
	}
	return r.closeErr
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
	machineSpec, err := loadValidatedRuntimeMachine(cfg, defs)
	if err != nil {
		shutdownTelemetry()
		return runResources{}, err
	}
	program, err := buildProgramRef(cfg)
	if err != nil {
		shutdownTelemetry()
		return runResources{}, fmt.Errorf("build declarative program reference: %w", err)
	}
	return runResources{
		Config: cfg, Tracer: tracer, Meter: meter, Definitions: defs,
		RestDefinitions: restDefs, Machine: machineSpec, Program: program,
		shutdownTelemetry: shutdownTelemetry,
	}, nil
}

func loadValidatedRuntimeMachine(
	cfg runtimeConfig, defs []catalog.ToolDef,
) (core.MachineSpec, error) {
	machineSpec, err := core.LoadMachineSpec(cfg.Machine)
	if err != nil {
		return core.MachineSpec{}, fmt.Errorf("load machine spec for budget: %w", err)
	}
	if err := core.ValidateRequiredMachinePolicy(machineSpec); err != nil {
		return core.MachineSpec{}, fmt.Errorf("load machine runtime policy: %w", err)
	}
	if err := validateRuntimeToolWiring(machineSpec, defs); err != nil {
		return core.MachineSpec{}, err
	}
	if err := profileaudit.Validate(cfg.Profile); err != nil {
		return core.MachineSpec{}, fmt.Errorf("inspect profile timeout closure: %w", err)
	}
	return machineSpec, nil
}

// validateRuntimeToolWiring is the ordinary startup boundary. It rejects
// unselected named actions, machine/tool signal mismatches, incomplete
// parse-retry routes, and reversible effects without receipt-consuming undo.
// Full six-section contract completeness remains an authoring and
// specification-audit concern.
func validateRuntimeToolWiring(machine core.MachineSpec, defs []catalog.ToolDef) error {
	if err := catalog.ValidateMachineActions(machine, defs); err != nil {
		return err
	}
	if err := catalog.ValidateToolPhases(machine, defs); err != nil {
		return err
	}
	if err := catalog.ValidateParseRetryWiring(machine, defs); err != nil {
		return err
	}
	if err := catalog.ValidateToolEmits(machine, defs); err != nil {
		return err
	}
	return catalog.ValidateReceiptContracts(defs)
}

func buildPreparedRun(cmd *cobra.Command, resources runResources) (preparedRun, error) {
	cfg := resources.Config
	var checkpoints checkpointResources
	runID, err := resolveRunID(cfg)
	if err != nil {
		resources.shutdownTelemetry()
		return preparedRun{}, err
	}
	checkpoint := openedCheckpoint{Checkpoint: core.NoopCheckpoint{}}
	if hasRequestSignalSources(resources.RestDefinitions) {
		// Request serving replaces this template with the envelope RunID's
		// adapter. A non-noop template also tells lifecycle words that the host
		// has process-local continuation even when Dolt is not configured.
		checkpoint.Checkpoint = core.NewInMemoryCheckpoint(runID)
	} else {
		checkpoint, err = resolveCheckpoint(cfg, resources.Machine, runID)
		if err != nil {
			resources.shutdownTelemetry()
			return preparedRun{}, err
		}
	}
	checkpoints.Add(checkpoint)
	lifecycleCheckpoint, err := resolveLifecycleCheckpoint(cfg, resources.Definitions, checkpoint.Checkpoint)
	if err != nil {
		return preparedRun{}, closeBuildFailure(err, nil, &checkpoints, resources.shutdownTelemetry)
	}
	checkpoints.Add(lifecycleCheckpoint)
	resources, err = augmentRollbackResources(resources, lifecycleCheckpoint.Checkpoint)
	if err != nil {
		return preparedRun{}, closeBuildFailure(err, nil, &checkpoints, resources.shutdownTelemetry)
	}
	loopCtx, loopCancel := context.WithCancel(commandContext(cmd))
	shutdown := newDeferredShutdown(loopCancel)
	monitorRuntime, err := newMonitorRuntime(
		resources.Machine, resources.Definitions, resources.RestDefinitions, resources.Meter,
		runID,
	)
	if err != nil {
		return preparedRun{}, closeBuildFailure(err, loopCancel, &checkpoints, resources.shutdownTelemetry)
	}
	selectedInits := selectedBuiltinInits(resources.Definitions)
	reg := core.NewRegistry()
	builtins := toolregistry.NewBuiltinRegistry()
	retries := parseErrorRetryTracker(resources.Machine)
	signalSourceRunner := &boundSignalSourceRunner{}
	// One live source is shared: the loop refreshes it per dispatch and the
	// monitor command_state view reads it (srd033-monitor-rest-api R7.1).
	commandStateSource := core.NewLiveCommandStateSource()
	st := newAgentState(cfg, agentStateDeps{
		Registry:            reg,
		Tracer:              resources.Tracer,
		RunID:               runID,
		Checkpoint:          checkpoint.Checkpoint,
		LifecycleCheckpoint: lifecycleCheckpoint.Checkpoint,
		Ctx:                 loopCtx,
		Monitor: monitorState(
			monitorRuntime.Store, monitorRuntime.Recorder, &resources.Machine, resources.Definitions,
			commandStateSource,
		),
		RestDefs:           resources.RestDefinitions,
		SignalSourceRunner: signalSourceRunner,
		shutdown:           shutdown.Request,
		ParseRetries:       retries,
	})

	registerBuiltinFactories(builtins, st, selectedInits)
	if err := registerRuntimeTools(reg, builtins, cfg, resources.Machine, resources.Definitions); err != nil {
		err = fmt.Errorf("register tools: %w", err)
		return preparedRun{}, closeBuildFailure(err, loopCancel, &checkpoints, resources.shutdownTelemetry)
	}
	params := loopParams(cfg, loopParamDeps{
		Machine: resources.Machine, State: st, Registry: reg, Tracer: resources.Tracer,
		RunID: runID, Checkpoint: checkpoint.Checkpoint, MonitorRecorder: monitorRuntime.Recorder,
		CommandStateObserver: commandStateSource,
		Program:              resources.Program,
	})
	if err := seedRequest(&params, cfg.Request); err != nil {
		return preparedRun{}, closeBuildFailure(err, loopCancel, &checkpoints, resources.shutdownTelemetry)
	}
	return preparedRun{
		Config: cfg, Params: params, State: st, Ctx: loopCtx,
		Cancel: loopCancel, Shutdown: shutdown, checkpoints: checkpoints,
		shutdownTelemetry: resources.shutdownTelemetry,
	}, nil
}

// seedRequest makes the universal --request file the Seed result for ordinary
// machine runs. This is the same request path propagated by self_invoke, so a
// child profile can validate the supplied payload without a profile-specific
// loader. Resume replaces this position from the checkpoint before dispatch.
func seedRequest(params *core.LoopParams, path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read --request file: %w", err)
	}
	params.InitialSignal = core.Seed
	params.InitialResult = core.Result{Signal: core.Seed, Output: string(data)}
	return nil
}

func closeBuildFailure(primary error, cancel context.CancelFunc, checkpoints *checkpointResources, shutdownTelemetry func()) error {
	if cancel != nil {
		cancel()
	}
	closeErr := checkpoints.Close()
	if shutdownTelemetry != nil {
		shutdownTelemetry()
	}
	return errors.Join(primary, closeErr)
}

func initRunTelemetry(cfg runtimeConfig) (tracing.Tracer, metric.Meter, func(), error) {
	parentCtx, err := telemetry.ParseParentSpan(cfg.OTelParent)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse --otel-parent-span: %w", err)
	}
	if cfg.OTelLog == "" && cfg.OTelOTLP == "" && cfg.OTelMetricOTLP == "" {
		return tracing.NoopTracer{}, nil, func() {}, nil
	}
	exporter := telemetry.ExporterConfig{
		FilePath:           cfg.OTelLog,
		OTLPEndpoint:       cfg.OTelOTLP,
		MetricOTLPEndpoint: cfg.OTelMetricOTLP,
	}
	serviceName := cfg.OTelService
	if serviceName == "" {
		serviceName = "agent"
	}
	t, shutdown, err := telemetry.NewRoot(serviceName, "agent.run", exporter, parentCtx)
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

func parseErrorRetryTracker(machine core.MachineSpec) *toollm.ParseErrorRetryTracker {
	limit := parseErrorLimit(machine)
	if limit == 0 {
		return nil
	}
	return &toollm.ParseErrorRetryTracker{MaxConsecutive: limit}
}

func parseErrorLimit(machine core.MachineSpec) int {
	if machine.BudgetSpec == nil {
		return 0
	}
	return machine.BudgetSpec.MaxConsecutiveParseErrors
}

type agentStateDeps struct {
	Registry            *core.Registry
	Tracer              tracing.Tracer
	RunID               string
	Checkpoint          core.Checkpoint
	LifecycleCheckpoint core.Checkpoint
	Ctx                 context.Context
	Monitor             toolrest.MonitorState
	RestDefs            toolrest.Collection
	SignalSourceRunner  toolrest.SignalSourceRunner
	shutdown            func()
	ParseRetries        *toollm.ParseErrorRetryTracker
}

// checkpointOrNoop defaults an unset Checkpoint dependency to the no-op adapter.
// The composition root resolves the Dolt-backed backend (or NoopCheckpoint) via
// resolveCheckpoint; this guard covers direct constructions that omit it.
func checkpointOrNoop(cp core.Checkpoint) core.Checkpoint {
	if cp == nil {
		return core.NoopCheckpoint{}
	}
	return cp
}

func newAgentState(cfg runtimeConfig, deps agentStateDeps) *agentState {
	return &agentState{
		conversation:        llm.NewConversation(nil, "", llm.ChatOptions{}),
		registry:            deps.Registry,
		tracer:              deps.Tracer,
		coreRoot:            cfg.CoreRoot,
		parseRetries:        deps.ParseRetries,
		captureLevel:        cfg.TelemetryCapture,
		ctx:                 deps.Ctx,
		directory:           cfg.Directory,
		request:             cfg.Request,
		output:              cfg.Output,
		childAgentBinary:    cfg.ChildAgentBinary,
		runID:               deps.RunID,
		doltDSN:             cfg.DoltDSN,
		checkpoint:          checkpointOrNoop(deps.Checkpoint),
		lifecycleCheckpoint: deps.LifecycleCheckpoint,
		monitor:             deps.Monitor,
		restDefs:            deps.RestDefs,
		signalSourceRunner:  deps.SignalSourceRunner,
		shutdown:            deps.shutdown,
	}
}

func registerRuntimeTools(reg *core.Registry, builtins *toolregistry.BuiltinRegistry, cfg runtimeConfig, machine core.MachineSpec, defs []catalog.ToolDef) error {
	vars := map[string]string{
		"directory": cfg.Directory,
		"request":   cfg.Request,
	}
	return toolregistry.RegisterUnifiedToolsForMachine(reg, builtins, cfg.Directory, machine, defs, vars, execBuilder)
}

type loopParamDeps struct {
	Machine              core.MachineSpec
	State                *agentState
	Registry             *core.Registry
	Tracer               tracing.Tracer
	RunID                string
	Checkpoint           core.Checkpoint
	MonitorRecorder      monitor.RuntimeRecorder
	CommandStateObserver core.CommandStateObserver
	Program              core.ProgramRef
}

func loopParams(cfg runtimeConfig, deps loopParamDeps) core.LoopParams {
	toolAction := toolregistry.BuildDynamicToolAction(toolregistry.DynamicToolActionDeps{
		Registry: deps.Registry,
		Tracer:   deps.Tracer,
		Verbose:  cfg.TelemetryCapture.CapturesFullContent(),
	})
	return core.LoopParams{
		MachineFile:          cfg.Machine,
		MachineSpec:          &deps.Machine,
		Program:              deps.Program,
		RunID:                deps.RunID,
		AgentName:            machineAgentName(deps.Machine),
		AgentVersion:         agentVersion,
		ModelName:            deps.State.model,
		ProviderName:         deps.State.providerName,
		Trace:                deps.Tracer,
		Budget:               runBudget(deps.Machine, deps.State),
		CommandTimeout:       deps.Machine.BudgetSpec.CommandTimeoutDuration(),
		ToolAction:           toolAction,
		Registry:             deps.Registry,
		Directory:            cfg.Directory,
		Checkpoint:           deps.Checkpoint,
		MonitorRecorder:      deps.MonitorRecorder,
		CommandStateObserver: deps.CommandStateObserver,
		Hooks: core.LoopHooks{
			OnResult:             cliResultReporterForMachine(&deps.Machine),
			SnapshotConversation: deps.State.snapshotConversation,
			SnapshotDomain:       deps.State.snapshotDomain,
		},
	}
}

func machineAgentName(machine core.MachineSpec) string {
	if machine.Name != "" {
		return machine.Name
	}
	return "agent"
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
	return core.Budget{}
}

func cliResultReporter(rr core.RunResult, res core.Result) core.RunResult {
	return reportRunResult(nil, rr, res)
}

func cliResultReporterForMachine(machine *core.MachineSpec) func(core.RunResult, core.Result) core.RunResult {
	return func(rr core.RunResult, res core.Result) core.RunResult {
		return reportRunResult(machine, rr, res)
	}
}

func reportRunResult(machine *core.MachineSpec, rr core.RunResult, res core.Result) core.RunResult {
	reportOperatorOutput(res)
	declaredSummary := machineDeclaresSummary(machine)
	if declaredSummary {
		rr.Summary = boundedTerminalSummary(rr.Summary)
	}
	if message := commandFailureMessage(res); message != "" {
		fmt.Fprintln(os.Stderr, message)
	}
	return rr
}

func machineDeclaresSummary(machine *core.MachineSpec) bool {
	if machine == nil {
		return false
	}
	if machine.SummarySignal != "" {
		return true
	}
	for _, transition := range machine.Transitions {
		if transition.Summary {
			return true
		}
	}
	return false
}

func boundedTerminalSummary(output string) string {
	if len(output) <= terminalSummaryMaxBytes {
		return output
	}
	keep := terminalSummaryMaxBytes - len(terminalSummaryTruncated)
	return output[:keep] + terminalSummaryTruncated
}

func commandFailureMessage(res core.Result) string {
	if res.Signal != core.CommandError {
		return ""
	}
	name := strings.TrimSpace(res.CommandName)
	if name == "" {
		name = "command"
	}
	detail := strings.TrimSpace(res.Output)
	if detail == "" && res.Err != nil {
		detail = res.Err.Error()
	}
	if detail == "" {
		return fmt.Sprintf("%s failed with signal %s", name, res.Signal)
	}
	return fmt.Sprintf("%s failed: %s", name, detail)
}

func reportOperatorOutput(res core.Result) {
	if res.OperatorReport == nil {
		return
	}
	field := res.OperatorReport.Field
	if res.OperatorReport.Label != "" {
		field = res.OperatorReport.Label + " " + field
	}
	fmt.Fprintf(os.Stderr, "%s: %s\n", field, res.OperatorReport.Value)
}

func (st *agentState) snapshotConversation() (json.RawMessage, error) {
	return json.Marshal(st.conversation.Snapshot())
}

type agentDomainSnapshot struct {
	ConsecutiveParseErrors int             `json:"consecutive_parse_errors"`
	Validation             json.RawMessage `json:"validation,omitempty"`
}

func (st *agentState) snapshotDomain() (json.RawMessage, error) {
	var validationSnapshot json.RawMessage
	if st.validation != nil {
		var err error
		validationSnapshot, err = validation.EncodeSpecState(st.validation)
		if err != nil {
			return nil, fmt.Errorf("encode validation domain snapshot: %w", err)
		}
	}
	return json.Marshal(agentDomainSnapshot{
		ConsecutiveParseErrors: st.parseRetries.Snapshot(),
		Validation:             validationSnapshot,
	})
}

func (st *agentState) restoreDomain(data json.RawMessage) error {
	if len(data) == 0 {
		return nil
	}
	var snapshot agentDomainSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("decode domain snapshot: %w", err)
	}
	if st.parseRetries == nil && snapshot.ConsecutiveParseErrors > 0 {
		return fmt.Errorf("domain snapshot has parse retries but current machine has no parse retry budget")
	}
	var restoredValidation *validation.SpecState
	if len(snapshot.Validation) > 0 {
		if st.validation == nil {
			return fmt.Errorf("domain snapshot has validation state but current machine has no validation state")
		}
		restoredValidation = &validation.SpecState{Stderr: st.validation.Stderr}
		if err := validation.RestoreSpecState(restoredValidation, snapshot.Validation); err != nil {
			return fmt.Errorf("restore validation state: %w", err)
		}
	} else if st.validation != nil {
		return fmt.Errorf("domain snapshot is missing validation state")
	}
	st.parseRetries.Restore(snapshot.ConsecutiveParseErrors)
	if restoredValidation != nil {
		*st.validation = *restoredValidation
	}
	return nil
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
			return result, fmt.Errorf("loop: %w", err)
		}
		return result, nil
	}
	return resumeRun(cfg, deps)
}

// resumeRun re-enters the loop through the single typed Checkpoint port path:
// LoadResume restores the persisted Position/Execution, the domain restores its
// conversation from the typed snapshot, then Loop runs to completion
// (srd035-checkpoint-port R6.2).
func resumeRun(cfg runtimeConfig, deps resumeDeps) (core.RunResult, error) {
	params := deps.Params
	params.InitialSignal = core.Signal(cfg.ResumeSignal)
	state, err := core.LoadResume(params)
	if err != nil {
		return core.RunResult{}, fmt.Errorf("resume: %w", err)
	}
	if state.Finalized {
		return state.Params.InitialRun, nil
	}
	if conversation := state.Position.Snapshot.Conversation; len(conversation) > 0 {
		if err := deps.State.restoreConversation(conversation); err != nil {
			return core.RunResult{}, fmt.Errorf("resume: restore conversation: %w", err)
		}
	}
	if err := deps.State.restoreDomain(state.Position.Snapshot.Domain); err != nil {
		return core.RunResult{}, fmt.Errorf("resume: restore domain: %w", err)
	}
	result, err := core.Loop(state.Params, deps.Ctx)
	if err != nil {
		return result, fmt.Errorf("resume: %w", err)
	}
	return result, nil
}

func (st *agentState) restoreConversation(data json.RawMessage) error {
	var messages []llm.Message
	if err := json.Unmarshal(data, &messages); err != nil {
		return err
	}
	st.conversation.Restore(messages)
	return nil
}
