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
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/stl"
)

var (
	flagProfile             string
	flagMachine             string
	flagTools               []string
	flagToolDeclarations    []string
	flagToolConfigDirs      []string
	flagOTelLog             string
	flagOTelParent          string
	flagDirectory           string
	flagProfilesDir         string
	flagVerboseTrace        bool
	flagModel               string
	flagInput               string
	flagOutput              string
	flagStateStoreDir       string
	flagResumeCheckpoint    string
	flagResumeSignal        string
	flagHistoryCheckpoint   string
	flagRollbackCheckpoint  string
	flagRollbackToIteration int
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

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show checkpoint history",
	RunE:  runHistory,
}

var rollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Roll back a checkpoint to a target iteration",
	RunE:  runRollback,
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
	f.StringVar(&flagModel, "model", "", "override LLM model name")
	f.StringVar(&flagInput, "input", "", "input file (e.g. suite YAML for evaluator mode)")
	f.StringVar(&flagOutput, "output", "", "output directory for eval results (default: eval-results)")
	f.StringVar(&flagStateStoreDir, "state-store-dir", "", "directory for lifecycle checkpoints")
	f.StringVar(&flagResumeCheckpoint, "resume-checkpoint", "", "checkpoint ID to resume from")
	f.StringVar(&flagResumeSignal, "resume-signal", string(core.Approved), "signal to feed the state machine when resuming")

	historyCmd.Flags().StringVar(&flagHistoryCheckpoint, "checkpoint", "latest", "checkpoint ID or latest")
	rollbackCmd.Flags().StringVar(&flagRollbackCheckpoint, "checkpoint", "latest", "checkpoint ID or latest")
	rollbackCmd.Flags().IntVar(&flagRollbackToIteration, "to-iteration", -1, "target iteration to roll back to")
	rootCmd.AddCommand(historyCmd, rollbackCmd)

	rootCmd.Version = "v0.0.0-dev"
}

func runHistory(cmd *cobra.Command, args []string) error {
	store, err := lifecycleStore()
	if err != nil {
		return err
	}
	checkpointID, err := core.ResolveLatestCheckpointID(cmd.Context(), store, flagHistoryCheckpoint)
	if err != nil {
		return err
	}
	cp, err := core.LoadCheckpoint(cmd.Context(), store, checkpointID)
	if err != nil {
		return err
	}
	fmt.Fprint(cmd.OutOrStdout(), core.FormatCheckpointHistory(cp))
	return nil
}

func runRollback(cmd *cobra.Command, args []string) error {
	if flagRollbackToIteration < 0 {
		return fmt.Errorf("--to-iteration is required and must be >= 0")
	}
	store, err := lifecycleStore()
	if err != nil {
		return err
	}
	var ws core.Workspace
	if flagDirectory != "" {
		ws, err = core.NewGitWorkspace(flagDirectory)
		if err != nil {
			return fmt.Errorf("rollback workspace %q: %w", flagDirectory, err)
		}
	}
	result, err := core.RollbackFromCheckpoint(core.RollbackFromCheckpointOptions{
		Store:           store,
		Workspace:       ws,
		CheckpointID:    flagRollbackCheckpoint,
		TargetIteration: flagRollbackToIteration,
		Ctx:             cmd.Context(),
	})
	if err != nil {
		return err
	}
	if result.WorkspaceRef != "" && ws == nil {
		return fmt.Errorf("rollback target has workspace ref %q; --directory is required for managed workspace restore", result.WorkspaceRef)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "rolled back checkpoint %s to iteration %d\n", result.Original.ID, flagRollbackToIteration)
	fmt.Fprintf(cmd.OutOrStdout(), "new checkpoint: %s\n", result.Checkpoint.ID)
	if result.WorkspaceRef != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "workspace ref: %s\n", result.WorkspaceRef)
	}
	return nil
}

