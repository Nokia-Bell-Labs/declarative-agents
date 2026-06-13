// Copyright (c) 2026 Nokia. All rights reserved.

// Command agent is the unified agentic-loop binary. It loads a state machine
// and tools from YAML configuration, then runs core.Loop. Different modes
// (generator, planner, evaluator, bench, validate) are selected entirely
// by config files.
//
// Usage:
//
//	agent --machine <machine.yaml> --tools <tools.yaml> [flags]
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/evaluation"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/evaluation/bench"
	benchui "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/evaluation/bench/ui"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/telemetry"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/planning/pipeline"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/support/execute"
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
	Use:   "agent",
	Short: "Unified agentic-loop binary",
	Long: `agent loads a state machine and tools from YAML config and runs core.Loop.

Different modes (generate, pipeline, eval, bench) are selected entirely
by which --machine and --tools files you pass.`,
	SilenceUsage: true,
	RunE:         run,
}

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show checkpoint history",
	Long: `Show a checkpoint history digest.

Examples:
  agent history --state-store-dir .agent-state --checkpoint latest
  agent history --state-store-dir .agent-state --checkpoint suspend-1-123`,
	RunE: runHistory,
}

var rollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Roll back a checkpoint to a target iteration",
	Long: `Roll back a persisted checkpoint to a target iteration.

The command rewrites checkpoint state to the target history position and
restores the target workspace ref when one is present. A workspace restore
requires --directory so the workspace can be verified as a managed git root.

Examples:
  agent rollback --state-store-dir .agent-state --checkpoint latest --to-iteration 2 --directory "$PWD"`,
	RunE: runRollback,
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

// agentState holds the shared state needed by builtin tool factories.
// Created during run() initialization and captured by factory closures.
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

	// Set up OTel if configured
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

	// Load tool definitions: either declaration+selection or legacy single file
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

	// Create conversation and tracker
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

	// Load the machine before tool registration so machine-scoped policy
	// metadata can parameterize explicit grammar words such as report_parse_error.
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

	// Build registries
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

	// Build $tool action (dynamic tool dispatch from parse_response output)
	toolAction := buildToolAction(st, reg)

	// Run the loop
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

// buildToolAction creates the ActionFunc for $tool dynamic dispatch.
// It unmarshals the ToolRequest from parse_response output, resolves
// the builder from the registry, records the tool in the tracker,
// and dispatches.
func buildToolAction(st *agentState, reg *core.Registry) core.ActionFunc {
	return func(r core.Result) core.Command {
		var treq llm.ToolRequest
		if err := json.Unmarshal([]byte(r.Output), &treq); err != nil {
			return &failCmd{err: fmt.Errorf("failed to unmarshal ToolRequest: %w", err)}
		}
		builder, ok := reg.Resolve(treq.ToolName)
		if !ok {
			return &failCmd{err: fmt.Errorf("no builder for tool %q", treq.ToolName)}
		}
		st.tracker.Record(treq.ToolName)
		cmd := builder.Build(core.Result{Output: r.Output})
		if st.verbose {
			return &tracedToolCmd{
				inner:    cmd,
				tracer:   st.tracer,
				toolName: treq.ToolName,
				params:   string(treq.Params),
			}
		}
		return cmd
	}
}

func selectedBuiltinInits(defs []stl.ToolDef) map[string]bool {
	selected := make(map[string]bool)
	for _, def := range defs {
		if def.Type == "builtin" && def.Init != "" {
			selected[def.Init] = true
		}
	}
	return selected
}

// registerBuiltinFactories wires only the builtin factory families required by
// the selected tool declarations. Program shape is still defined by machine and
// tools YAML; this bootstrap only installs factories that selected init names
// can resolve.
func registerBuiltinFactories(br *stl.BuiltinRegistry, st *agentState, selected map[string]bool) {
	for _, entry := range builtinFactoryCatalog(st) {
		if entry.selectedBy(selected) {
			entry.register(br)
		}
	}
}

type builtinFactoryCatalogEntry struct {
	Name     string
	Inits    []string
	register func(*stl.BuiltinRegistry)
}

