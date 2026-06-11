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
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/bench"
	benchui "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/bench/ui"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/execute"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/llm/ollama"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/pipeline"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/prompt"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/stl"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/telemetry"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
)

var (
	flagMachine          string
	flagTools            string
	flagToolDeclarations []string
	flagOTelLog          string
	flagOTelParent       string
	flagDirectory        string
	flagProfilesDir      string
	flagVerboseTrace     bool
	flagModel            string
	flagOllamaURL        string
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
	Use:   "agent",
	Short: "Unified agentic-loop binary",
	Long: `agent loads a state machine and tools from YAML config and runs core.Loop.

Different modes (generate, pipeline, eval, bench) are selected entirely
by which --machine and --tools files you pass.`,
	SilenceUsage: true,
	RunE:         run,
}

func init() {
	f := rootCmd.Flags()
	f.StringVar(&flagMachine, "machine", "", "path to state machine YAML (required)")
	f.StringVar(&flagTools, "tools", "", "path to tool selection YAML (required)")
	f.StringArrayVar(&flagToolDeclarations, "tools-declaration", nil, "path to tool declaration YAML (repeatable)")
	f.StringVar(&flagOTelLog, "otel-log-file", "", "path to OTel trace output file")
	f.StringVar(&flagOTelParent, "otel-parent-span", "", "W3C traceparent for parent span")
	f.StringVar(&flagDirectory, "directory", "", "workspace directory")
	f.StringVar(&flagProfilesDir, "profiles-dir", "", "directory with model profile YAML files (overrides embedded)")
	f.BoolVar(&flagVerboseTrace, "verbose-trace", false, "record LLM input/output in traces")
	f.StringVar(&flagModel, "model", "", "override LLM model name")
	f.StringVar(&flagOllamaURL, "ollama-url", "", "override Ollama server URL")
	f.StringVar(&flagInput, "input", "", "input file (e.g. suite YAML for evaluator mode)")
	f.StringVar(&flagOutput, "output", "", "output directory for eval results (default: eval-results)")
	f.StringVar(&flagStateStoreDir, "state-store-dir", "", "directory for lifecycle checkpoints")
	f.StringVar(&flagResumeCheckpoint, "resume-checkpoint", "", "checkpoint ID to resume from")
	f.StringVar(&flagResumeSignal, "resume-signal", string(core.Approved), "signal to feed the state machine when resuming")

	rootCmd.Version = "v0.0.0-dev"
}

// agentState holds the shared state needed by builtin tool factories.
// Created during run() initialization and captured by factory closures.
type agentState struct {
	adapter       llm.Client
	profileReg    *llm.ProfileRegistry
	parser        llm.ResponseParser
	assembler     llm.PromptAssembler
	conversation  *llm.Conversation
	conversations *stl.ConversationStore
	tracker       *stl.ToolTracker
	registry      *core.Registry
	tracer        tracing.Tracer
	model         string
	providerName  string
	serverAddr    string
	manifestState core.State
	numCtx        int
	callTimeout   time.Duration
	verbose       bool
	ctx           context.Context
	directory     string
	stateStore    core.StateStore
}