func lifecycleStore() (core.StateStore, error) {
	if flagStateStoreDir == "" {
		return nil, fmt.Errorf("--state-store-dir is required")
	}
	return core.NewFileStore(flagStateStoreDir), nil
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

	var defs []stl.ToolDef
	var err error
	if len(flagToolDeclarations) > 0 || len(flagToolConfigDirs) > 0 {
		var declarations []stl.ToolDef
		if len(flagToolConfigDirs) > 0 {
			declarations, err = stl.LoadToolDeclarationsFromDirs(flagToolConfigDirs)
			if err != nil {
				return fmt.Errorf("load tool config dirs: %w", err)
			}
		}
		if len(flagToolDeclarations) > 0 {
			explicit, err := stl.LoadToolDeclarations(flagToolDeclarations)
			if err != nil {
				return fmt.Errorf("load tool declarations: %w", err)
			}
			declarations = stl.MergeToolDefs(declarations, explicit)
		}
		var selection []string
		selection, err = stl.LoadToolSelections(flagTools)
		if err != nil {
			return fmt.Errorf("load tool selection: %w", err)
		}
		defs, err = stl.SelectTools(declarations, selection)
		if err != nil {
			return fmt.Errorf("select tools: %w", err)
		}
	} else {
		if len(flagTools) > 1 {
			return fmt.Errorf("multiple --tools files require --tools-declaration")
		}
		defs, err = stl.LoadToolDefs(flagTools[0])
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
		"model":     flagModel,
		"directory": flagDirectory,
	}

	machineSpec, err := core.LoadMachineSpec(flagMachine)
	if err != nil {
		return fmt.Errorf("load machine spec for budget: %w", err)
	}
	if err := stl.ValidateToolEmits(machineSpec, defs); err != nil {
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
	builtins := stl.NewBuiltinRegistry()
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

	if err := stl.RegisterUnifiedTools(reg, builtins, flagDirectory, defs, vars); err != nil {
		return fmt.Errorf("register tools: %w", err)
	}
	if st.maxDuration > 0 {
		budget.MaxDuration = st.maxDuration
	}
	if st.maxTokens > 0 {
		budget.MaxTokens = st.maxTokens
	}

	toolAction := stl.BuildDynamicToolAction(stl.DynamicToolActionDeps{
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

func selectedBuiltinInits(defs []stl.ToolDef) map[string]bool {
	return stl.SelectedBuiltinInits(defs)
}

func registerBuiltinFactories(br *stl.BuiltinRegistry, st *agentState, selected map[string]bool) {
	stl.RegisterStandardBuiltinFactories(br, selected, standardFactoryDeps(st))
}

type builtinFactoryCatalogEntry struct {
	Name  string
	Inits []string
}

func (e builtinFactoryCatalogEntry) selectedBy(selected map[string]bool) bool {
	return stl.StandardFactoryCatalogEntry{Name: e.Name, Inits: e.Inits}.SelectedBy(selected)
}

func builtinFactoryCatalog(st *agentState) []builtinFactoryCatalogEntry {
	entries := stl.StandardFactoryCatalog(standardFactoryDeps(st))
	out := make([]builtinFactoryCatalogEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, builtinFactoryCatalogEntry{Name: entry.Name, Inits: entry.Inits})
	}
	return out
}

func standardFactoryDeps(st *agentState) stl.StandardFactoryDeps {
	return stl.StandardFactoryDeps{
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
		RegisterPlanning:   registerPlanningFactories(st),
		RegisterEvaluation: registerEvaluationFactories(st),
		RegisterBench:      registerBenchFactories(),
	}
}

func registerPlanningFactories(st *agentState) func(*stl.BuiltinRegistry) {
	return func(br *stl.BuiltinRegistry) {
		pipeline.RegisterFactories(br, pipeline.FactoryDeps{
			Directory: st.directory,
			Tracer:    st.tracer,
			Ctx:       st.ctx,
		})
	}
}

func registerEvaluationFactories(st *agentState) func(*stl.BuiltinRegistry) {
	return func(br *stl.BuiltinRegistry) {
		evaluation.RegisterEvalFactories(br, evaluation.EvalFactoryDeps{
			Ctx:       st.ctx,
			Registry:  st.registry,
			Stderr:    os.Stderr,
			SuitePath: flagInput,
			OutputDir: flagOutput,
		})
	}
}

func registerBenchFactories() func(*stl.BuiltinRegistry) {
	return func(br *stl.BuiltinRegistry) {
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
		{"model", "deprecated: configure model in invoke_llm tool declaration via --profile"},
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
	p, err := stl.LoadProfile(path)
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
