// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/evaluation"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/evaluation/bench"
	benchui "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/evaluation/bench/ui"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/telemetry"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/planning/pipeline"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/support/execute"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/control"
	toolexec "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/exec"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/filesystem"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/lifecycle"
	toollm "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/llm"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/validation"
)

var (
	flagProfile          string
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
}

func run(cmd *cobra.Command, args []string) error {
	cfg, err := loadRuntimeConfig()
	if err != nil {
		return err
	}

	var tracer tracing.Tracer = tracing.NoopTracer{}
	if cfg.OTelLog != "" {
		parentCtx, _ := telemetry.ParseParentSpan(cfg.OTelParent)
		exporter := telemetry.ExporterConfig{FilePath: cfg.OTelLog}
		t, shutdown, err := telemetry.NewRoot("agent", "agent.run", exporter, parentCtx)
		if err != nil {
			return fmt.Errorf("otel init: %w", err)
		}
		defer shutdown()
		tracer = telemetry.TraceAdapter{T: t}
	}

	defs, err := loadProfileToolDefs(cfg)
	if err != nil {
		return err
	}

	conversation := llm.NewConversation(nil, "", llm.ChatOptions{})
	tracker := validation.NewToolTracker()
	var stateStore core.StateStore
	if cfg.StateStoreDir != "" {
		stateStore = core.NewFileStore(cfg.StateStoreDir)
	}

	vars := map[string]string{
		"directory": cfg.Directory,
		"request":   cfg.Request,
	}

	machineSpec, err := core.LoadMachineSpec(cfg.Machine)
	if err != nil {
		return fmt.Errorf("load machine spec for budget: %w", err)
	}
	if err := catalog.ValidateToolEmits(machineSpec, defs); err != nil {
		return err
	}
	budgetDefaults := core.Budget{
		MaxIterations: 100,
	}
	budget := machineSpec.BudgetSpec.ToBudget(budgetDefaults)

	selectedInits := selectedBuiltinInits(defs)
	parseErrorLimit := 0
	if machineSpec.BudgetSpec != nil {
		parseErrorLimit = machineSpec.BudgetSpec.MaxConsecutiveParseErrors
	}
	var parseRetries *toollm.ParseErrorRetryTracker
	var afterDispatch func(core.Command, core.Result) core.Signal
	if parseErrorLimit > 0 {
		if selectedInits["report_parse_error"] {
			parseRetries = &toollm.ParseErrorRetryTracker{MaxConsecutive: parseErrorLimit}
		} else {
			afterDispatch = toollm.ParseErrorPolicy(parseErrorLimit)
		}
	}

	reg := core.NewRegistry()
	builtins := toolregistry.NewBuiltinRegistry()
	st := &agentState{
		conversation: conversation,
		tracker:      tracker,
		registry:     reg,
		tracer:       tracer,
		parseRetries: parseRetries,
		verbose:      cfg.VerboseTrace,
		ctx:          cmd.Context(),
		directory:    cfg.Directory,
		request:      cfg.Request,
		output:       cfg.Output,
		stateStore:   stateStore,
	}

	registerBuiltinFactories(builtins, st, selectedInits)

	if err := toolregistry.RegisterUnifiedTools(reg, builtins, cfg.Directory, defs, vars, execBuilder); err != nil {
		return fmt.Errorf("register tools: %w", err)
	}
	if st.maxDuration > 0 {
		budget.MaxDuration = st.maxDuration
	}
	if st.maxTokens > 0 {
		budget.MaxTokens = st.maxTokens
	}

	toolAction := toolregistry.BuildDynamicToolAction(toolregistry.DynamicToolActionDeps{
		Registry: reg,
		Tracker:  tracker,
		Tracer:   tracer,
		Verbose:  cfg.VerboseTrace,
	})

	params := core.LoopParams{
		MachineFile:  cfg.Machine,
		MachineSpec:  &machineSpec,
		AgentName:    "agent",
		ModelName:    st.model,
		ProviderName: st.providerName,
		Trace:        tracer,
		Budget:       budget,
		ToolAction:   toolAction,
		Registry:     reg,
		Directory:    cfg.Directory,
		StateStore:   stateStore,
		Hooks: core.LoopHooks{
			AfterDispatch: afterDispatch,
			SnapshotConversation: func() (json.RawMessage, error) {
				return json.Marshal(conversation.Snapshot())
			},
		},
	}

	var result core.RunResult
	if cfg.ResumeCheckpoint != "" {
		if stateStore == nil {
			return fmt.Errorf("--resume-checkpoint requires --state-store-dir")
		}
		resumeResult, err := core.ResumeFromCheckpoint(core.ResumeOptions{
			Store:        stateStore,
			CheckpointID: cfg.ResumeCheckpoint,
			Params:       params,
			ResumeSignal: core.Signal(cfg.ResumeSignal),
			RestoreConversation: func(data json.RawMessage) error {
				var messages []llm.Message
				if err := json.Unmarshal(data, &messages); err != nil {
					return err
				}
				conversation.Restore(messages)
				return nil
			},
			Ctx: cmd.Context(),
		})
		if err != nil {
			return fmt.Errorf("resume: %w", err)
		}
		result = resumeResult.Run
	} else {
		result, err = core.Loop(params, context.Background())
	}
	if err != nil {
		return fmt.Errorf("loop: %w", err)
	}

	fmt.Fprintf(os.Stderr, "terminal state: %s\n", result.Status)
	return nil
}