func run(cmd *cobra.Command, args []string) error {
	if flagMachine == "" {
		return fmt.Errorf("--machine is required")
	}
	if flagTools == "" {
		return fmt.Errorf("--tools is required")
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
	if len(flagToolDeclarations) > 0 {
		var declarations []stl.ToolDef
		declarations, err = stl.LoadToolDeclarations(flagToolDeclarations)
		if err != nil {
			return fmt.Errorf("load tool declarations: %w", err)
		}
		var selection []string
		selection, err = stl.LoadToolSelection(flagTools)
		if err != nil {
			return fmt.Errorf("load tool selection: %w", err)
		}
		defs, err = stl.SelectTools(declarations, selection)
		if err != nil {
			return fmt.Errorf("select tools: %w", err)
		}
	} else {
		defs, err = stl.LoadToolDefs(flagTools)
		if err != nil {
			return fmt.Errorf("load tools: %w", err)
		}
	}

	llmCfg := extractLLMConfig(defs)
	if flagModel != "" {
		llmCfg.Model = flagModel
	}
	if flagOllamaURL != "" {
		llmCfg.OllamaURL = flagOllamaURL
	}
	if err := validateLLMConfig(llmCfg); err != nil {
		return err
	}

	var agentPrompt prompt.Prompt
	if sp := llmCfg.SystemPrompt; sp != "" {
		agentPrompt = prompt.Prompt{
			Role:         sp,
			OutputFormat: llmCfg.ToolPrompt,
		}
	}

	// Create Ollama adapter (only if model is configured)
	var adapter llm.Client
	var serverAddr string
	var profileReg *llm.ProfileRegistry
	var parser llm.ResponseParser
	if llmCfg.Model != "" {
		httpTimeout := 5 * time.Minute
		if llmCfg.MaxTime > httpTimeout {
			httpTimeout = llmCfg.MaxTime
		}
		adapter, err = createLLMAdapter(llmCfg, httpTimeout, tracer)
		if err != nil {
			return fmt.Errorf("llm adapter: %w", err)
		}
		if u, err := url.Parse(llmCfg.OllamaURL); err == nil {
			serverAddr = u.Host
		}
		tracer.Event("setup.adapter_ready",
			attribute.String("ollama.url", llmCfg.OllamaURL),
			attribute.String("llm.model", llmCfg.Model),
		)

		if flagProfilesDir != "" {
			profileReg, err = llm.LoadProfiles(flagProfilesDir)
		} else {
			profileReg, err = llm.DefaultProfileRegistry()
		}
		if err != nil {
			return fmt.Errorf("load profiles: %w", err)
		}
		parser = profileReg.ResolveProfile(llmCfg.Model)

		profileSpec := profileReg.ResolveProfileSpec(llmCfg.Model)
		tracer.Event("setup.model_profile",
			attribute.String("profile.name", profileSpec.ProfileName),
		)
		if profileSpec.MachineName != "" {
			resolved := filepath.Join(filepath.Dir(flagMachine), profileSpec.MachineName+".yaml")
			if _, err := os.Stat(resolved); err != nil {
				return fmt.Errorf("profile %q references machine %q but %s does not exist: %w",
					profileSpec.ProfileName, profileSpec.MachineName, resolved, err)
			}
			flagMachine = resolved
			tracer.Event("setup.machine_from_profile",
				attribute.String("machine.resolved_path", flagMachine),
			)
		}
	}

	// Create assembler
	var assembler llm.PromptAssembler
	if agentPrompt.Role != "" || agentPrompt.Task != "" {
		assembler = &llm.DefaultAssembler{
			Prompt: agentPrompt,
			Parser: parser,
		}
	}

	// Create conversation and tracker
	conversation := llm.NewConversation(adapter, "", llm.ChatOptions{
		Model:  llmCfg.Model,
		NumCtx: llmCfg.NumCtx,
	})
	conversations := stl.NewConversationStore()
	tracker := stl.NewToolTracker()
	var stateStore core.StateStore
	if flagStateStoreDir != "" {
		stateStore = core.NewFileStore(flagStateStoreDir)
	}

	vars := map[string]string{
		"model":      llmCfg.Model,
		"directory":  flagDirectory,
		"ollama_url": llmCfg.OllamaURL,
	}

	// Build registries
	reg := core.NewRegistry()
	builtins := stl.NewBuiltinRegistry()

	st := &agentState{
		adapter:       adapter,
		profileReg:    profileReg,
		parser:        parser,
		assembler:     assembler,
		conversation:  conversation,
		conversations: conversations,
		tracker:       tracker,
		registry:      reg,
		tracer:        tracer,
		model:         llmCfg.Model,
		providerName:  llmCfg.Provider,
		serverAddr:    serverAddr,
		manifestState: core.State(llmCfg.ManifestState),
		numCtx:        llmCfg.NumCtx,
		callTimeout:   llmCfg.LLMTimeout,
		verbose:       flagVerboseTrace,
		ctx:           cmd.Context(),
		directory:     flagDirectory,
		stateStore:    stateStore,
	}

	registerBuiltinFactories(builtins, st, selectedBuiltinInits(defs))

	if err := stl.RegisterUnifiedTools(reg, builtins, flagDirectory, defs, vars); err != nil {
		return fmt.Errorf("register tools: %w", err)
	}

	// Build $tool action (dynamic tool dispatch from parse_response output)
	toolAction := buildToolAction(st, reg)

	// Build budget: defaults from machine.yaml, overridden by LLM config
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
	if llmCfg.MaxTime > 0 {
		budget.MaxDuration = llmCfg.MaxTime
	}
	if llmCfg.MaxTokens > 0 {
		budget.MaxTokens = llmCfg.MaxTokens
	}

	var afterDispatch func(core.Command, core.Result) core.Signal
	if machineSpec.BudgetSpec != nil && machineSpec.BudgetSpec.MaxConsecutiveParseErrors > 0 {
		afterDispatch = stl.ParseErrorPolicy(machineSpec.BudgetSpec.MaxConsecutiveParseErrors)
	}

	// Run the loop
	params := core.LoopParams{
		MachineFile:  flagMachine,
		MachineSpec:  &machineSpec,
		AgentName:    "agent",
		ModelName:    llmCfg.Model,
		ProviderName: "ollama",
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

// llmConfig holds LLM-related settings resolved from the invoke_llm
// tool definition's config block.
type llmConfig struct {
	Model         string
	Provider      string
	OllamaURL     string
	ManifestState string
	SystemPrompt  string
	ToolPrompt    string
	NumCtx        int
	LLMTimeout    time.Duration
	MaxTime       time.Duration
	MaxTokens     int
}

// extractLLMConfig scans tool definitions for an invoke_llm tool and
// extracts LLM settings from its config block.
func extractLLMConfig(defs []stl.ToolDef) llmConfig {
	cfg := llmConfig{
		Provider:      "ollama",
		OllamaURL:     "http://localhost:11434",
		ManifestState: "Composing",
	}
	for _, td := range defs {
		if td.Init != "invoke_llm" {
			continue
		}
		var tc stl.LLMToolConfig
		if err := stl.DecodeToolConfig(td, &tc); err != nil {
			break
		}
		if tc.Model != "" {
			cfg.Model = tc.Model
		}
		if tc.Provider != "" {
			cfg.Provider = tc.Provider
		}
		if tc.OllamaURL != "" {
			cfg.OllamaURL = tc.OllamaURL
		}
		if tc.ProviderURL != "" {
			cfg.OllamaURL = tc.ProviderURL
		}
		if tc.ManifestState != "" {
			cfg.ManifestState = tc.ManifestState
		}
		if tc.SystemPrompt != "" {
			cfg.SystemPrompt = tc.SystemPrompt
		}
		if tc.ToolPrompt != "" {
			cfg.ToolPrompt = tc.ToolPrompt
		}
		if tc.NumCtx > 0 {
			cfg.NumCtx = tc.NumCtx
		}
		if tc.LLMTimeout > 0 {
			cfg.LLMTimeout = time.Duration(tc.LLMTimeout) * time.Second
		}
		if tc.MaxTime > 0 {
			cfg.MaxTime = time.Duration(tc.MaxTime) * time.Second
		}
		if tc.MaxTokens > 0 {
			cfg.MaxTokens = tc.MaxTokens
		}
		break
	}
	return cfg
}

func validateLLMConfig(cfg llmConfig) error {
	if cfg.Model == "" {
		return nil
	}
	if cfg.ManifestState == "" {
		return fmt.Errorf("invoke_llm config requires manifest_state when model is set")
	}
	switch cfg.Provider {
	case "ollama":
		if cfg.OllamaURL == "" {
			return fmt.Errorf("invoke_llm config provider %q requires provider_url or ollama_url", cfg.Provider)
		}
		return nil
	default:
		return fmt.Errorf("unsupported invoke_llm provider %q", cfg.Provider)
	}
}

func createLLMAdapter(cfg llmConfig, httpTimeout time.Duration, tracer tracing.Tracer) (llm.Client, error) {
	switch cfg.Provider {
	case "ollama":
		return ollama.NewAdapter(cfg.OllamaURL, cfg.Model,
			ollama.WithHTTPClient(&http.Client{Timeout: httpTimeout}),
			ollama.WithTracer(tracer),
		)
	default:
		return nil, fmt.Errorf("unsupported provider %q", cfg.Provider)
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

func anyInitSelected(selected map[string]bool, names ...string) bool {
	for _, name := range names {
		if selected[name] {
			return true
		}
	}
	return false
}

// registerBuiltinFactories wires only the builtin factory families required by
// the selected tool declarations. Program shape is still defined by machine and
// tools YAML; this bootstrap only installs factories that selected init names
// can resolve.
func registerBuiltinFactories(br *stl.BuiltinRegistry, st *agentState, selected map[string]bool) {
	if selected["file_read"] {
		br.Register("file_read", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
			return &stl.ReadBuilder{Root: vars["directory"]}, nil
		})
	}
	if selected["file_write"] {
		br.Register("file_write", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
			return &stl.WriteBuilder{Root: vars["directory"]}, nil
		})
	}
	if selected["file_edit"] {
		br.Register("file_edit", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
			return &stl.EditBuilder{Root: vars["directory"]}, nil
		})
	}
	if selected["file_find"] {
		br.Register("file_find", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
			return &stl.FindBuilder{Root: vars["directory"]}, nil
		})
	}
	if selected["file_list"] {
		br.Register("file_list", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
			return &stl.ListFilesBuilder{Root: vars["directory"]}, nil
		})
	}

	if selected["invoke_llm"] {
		br.Register("invoke_llm", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
			if st.adapter == nil {
				return nil, fmt.Errorf("invoke_llm requires --model flag")
			}
			return &stl.InvokeLLMBuilder{
				Client:       st.adapter,
				History:      st.conversation,
				Registry:     st.registry,
				Assembler:    st.assembler,
				State:        st.manifestState,
				Model:        st.model,
				ProviderName: st.providerName,
				ServerAddr:   st.serverAddr,
				Tracer:       st.tracer,
				NumCtx:       st.numCtx,
				CallTimeout:  st.callTimeout,
				Verbose:      st.verbose,
				Ctx:          st.ctx,
			}, nil
		})
	}
	if selected["parse_response"] {
		br.Register("parse_response", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
			return &stl.ParseResponseBuilder{
				Registry: st.registry,
				Parser:   st.parser,
				Tracer:   st.tracer,
				Verbose:  st.verbose,
			}, nil
		})
	}
	if selected["report_parse_error"] {
		br.Register("report_parse_error", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
			return &stl.ReportParseErrorBuilder{
				Tracer: st.tracer,
			}, nil
		})
	}
	if selected["reset_history"] {
		br.Register("reset_history", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
			return &stl.ResetHistoryBuilder{
				History: st.conversation,
				Tracer:  st.tracer,
			}, nil
		})
	}
	if selected["nudge_reread"] {
		br.Register("nudge_reread", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
			return &stl.NudgeRereadBuilder{
				Tracer: st.tracer,
			}, nil
		})
	}
	if selected["done"] {
		br.Register("done", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
			return stl.DoneBuilder{}, nil
		})
	}
	if selected["suspend"] {
		stl.RegisterLifecycleFactories(br, stl.LifecycleFactoryDeps{
			StateStore: st.stateStore,
			Tracer:     st.tracer,
		})
	}
	if selected["validate"] {
		br.Register("validate", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
			return &stl.ValidateBuilder{
				Tracker:  st.tracker,
				Registry: st.registry,
				Tracer:   st.tracer,
				Verbose:  st.verbose,
			}, nil
		})
	}
	if selected["self_invoke"] {
		br.Register("self_invoke", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
			var parsed stl.ChildAgentConfig
			if err := stl.DecodeToolConfig(def, &parsed); err != nil {
				return nil, err
			}
			cfg := execute.Config{
				Machine:          parsed.Machine,
				Tools:            parsed.Tools,
				ToolDeclarations: parsed.ToolDeclarations,
				Model:            vars["model"],
				OllamaURL:        vars["ollama_url"],
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
	}

	if anyInitSelected(selected,
		"extract_task", "extract_all", "assemble_prompt", "parse_plan",
		"create_issue", "execute_task", "check_result",
	) {
		pipeline.RegisterFactories(br, pipeline.FactoryDeps{
			Directory: st.directory,
			Tracer:    st.tracer,
			Ctx:       st.ctx,
		})
	}

	if anyInitSelected(selected,
		"parse_suite_config", "discover_suite_samples", "expand_eval_grid",
		"init_eval_session", "report_suite_summary",
		"next_point", "run_point", "report_session",
		"run_agent", "run_oracle_check", "collect_trace_tokens",
		"check_agent_version", "summarize_point_results", "collect_metrics",
		"dump_config",
	) {
		stl.RegisterEvalFactories(br, stl.EvalFactoryDeps{
			Ctx:       st.ctx,
			Registry:  st.registry,
			Stderr:    os.Stderr,
			SuitePath: flagInput,
			OutputDir: flagOutput,
			OllamaURL: flagOllamaURL,
		})
	}

	if anyInitSelected(selected, "serve_ui", "launch_eval") {
		bench.RegisterFactories(br, benchui.Assets())
	}

	if anyInitSelected(selected, "load_corpus", "validate_specs", "format_report") {
		stl.RegisterValidateFactories(br, st.directory)
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