func (e builtinFactoryCatalogEntry) selectedBy(selected map[string]bool) bool {
	for _, init := range e.Inits {
		if selected[init] {
			return true
		}
	}
	return false
}

func builtinFactoryCatalog(st *agentState) []builtinFactoryCatalogEntry {
	return []builtinFactoryCatalogEntry{
		{Name: "file_read", Inits: []string{"file_read"}, register: func(br *stl.BuiltinRegistry) {
			br.Register("file_read", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
				return &stl.ReadBuilder{Root: vars["directory"]}, nil
			})
		}},
		{Name: "file_write", Inits: []string{"file_write"}, register: func(br *stl.BuiltinRegistry) {
			br.Register("file_write", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
				return &stl.WriteBuilder{Root: vars["directory"]}, nil
			})
		}},
		{Name: "file_edit", Inits: []string{"file_edit"}, register: func(br *stl.BuiltinRegistry) {
			br.Register("file_edit", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
				return &stl.EditBuilder{Root: vars["directory"]}, nil
			})
		}},
		{Name: "file_find", Inits: []string{"file_find"}, register: func(br *stl.BuiltinRegistry) {
			br.Register("file_find", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
				return &stl.FindBuilder{Root: vars["directory"]}, nil
			})
		}},
		{Name: "file_list", Inits: []string{"file_list"}, register: func(br *stl.BuiltinRegistry) {
			br.Register("file_list", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
				return &stl.ListFilesBuilder{Root: vars["directory"]}, nil
			})
		}},
		{Name: "invoke_llm", Inits: []string{"invoke_llm"}, register: func(br *stl.BuiltinRegistry) {
			br.Register("invoke_llm", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
				return stl.NewInvokeLLMBuilder(def, stl.InvokeLLMFactoryDeps{
					History:     st.conversation,
					Registry:    st.registry,
					Tracer:      st.tracer,
					ProfilesDir: flagProfilesDir,
					Verbose:     st.verbose,
					Ctx:         st.ctx,
					OnResolved: func(cfg stl.InvokeLLMResolvedConfig) {
						st.parser = cfg.Parser
						st.model = cfg.Model
						st.providerName = cfg.ProviderName
						st.maxDuration = cfg.MaxTime
						st.maxTokens = cfg.MaxTokens
					},
				})
			})
		}},
		{Name: "parse_response", Inits: []string{"parse_response"}, register: func(br *stl.BuiltinRegistry) {
			br.Register("parse_response", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
				return &stl.ParseResponseBuilder{
					Registry: st.registry,
					Parser:   st.parser,
					Tracer:   st.tracer,
					Verbose:  st.verbose,
					Retry:    st.parseRetries,
				}, nil
			})
		}},
		{Name: "report_parse_error", Inits: []string{"report_parse_error"}, register: func(br *stl.BuiltinRegistry) {
			br.Register("report_parse_error", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
				return &stl.ReportParseErrorBuilder{
					Tracer: st.tracer,
					Retry:  st.parseRetries,
				}, nil
			})
		}},
		{Name: "reset_history", Inits: []string{"reset_history"}, register: func(br *stl.BuiltinRegistry) {
			br.Register("reset_history", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
				return &stl.ResetHistoryBuilder{
					History: st.conversation,
					Tracer:  st.tracer,
				}, nil
			})
		}},
		{Name: "nudge_reread", Inits: []string{"nudge_reread"}, register: func(br *stl.BuiltinRegistry) {
			br.Register("nudge_reread", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
				return &stl.NudgeRereadBuilder{
					Tracer: st.tracer,
				}, nil
			})
		}},
		{Name: "done", Inits: []string{"done"}, register: func(br *stl.BuiltinRegistry) {
			br.Register("done", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
				return stl.DoneBuilder{}, nil
			})
		}},
		{Name: "lifecycle", Inits: []string{"suspend"}, register: func(br *stl.BuiltinRegistry) {
			stl.RegisterLifecycleFactories(br, stl.LifecycleFactoryDeps{
				StateStore: st.stateStore,
				Tracer:     st.tracer,
			})
		}},
		{Name: "validate", Inits: []string{"validate"}, register: func(br *stl.BuiltinRegistry) {
			br.Register("validate", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
				return &stl.ValidateBuilder{
					Tracker:  st.tracker,
					Registry: st.registry,
					Tracer:   st.tracer,
					Verbose:  st.verbose,
				}, nil
			})
		}},
		{Name: "self_invoke", Inits: []string{"self_invoke"}, register: func(br *stl.BuiltinRegistry) {
			br.Register("self_invoke", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
				var parsed stl.ChildAgentConfig
				if err := stl.DecodeToolConfig(def, &parsed); err != nil {
					return nil, err
				}
				if err := stl.ValidateChildAgentConfig(def.Name, parsed); err != nil {
					return nil, err
				}
				cfg := execute.Config{
					Profile:          parsed.Profile,
					Machine:          parsed.Machine,
					Tools:            parsed.Tools,
					ToolDeclarations: parsed.ToolDeclarations,
					Model:            vars["model"],
				}
				var extra []string
				if dir := vars["directory"]; dir != "" {
					extra = append(extra, "--directory", dir)
				}
				return &stl.SelfInvokeBuilder{
					Config:    cfg,
					ExtraArgs: extra,
					Ctx:       st.ctx,
					Tracer:    st.tracer,
				}, nil
			})
		}},
		{Name: "planning", Inits: []string{
			"extract_task", "extract_all", "assemble_prompt", "parse_plan",
			"create_issue", "execute_task", "check_result",
		}, register: func(br *stl.BuiltinRegistry) {
			pipeline.RegisterFactories(br, pipeline.FactoryDeps{
				Directory: st.directory,
				Tracer:    st.tracer,
				Ctx:       st.ctx,
			})
		}},
		{Name: "evaluation", Inits: []string{
			"parse_suite_config", "discover_suite_samples", "expand_eval_grid",
			"init_eval_session", "report_suite_summary",
			"next_point", "run_point", "report_session",
			"run_agent", "run_oracle_check", "collect_trace_tokens",
			"check_agent_version", "summarize_point_results", "collect_metrics",
			"dump_config",
		}, register: func(br *stl.BuiltinRegistry) {
			evaluation.RegisterEvalFactories(br, evaluation.EvalFactoryDeps{
				Ctx:       st.ctx,
				Registry:  st.registry,
				Stderr:    os.Stderr,
				SuitePath: flagInput,
				OutputDir: flagOutput,
			})
		}},
		{Name: "bench", Inits: []string{"serve_ui", "launch_eval"}, register: func(br *stl.BuiltinRegistry) {
			bench.RegisterFactories(br, benchui.Assets())
		}},
		{Name: "spec_validation", Inits: []string{"load_corpus", "validate_specs", "format_report"}, register: func(br *stl.BuiltinRegistry) {
			stl.RegisterValidateFactories(br, st.directory)
		}},
	}
}