type runtimeConfig struct {
	Machine          string
	Tools            []string
	ToolDeclarations []string
	ToolConfigDirs   []string
	Directory        string
	Request          string
	Output           string
	OTelLog          string
	OTelParent       string
	VerboseTrace     bool
	StateStoreDir    string
	ResumeCheckpoint string
	ResumeSignal     string
}

func loadRuntimeConfig() (runtimeConfig, error) {
	if flagProfile == "" {
		return runtimeConfig{}, fmt.Errorf("--profile is required")
	}
	p, err := catalog.LoadProfile(flagProfile)
	if err != nil {
		return runtimeConfig{}, fmt.Errorf("load profile: %w", err)
	}
	directory := flagDirectory
	if directory == "" {
		directory = p.Directory
	}
	return runtimeConfig{
		Machine:          p.Machine,
		Tools:            append([]string(nil), p.Tools...),
		ToolDeclarations: append([]string(nil), p.ToolDeclarations...),
		ToolConfigDirs:   append([]string(nil), p.ToolConfigDirs...),
		Directory:        directory,
		Request:          flagRequest,
		Output:           flagOutput,
		OTelLog:          flagOTelLog,
		OTelParent:       flagOTelParent,
		VerboseTrace:     flagVerboseTrace,
		StateStoreDir:    flagStateStoreDir,
		ResumeCheckpoint: flagResumeCheckpoint,
		ResumeSignal:     flagResumeSignal,
	}, nil
}

func loadProfileToolDefs(cfg runtimeConfig) ([]catalog.ToolDef, error) {
	declarations, err := catalog.LoadToolDeclarationsFromDirs(cfg.ToolConfigDirs)
	if err != nil {
		return nil, fmt.Errorf("load tool config dirs: %w", err)
	}
	explicit, err := catalog.LoadToolDeclarations(cfg.ToolDeclarations)
	if err != nil {
		return nil, fmt.Errorf("load tool declarations: %w", err)
	}
	selection, err := catalog.LoadToolSelections(cfg.Tools)
	if err != nil {
		return nil, fmt.Errorf("load tool selection: %w", err)
	}
	defs, err := catalog.SelectTools(catalog.MergeToolDefs(declarations, explicit), selection)
	if err != nil {
		return nil, fmt.Errorf("select tools: %w", err)
	}
	return defs, nil
}

