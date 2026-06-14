// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/stl"
)

var (
	flagProfile          string
	flagMachine          string
	flagTools            []string
	flagToolDeclarations []string
	flagToolConfigDirs   []string
	flagOTelLog          string
	flagOTelParent       string
	flagDirectory        string
	flagProfilesDir      string
	flagVerboseTrace     bool
	flagInput            string
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
	f.StringVar(&flagProfile, "profile", "", "path to agent profile YAML (replaces --machine/--tools/--tools-declaration)")
	f.StringVar(&flagMachine, "machine", "", "path to state machine YAML (required unless --profile)")
	f.StringArrayVar(&flagTools, "tools", nil, "path to tool selection YAML (repeatable, required unless --profile)")
	f.StringArrayVar(&flagToolDeclarations, "tools-declaration", nil, "path to tool declaration YAML (repeatable)")
	f.StringArrayVar(&flagToolConfigDirs, "tool-config-dir", nil, "directory of tool declaration YAMLs (repeatable)")
	f.StringVar(&flagOTelLog, "otel-log-file", "", "path to OTel trace output file")
	f.StringVar(&flagOTelParent, "otel-parent-span", "", "W3C traceparent for parent span")
	f.StringVar(&flagDirectory, "directory", "", "workspace directory")
	f.StringVar(&flagProfilesDir, "profiles-dir", "", "directory with model profile YAML files (overrides embedded)")
	f.BoolVar(&flagVerboseTrace, "verbose-trace", false, "record LLM input/output in traces")
	f.StringVar(&flagInput, "input", "", "input file (e.g. suite YAML for evaluator mode)")
	f.StringVar(&flagOutput, "output", "", "output directory for eval results (default: eval-results)")
	f.StringVar(&flagStateStoreDir, "state-store-dir", "", "directory for lifecycle checkpoints")
	f.StringVar(&flagResumeCheckpoint, "resume-checkpoint", "", "checkpoint ID to resume from")
	f.StringVar(&flagResumeSignal, "resume-signal", string(core.Approved), "signal to feed the state machine when resuming")

	rootCmd.Version = "v0.0.0-dev"
}

type agentState struct {
	parser        llm.ResponseParser
	conversation  *llm.Conversation
	conversations *stl.ConversationStore
	tracker       *stl.ToolTracker
	registry      *core.Registry
	tracer        tracing.Tracer
	model         string
	providerName  string
	parseRetries  *stl.ParseErrorRetryTracker
	maxDuration   time.Duration
	maxTokens     int
	verbose       bool
	ctx           context.Context
	directory     string
	stateStore    core.StateStore
}