// failCmd immediately returns CommandError with the given error.
type failCmd struct {
	err error
}

func (f *failCmd) Name() string      { return "fail" }
func (f *failCmd) Undo() core.Result { return core.NoopUndo(f.Name()) }

func (f *failCmd) Execute() core.Result {
	return core.Result{
		Signal:      core.CommandError,
		Err:         f.err,
		Output:      f.err.Error(),
		CommandName: "fail",
	}
}

// tracedToolCmd wraps a tool command to record its input parameters
// and output in the trace when verbose tracing is enabled.
type tracedToolCmd struct {
	inner    core.Command
	tracer   tracing.Tracer
	toolName string
	params   string
}

func (t *tracedToolCmd) Name() string      { return t.inner.Name() }
func (t *tracedToolCmd) Undo() core.Result { return t.inner.Undo() }

func (t *tracedToolCmd) Execute() core.Result {
	child, done := t.tracer.Push("dispatch/"+t.toolName,
		attribute.String("tool.name", t.toolName),
		attribute.String("tool.params", t.params),
	)
	defer done()

	res := t.inner.Execute()

	child.SetAttributes(
		attribute.String("tool.output", llm.Truncate(res.Output, 8192)),
		attribute.String("tool.signal", string(res.Signal)),
	)
	return res
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