func selectedBuiltinInits(defs []catalog.ToolDef) map[string]bool {
	return toolregistry.SelectedBuiltinInits(defs)
}

func execBuilder(def catalog.ToolDef, root string) core.Builder {
	return &toolexec.ExecBuilder{Def: def, Root: root}
}

func registerBuiltinFactories(br *toolregistry.BuiltinRegistry, st *agentState, selected map[string]bool) {
	toolregistry.RegisterStandardBuiltinFactories(br, selected, standardFactoryDeps(st))
}

type builtinFactoryCatalogEntry struct {
	Name  string
	Inits []string
}

func (e builtinFactoryCatalogEntry) selectedBy(selected map[string]bool) bool {
	return toolregistry.StandardFactoryCatalogEntry{Name: e.Name, Inits: e.Inits}.SelectedBy(selected)
}

func builtinFactoryCatalog(st *agentState) []builtinFactoryCatalogEntry {
	entries := toolregistry.StandardFactoryCatalog(standardFactoryDeps(st))
	out := make([]builtinFactoryCatalogEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, builtinFactoryCatalogEntry{Name: entry.Name, Inits: entry.Inits})
	}
	return out
}

func standardFactoryDeps(st *agentState) toolregistry.StandardFactoryDeps {
	return toolregistry.StandardFactoryDeps{
		RegisterFilesystem:     registerFilesystemFactories(),
		RegisterLLM:            registerLLMFactories(st),
		RegisterLifecycle:      registerLifecycleFactories(st),
		RegisterValidation:     registerValidationFactory(st),
		RegisterControl:        registerControlFactories(st),
		RegisterPlanning:       registerPlanningFactories(st),
		RegisterEvaluation:     registerEvaluationFactories(st),
		RegisterBench:          registerBenchFactories(),
		RegisterSpecValidation: registerSpecValidationFactories(st),
	}
}

func registerFilesystemFactories() toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		fileFactories := []struct {
			init    string
			builder func(string) core.Builder
		}{
			{"file_read", func(root string) core.Builder { return &filesystem.ReadBuilder{Root: root} }},
			{"file_write", func(root string) core.Builder { return &filesystem.WriteBuilder{Root: root} }},
			{"file_edit", func(root string) core.Builder { return &filesystem.EditBuilder{Root: root} }},
			{"file_find", func(root string) core.Builder { return &filesystem.FindBuilder{Root: root} }},
			{"file_list", func(root string) core.Builder { return &filesystem.ListFilesBuilder{Root: root} }},
		}
		for _, entry := range fileFactories {
			registerFileFactory(br, entry.init, entry.builder)
		}
	}
}

func registerFileFactory(br *toolregistry.BuiltinRegistry, init string, builder func(string) core.Builder) {
	br.Register(init, func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return builder(vars["directory"]), nil
	})
}

func registerLLMFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		br.Register("invoke_llm", invokeLLMFactory(st))
		br.Register("parse_response", parseResponseFactory(st))
		br.Register("report_parse_error", reportParseErrorFactory(st))
		br.Register("reset_history", resetHistoryFactory(st))
		br.Register("nudge_reread", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
			return &control.NudgeRereadBuilder{Tracer: st.tracer}, nil
		})
		br.Register("done", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
			return control.DoneBuilder{}, nil
		})
	}
}

func invokeLLMFactory(st *agentState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return toollm.NewInvokeLLMBuilder(def, toollm.InvokeLLMFactoryDeps{
			History:    st.conversation,
			Registry:   st.registry,
			Tracer:     st.tracer,
			Verbose:    st.verbose,
			Ctx:        st.ctx,
			OnResolved: onModelResolved(st),
		})
	}
}