func run(cmd *cobra.Command, args []string) error {
	if flagProfile != "" {
		if err := applyProfile(flagProfile); err != nil {
			return err
		}
	}

	warnDeprecated(cmd)

	if flagMachine == "" {
		return fmt.Errorf("--machine is required (or use --profile)")
	}
	if len(flagTools) == 0 {
		return fmt.Errorf("--tools is required (or use --profile)")
	}

	var tracer tracing.Tracer = tracing.NoopTracer{}
	if flagOTelLog != "" {
		parentCtx, _ := telemetry.ParseParentSpan(flagOTelParent)
		cfg := telemetry.ExporterConfig{FilePath: flagOTelLog}
		t, shutdown, err := telemetry.NewRoot("agent", "agent.run", cfg, parentCtx)
		if err != nil {
			return fmt.Errorf("otel init: %w", err)
		}
		defer shutdown()
		tracer = telemetry.TraceAdapter{T: t}
	}

	var defs []catalog.ToolDef
	var err error
	if len(flagToolDeclarations) > 0 || len(flagToolConfigDirs) > 0 {
		var declarations []catalog.ToolDef
		if len(flagToolConfigDirs) > 0 {
			declarations, err = catalog.LoadToolDeclarationsFromDirs(flagToolConfigDirs)
			if err != nil {
				return fmt.Errorf("load tool config dirs: %w", err)
			}
		}
		if len(flagToolDeclarations) > 0 {
			explicit, err := catalog.LoadToolDeclarations(flagToolDeclarations)
			if err != nil {
				return fmt.Errorf("load tool declarations: %w", err)
			}
			declarations = catalog.MergeToolDefs(declarations, explicit)
		}
		var selection []string
		selection, err = catalog.LoadToolSelections(flagTools)
		if err != nil {
			return fmt.Errorf("load tool selection: %w", err)
		}
		defs, err = catalog.SelectTools(declarations, selection)
		if err != nil {
			return fmt.Errorf("select tools: %w", err)
		}
	} else {
		if len(flagTools) > 1 {
			return fmt.Errorf("multiple --tools files require --tools-declaration")
		}
		defs, err = catalog.LoadToolDefs(flagTools[0])
		if err != nil {
			return fmt.Errorf("load tools: %w", err)
		}
	}

	conversation := llm.NewConversation(nil, "", llm.ChatOptions{})
	conversations := stl.NewConversationStore()
	tracker := stl.NewToolTracker()
	var stateStore core.StateStore
	if flagStateStoreDir != "" {
		stateStore = core.NewFileStore(flagStateStoreDir)
	}

	vars := map[string]string{
		"directory": flagDirectory,
		"input":     flagInput,
	}

	machineSpec, err := core.LoadMachineSpec(flagMachine)
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
	var parseRetries *stl.ParseErrorRetryTracker
	var afterDispatch func(core.Command, core.Result) core.Signal
	if parseErrorLimit > 0 {
		if selectedInits["report_parse_error"] {
			parseRetries = &stl.ParseErrorRetryTracker{MaxConsecutive: parseErrorLimit}
		} else {
			afterDispatch = stl.ParseErrorPolicy(parseErrorLimit)
		}
	}

	reg := core.NewRegistry()
	builtins := toolregistry.NewBuiltinRegistry()
	st := &agentState{
		conversation:  conversation,
		conversations: conversations,
		tracker:       tracker,
		registry:      reg,
		tracer:        tracer,
		parseRetries:  parseRetries,
		verbose:       flagVerboseTrace,
		ctx:           cmd.Context(),
		directory:     flagDirectory,
		stateStore:    stateStore,
	}

	registerBuiltinFactories(builtins, st, selectedInits)

	if err := toolregistry.RegisterUnifiedTools(reg, builtins, flagDirectory, defs, vars, execBuilder); err != nil {
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
		Verbose:  flagVerboseTrace,
	})

	params := core.LoopParams{
		MachineFile:  flagMachine,
		MachineSpec:  &machineSpec,
		AgentName:    "agent",
		ModelName:    st.model,
		ProviderName: st.providerName,
		Trace:        tracer,
		Budget:       budget,
		ToolAction:   toolAction,
		Registry:     reg,
		Directory:    flagDirectory,
		StateStore:   stateStore,
		Hooks: core.LoopHooks{
			AfterDispatch: afterDispatch,
			SnapshotConversation: func() (json.RawMessage, error) {
				return json.Marshal(conversation.Snapshot())
			},
		},
	}

	var result core.RunResult
	if flagResumeCheckpoint != "" {
		if stateStore == nil {
			return fmt.Errorf("--resume-checkpoint requires --state-store-dir")
		}
		resumeResult, err := core.ResumeFromCheckpoint(core.ResumeOptions{
			Store:        stateStore,
			CheckpointID: flagResumeCheckpoint,
			Params:       params,
			ResumeSignal: core.Signal(flagResumeSignal),
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

func selectedBuiltinInits(defs []catalog.ToolDef) map[string]bool {
	return toolregistry.SelectedBuiltinInits(defs)
}

func execBuilder(def catalog.ToolDef, root string) core.Builder {
	return &stl.ExecBuilder{Def: def, Root: root}
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
		RegisterFilesystem:     registerSTLFactories(stl.RegisterFilesystemFactories, st),
		RegisterLLM:            registerSTLFactories(stl.RegisterLLMFactories, st),
		RegisterLifecycle:      registerSTLFactories(stl.RegisterLifecycleFactoryGroup, st),
		RegisterValidation:     registerSTLFactories(stl.RegisterValidationFactory, st),
		RegisterControl:        registerSTLFactories(stl.RegisterControlFactories, st),
		RegisterPlanning:       registerPlanningFactories(st),
		RegisterEvaluation:     registerEvaluationFactories(st),
		RegisterBench:          registerBenchFactories(),
		RegisterSpecValidation: registerSTLFactories(stl.RegisterSpecValidationFactories, st),
	}
}

func registerSTLFactories(register func(*stl.BuiltinRegistry, stl.STLFactoryDeps), st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		register(br, stlFactoryDeps(st))
	}
}

func stlFactoryDeps(st *agentState) stl.STLFactoryDeps {
	return stl.STLFactoryDeps{
		Conversation: st.conversation,
		Registry:     st.registry,
		Parser:       func() llm.ResponseParser { return st.parser },
		Tracer:       st.tracer,
		ProfilesDir:  flagProfilesDir,
		Verbose:      st.verbose,
		Ctx:          st.ctx,
		Directory:    st.directory,
		StateStore:   st.stateStore,
		Tracker:      st.tracker,
		ParseRetries: st.parseRetries,
		OnModelResolved: func(cfg stl.InvokeLLMResolvedConfig) {
			st.parser = cfg.Parser
			st.model = cfg.Model
			st.providerName = cfg.ProviderName
			st.maxDuration = cfg.MaxTime
			st.maxTokens = cfg.MaxTokens
		},
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
			SuitePath: flagInput,
			OutputDir: flagOutput,
		})
	}
}

func registerBenchFactories() toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		bench.RegisterFactories(br, benchui.Assets())
	}
}

var osStderr io.Writer = os.Stderr

func warnDeprecated(cmd *cobra.Command) {
	deprecated := []struct {
		flag    string
		message string
	}{
		{"machine", "deprecated: use --profile instead of --machine"},
		{"tools", "deprecated: use --profile instead of --tools"},
		{"tools-declaration", "deprecated: use --profile instead of --tools-declaration"},
	}
	for _, d := range deprecated {
		if cmd.Flags().Changed(d.flag) {
			fmt.Fprintf(osStderr, "warning: --%s is %s\n", d.flag, d.message)
		}
	}
}

// applyProfile loads an agent profile and fills in any CLI flags that
// were not explicitly set. Explicit CLI flags always take precedence.
func applyProfile(path string) error {
	p, err := catalog.LoadProfile(path)
	if err != nil {
		return fmt.Errorf("load profile: %w", err)
	}
	if flagMachine == "" {
		flagMachine = p.Machine
	}
	if len(flagTools) == 0 {
		flagTools = p.Tools
	}
	if len(flagToolDeclarations) == 0 {
		flagToolDeclarations = p.ToolDeclarations
	}
	if len(flagToolConfigDirs) == 0 {
		flagToolConfigDirs = p.ToolConfigDirs
	}
	if flagDirectory == "" && p.Directory != "" {
		flagDirectory = p.Directory
	}
	return nil
}