func onModelResolved(st *agentState) func(toollm.InvokeLLMResolvedConfig) {
	return func(cfg toollm.InvokeLLMResolvedConfig) {
		st.parser = cfg.Parser
		st.model = cfg.Model
		st.providerName = cfg.ProviderName
		st.maxDuration = cfg.MaxTime
		st.maxTokens = cfg.MaxTokens
	}
}

func parseResponseFactory(st *agentState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &toollm.ParseResponseBuilder{
			Registry: st.registry,
			Parser:   st.parser,
			Tracer:   st.tracer,
			Verbose:  st.verbose,
			Retry:    st.parseRetries,
		}, nil
	}
}

func reportParseErrorFactory(st *agentState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &toollm.ReportParseErrorBuilder{Tracer: st.tracer, Retry: st.parseRetries}, nil
	}
}

func resetHistoryFactory(st *agentState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &toollm.ResetHistoryBuilder{History: st.conversation, Tracer: st.tracer}, nil
	}
}

func registerLifecycleFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		lifecycle.RegisterFactories(br, lifecycle.FactoryDeps{StateStore: st.stateStore, Tracer: st.tracer})
		br.Register("checkpoint_history", checkpointHistoryFactory(st))
		br.Register("checkpoint_rollback", checkpointRollbackFactory(st))
	}
}

func checkpointHistoryFactory(st *agentState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var cfg catalog.CheckpointHistoryConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		return &lifecycle.CheckpointHistoryBuilder{Config: cfg, StateStore: st.stateStore, Ctx: st.ctx}, nil
	}
}

func checkpointRollbackFactory(st *agentState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var cfg catalog.CheckpointRollbackConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		return &lifecycle.CheckpointRollbackBuilder{Config: cfg, StateStore: st.stateStore, Directory: st.directory, Tracer: st.tracer, Ctx: st.ctx}, nil
	}
}

func registerValidationFactory(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		br.Register("validate", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
			return &validation.ValidateBuilder{Tracker: st.tracker, Registry: st.registry, Tracer: st.tracer, Verbose: st.verbose}, nil
		})
	}
}

func registerControlFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		br.Register("self_invoke", selfInvokeFactory(st))
	}
}

func selfInvokeFactory(st *agentState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		parsed, err := decodeChildAgent(def)
		if err != nil {
			return nil, err
		}
		return &control.SelfInvokeBuilder{
			Config:    childExecuteConfig(parsed),
			ExtraArgs: directoryArgs(vars["directory"]),
			Ctx:       st.ctx,
			Tracer:    st.tracer,
		}, nil
	}
}

func decodeChildAgent(def catalog.ToolDef) (catalog.ChildAgentConfig, error) {
	var parsed catalog.ChildAgentConfig
	if err := catalog.DecodeToolConfig(def, &parsed); err != nil {
		return catalog.ChildAgentConfig{}, err
	}
	if err := catalog.ValidateChildAgentConfig(def.Name, parsed); err != nil {
		return catalog.ChildAgentConfig{}, err
	}
	return parsed, nil
}

func childExecuteConfig(parsed catalog.ChildAgentConfig) execute.Config {
	return execute.Config{
		Profile: parsed.Profile,
	}
}

func directoryArgs(directory string) []string {
	if directory == "" {
		return nil
	}
	return []string{"--directory", directory}
}

func registerSpecValidationFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		validation.RegisterSpecFactories(br, st.directory)
	}
}

func registerPlanningFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		pipeline.RegisterFactories(br, pipeline.FactoryDeps{
			Directory: st.directory,
			Tracer:    st.tracer,
			Ctx:       st.ctx,
		})
	}
}

func registerEvaluationFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		evaluation.RegisterEvalFactories(br, evaluation.EvalFactoryDeps{
			Ctx:       st.ctx,
			Registry:  st.registry,
			Stderr:    os.Stderr,
			SuitePath: st.request,
			OutputDir: st.output,
		})
	}
}

func registerBenchFactories() toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		bench.RegisterFactories(br, benchui.Assets())
	}
}
