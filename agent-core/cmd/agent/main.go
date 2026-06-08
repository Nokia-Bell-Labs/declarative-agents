// Copyright (c) 2026 Nokia. All rights reserved.

// Command agent is the unified agentic-loop binary. It loads a state machine
// and tools from YAML configuration, then runs core.Loop. Different modes
// (generate, pipeline, eval) are selected entirely by config files.
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

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/eval"
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
	flagLLM              []string
	flagOTelLog          string
	flagOTelParent       string
	flagDirectory        string
	flagPrompt           string
	flagPromptString     string
	flagProfilesDir      string
	flagVerboseTrace     bool
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

Different modes (generate, pipeline, eval) are selected by which
--machine and --tools files you pass.`,
	SilenceUsage: true,
	RunE:         run,
}

func init() {
	f := rootCmd.Flags()
	f.StringVar(&flagMachine, "machine", "", "path to state machine YAML (required)")
	f.StringVar(&flagTools, "tools", "", "path to tool selection YAML (required)")
	f.StringArrayVar(&flagToolDeclarations, "tools-declaration", nil, "path to tool declaration YAML (repeatable)")
	f.StringArrayVar(&flagLLM, "llm", nil, "path to LLM config YAML (repeatable, registers invoke_llm tools)")
	f.StringVar(&flagOTelLog, "otel-log-file", "", "path to OTel trace output file")
	f.StringVar(&flagOTelParent, "otel-parent-span", "", "W3C traceparent for parent span")
	f.StringVar(&flagDirectory, "directory", "", "workspace directory")
	f.StringVar(&flagPrompt, "prompt", "", "path to prompt YAML file")
	f.StringVar(&flagPromptString, "prompt-string", "", "inline prompt text (alternative to --prompt)")
	f.StringVar(&flagProfilesDir, "profiles-dir", "", "directory with model profile YAML files (overrides embedded)")
	f.BoolVar(&flagVerboseTrace, "verbose-trace", false, "record LLM input/output in traces")

	rootCmd.AddCommand(evalCmd)

	rootCmd.Version = "v0.0.0-dev"
}

var evalCmd = &cobra.Command{
	Use:   "eval <suite.yaml>",
	Short: "Run an evaluation suite",
	Long: `Runs an evaluation suite defined in YAML. The suite specifies harnesses,
models, grid parameters, and sample directories. Each combination is
executed as an evaluation point.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		suite, err := eval.LoadSuite(args[0])
		if err != nil {
			return fmt.Errorf("load suite: %w", err)
		}

		outputDir, _ := cmd.Flags().GetString("output")
		if outputDir == "" {
			outputDir = "eval-results"
		}

		reps, _ := cmd.Flags().GetInt("reps")
		if reps == 0 && suite.Reps > 0 {
			reps = suite.Reps
		}
		if reps == 0 {
			reps = 1
		}

		cfg := eval.SessionConfig{
			OutputDir:  outputDir,
			OllamaURL:  suite.OllamaURL,
			Timeout:    suite.Timeout,
			Reps:       reps,
			Stderr:     os.Stderr,
		}

		result, err := eval.RunSession(cmd.Context(), suite, cfg)
		if err != nil {
			return err
		}

		if result.Passed < result.TotalPoints {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	evalCmd.Flags().String("output", "eval-results", "output directory for results")
	evalCmd.Flags().Int("reps", 1, "number of repetitions per point")
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
	numCtx        int
	callTimeout   time.Duration
	verbose       bool
	ctx           context.Context
	directory     string
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

	// Load prompt (optional — not all machines need it, e.g. eval)
	var agentPrompt prompt.Prompt
	if flagPrompt != "" && flagPromptString != "" {
		return fmt.Errorf("--prompt and --prompt-string are mutually exclusive")
	}
	if flagPrompt != "" {
		lr, err := prompt.LoadPrompt(flagPrompt)
		if err != nil {
			return fmt.Errorf("load prompt: %w", err)
		}
		agentPrompt = lr.Prompt
		tracer.Event("prompt.loaded",
			attribute.String("prompt.path", flagPrompt),
			attribute.String("prompt.mode", lr.Mode),
			attribute.Int("prompt.task_len", lr.TaskLen),
		)
	} else if flagPromptString != "" {
		lr, err := prompt.LoadPromptFromString(flagPromptString)
		if err != nil {
			return fmt.Errorf("load prompt string: %w", err)
		}
		agentPrompt = lr.Prompt
		tracer.Event("prompt.loaded",
			attribute.String("prompt.source", "string"),
			attribute.Int("prompt.task_len", lr.TaskLen),
		)
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

	// Load LLM configs from --llm flags, or fall back to legacy extractLLMConfig
	var llmConfigs []stl.LLMConfig
	if len(flagLLM) > 0 {
		llmConfigs, err = stl.LoadLLMConfigs(flagLLM)
		if err != nil {
			return fmt.Errorf("load llm configs: %w", err)
		}
	}

	var llmCfg llmConfig
	if len(llmConfigs) > 0 {
		primary := llmConfigs[0]
		llmCfg = llmConfig{
			Model:      primary.Model,
			OllamaURL:  primary.EffectiveProviderURL(),
			NumCtx:     primary.NumCtx,
			LLMTimeout: primary.LLMTimeoutDuration(),
			MaxTime:    primary.MaxTimeDuration(),
			MaxTokens:  primary.MaxTokens,
		}
		if agentPrompt.Task == "" && primary.SystemPrompt != "" {
			agentPrompt = prompt.Prompt{
				Role:         primary.SystemPrompt,
				OutputFormat: primary.ToolPrompt,
			}
		}
	} else {
		llmCfg = extractLLMConfig(defs)
	}
	applyLLMEnvOverrides(&llmCfg)

	// Create Ollama adapter (only if model is configured)
	var adapter llm.Client
	var serverAddr string
	var profileReg *llm.ProfileRegistry
	var parser llm.ResponseParser
	if llmCfg.Model != "" {
		httpTimeout := 5 * time.Minute
		if llmCfg.MaxTime > 0 && llmCfg.MaxTime < httpTimeout {
			httpTimeout = llmCfg.MaxTime
		}
		adapter, err = ollama.NewAdapter(llmCfg.OllamaURL, llmCfg.Model,
			ollama.WithHTTPClient(&http.Client{Timeout: httpTimeout}),
			ollama.WithTracer(tracer),
		)
		if err != nil {
			return fmt.Errorf("ollama adapter: %w", err)
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
			flagMachine = filepath.Join(filepath.Dir(flagMachine), profileSpec.MachineName+".yaml")
			tracer.Event("setup.machine_from_profile",
				attribute.String("machine.resolved_path", flagMachine),
			)
		}
	}

	// Create assembler
	var assembler llm.PromptAssembler
	if agentPrompt.Task != "" || agentPrompt.Role != "" {
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

	vars := map[string]string{
		"model":         llmCfg.Model,
		"directory":     flagDirectory,
		"ollama_url":    llmCfg.OllamaURL,
		"prompt_string": flagPromptString,
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
		providerName:  "ollama",
		serverAddr:    serverAddr,
		numCtx:        llmCfg.NumCtx,
		callTimeout:   llmCfg.LLMTimeout,
		verbose:       flagVerboseTrace,
		ctx:           cmd.Context(),
		directory:     flagDirectory,
	}

	registerBuiltinFactories(builtins, st)

	if err := stl.RegisterUnifiedTools(reg, builtins, flagDirectory, defs, vars); err != nil {
		return fmt.Errorf("register tools: %w", err)
	}

	// Register LLM tools from --llm configs
	for _, lc := range llmConfigs {
		toolName := lc.EffectiveName()
		reg.Register(core.ToolSpec{
			Name:        toolName,
			Description: fmt.Sprintf("Invoke %s via %s", lc.Model, lc.Provider),
			Visibility:  core.Internal,
		}, &stl.InvokeLLMBuilder{
			Client:       adapter,
			History:      conversation,
			Registry:     reg,
			Assembler:    assembler,
			Model:        llmCfg.Model,
			ProviderName: lc.Provider,
			ServerAddr:   serverAddr,
			Tracer:       tracer,
			NumCtx:       llmCfg.NumCtx,
			CallTimeout:  llmCfg.LLMTimeout,
			Verbose:      flagVerboseTrace,
			Ctx:          cmd.Context(),
		})
	}

	// Build $tool action (dynamic tool dispatch from parse_response output)
	toolAction := buildToolAction(st, reg)

	// Build budget: defaults from machine.yaml, overridden by LLM config
	machineSpec, err := core.LoadMachineSpec(flagMachine)
	if err != nil {
		return fmt.Errorf("load machine spec for budget: %w", err)
	}
	budgetDefaults := core.Budget{
		MaxIterations:            100,
		MaxConsecutiveParseErrors: 5,
	}
	budget := machineSpec.BudgetSpec.ToBudget(budgetDefaults)
	if llmCfg.MaxTime > 0 {
		budget.MaxDuration = llmCfg.MaxTime
	}
	if llmCfg.MaxTokens > 0 {
		budget.MaxTokens = llmCfg.MaxTokens
	}

	// Run the loop
	params := core.LoopParams{
		MachineFile:  flagMachine,
		AgentName:    "agent",
		ModelName:    llmCfg.Model,
		ProviderName: "ollama",
		Trace:        tracer,
		Budget:       budget,
		ToolAction:   toolAction,
		Registry:     reg,
		Directory:    flagDirectory,
	}

	result, err := core.Loop(params, context.Background())
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

// llmConfig holds LLM-related settings resolved from tools.yaml config
// and optionally overridden by CLI flags.
type llmConfig struct {
	Model      string
	OllamaURL  string
	NumCtx     int
	LLMTimeout time.Duration
	MaxTime    time.Duration
	MaxTokens  int
}

// extractLLMConfig scans tool definitions for an invoke_llm tool and
// extracts LLM settings from its config block. Legacy path used when
// no --llm flags are provided.
func extractLLMConfig(defs []stl.ToolDef) llmConfig {
	cfg := llmConfig{
		OllamaURL: "http://localhost:11434",
	}
	for _, td := range defs {
		if td.Init != "invoke_llm" {
			continue
		}
		if v, ok := td.Config["model"].(string); ok && v != "" {
			cfg.Model = v
		}
		if v, ok := td.Config["ollama_url"].(string); ok && v != "" {
			cfg.OllamaURL = v
		}
		if v := configInt(td.Config, "num_ctx"); v > 0 {
			cfg.NumCtx = v
		}
		if v := configInt(td.Config, "llm_timeout"); v > 0 {
			cfg.LLMTimeout = time.Duration(v) * time.Second
		}
		if v := configInt(td.Config, "max_time"); v > 0 {
			cfg.MaxTime = time.Duration(v) * time.Second
		}
		if v := configInt(td.Config, "max_tokens"); v > 0 {
			cfg.MaxTokens = v
		}
		break
	}
	return cfg
}

// applyLLMEnvOverrides applies environment variable overrides to an
// llmConfig. AGENT_MODEL, AGENT_OLLAMA_URL, AGENT_NUM_CTX,
// AGENT_LLM_TIMEOUT, AGENT_MAX_TIME, AGENT_MAX_TOKENS.
func applyLLMEnvOverrides(cfg *llmConfig) {
	if v := os.Getenv("AGENT_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("AGENT_OLLAMA_URL"); v != "" {
		cfg.OllamaURL = v
	}
	if v := envInt("AGENT_NUM_CTX"); v > 0 {
		cfg.NumCtx = v
	}
	if v := envInt("AGENT_LLM_TIMEOUT"); v > 0 {
		cfg.LLMTimeout = time.Duration(v) * time.Second
	}
	if v := envInt("AGENT_MAX_TIME"); v > 0 {
		cfg.MaxTime = time.Duration(v) * time.Second
	}
	if v := envInt("AGENT_MAX_TOKENS"); v > 0 {
		cfg.MaxTokens = v
	}
}

// envInt reads an environment variable as an integer, returning 0 if
// unset or unparseable.
func envInt(key string) int {
	v := os.Getenv(key)
	if v == "" {
		return 0
	}
	n, _ := fmt.Sscanf(v, "%d", new(int))
	if n == 0 {
		return 0
	}
	var val int
	fmt.Sscanf(v, "%d", &val)
	return val
}

// configInt extracts an integer from a map[string]interface{}, handling
// both int and float64 (YAML numbers decode as int via go-yaml).
func configInt(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}

// registerBuiltinFactories wires all known builtin tool factories.
func registerBuiltinFactories(br *stl.BuiltinRegistry, st *agentState) {
	// File tools
	br.Register("file_read", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.ReadBuilder{Root: vars["directory"]}, nil
	})
	br.Register("file_write", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.WriteBuilder{Root: vars["directory"]}, nil
	})
	br.Register("file_edit", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.EditBuilder{Root: vars["directory"]}, nil
	})
	br.Register("file_find", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.FindBuilder{Root: vars["directory"]}, nil
	})
	br.Register("file_list", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.ListFilesBuilder{Root: vars["directory"]}, nil
	})

	// LLM tools
	br.Register("invoke_llm", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		if st.adapter == nil {
			return nil, fmt.Errorf("invoke_llm requires --model flag")
		}
		return &stl.InvokeLLMBuilder{
			Client:       st.adapter,
			History:      st.conversation,
			Registry:     st.registry,
			Assembler:    st.assembler,
			State:        core.State("Composing"),
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
	br.Register("parse_response", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.ParseResponseBuilder{
			Registry: st.registry,
			Parser:   st.parser,
			Tracer:   st.tracer,
			Verbose:  st.verbose,
		}, nil
	})
	br.Register("report_parse_error", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.ReportParseErrorBuilder{
			Tracer: st.tracer,
		}, nil
	})
	br.Register("reset_history", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.ResetHistoryBuilder{
			History: st.conversation,
			Tracer:  st.tracer,
		}, nil
	})

	// Nudge reread (used by deepseek machine variant)
	br.Register("nudge_reread", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.NudgeRereadBuilder{
			Tracer: st.tracer,
		}, nil
	})

	// Done (TaskCompleted signal)
	br.Register("done", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return stl.DoneBuilder{}, nil
	})

	// Validate (runs skipped build/lint/test tools from registry)
	br.Register("validate", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.ValidateBuilder{
			Tracker:  st.tracker,
			Registry: st.registry,
			Tracer:   st.tracer,
			Verbose:  st.verbose,
		}, nil
	})

	// Self-invoke (for pipeline→generate child calls)
	br.Register("self_invoke", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		cfg := stl.SelfInvokeConfig{
			Directory: vars["directory"],
			Model:     vars["model"],
			OllamaURL: vars["ollama_url"],
		}
		return &stl.SelfInvokeBuilder{
			Config: cfg,
			Ctx:    st.ctx,
			Tracer: st.tracer,
		}, nil
	})

	// Pipeline tools (extract_task, extract_all, assemble_prompt, parse_plan,
	// create_issue, execute_task, check_result)
	registerPipelineFactories(br, st)

	// Eval tools (prepare_workspace, run_agent, check_results,
	// collect_metrics, summarize)
	registerEvalFactories(br, st)

	// Validate spec tools (load_corpus, validate_specs, format_report)
	registerValidateSpecFactories(br, st)

}

// registerPipelineFactories registers real factories for pipeline tools.
// PipelineState is lazily initialized on first factory call.
func registerPipelineFactories(br *stl.BuiltinRegistry, st *agentState) {
	var ps *pipeline.State

	initPS := func() *pipeline.State {
		if ps != nil {
			return ps
		}
		ps = &pipeline.State{
			Directory: st.directory,
			Tracer:    st.tracer,
			Ctx:       st.ctx,
			TaskDeps:  make(map[string]string),
		}
		return ps
	}

	br.Register("extract_task", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &pipeline.ExtractTaskBuilder{PS: initPS()}, nil
	})
	br.Register("extract_all", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &pipeline.ExtractAllBuilder{PS: initPS()}, nil
	})
	br.Register("assemble_prompt", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &pipeline.AssemblePromptBuilder{PS: initPS()}, nil
	})
	br.Register("parse_plan", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &pipeline.ParsePlanBuilder{PS: initPS()}, nil
	})
	br.Register("create_issue", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &pipeline.CreateIssueBuilder{PS: initPS()}, nil
	})
	br.Register("execute_task", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &pipeline.ExecuteTaskBuilder{PS: initPS()}, nil
	})
	br.Register("check_result", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &pipeline.CheckResultBuilder{PS: initPS()}, nil
	})
}

// registerValidateSpecFactories registers factories for spec validation tools.
func registerValidateSpecFactories(br *stl.BuiltinRegistry, st *agentState) {
	var vs *stl.ValidateSpecState

	initVS := func() *stl.ValidateSpecState {
		if vs != nil {
			return vs
		}
		vs = &stl.ValidateSpecState{Directory: st.directory}
		return vs
	}

	br.Register("load_corpus", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		s := initVS()
		if dir := vars["directory"]; dir != "" {
			s.Directory = dir
		}
		return &stl.LoadCorpusBuilder{VS: s}, nil
	})
	br.Register("validate_specs", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.ValidateSpecsBuilder{VS: initVS()}, nil
	})
	br.Register("format_report", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &stl.FormatReportBuilder{VS: initVS()}, nil
	})
}


// registerEvalFactories registers factories for eval tools.
// These use the existing pkg/eval/ implementations. The PointContext
// must be set on the builders before they can execute (done by the
// eval session orchestrator).
func registerEvalFactories(br *stl.BuiltinRegistry, st *agentState) {
	br.Register("prepare_workspace", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &evalStaticFactory{name: "prepare_workspace"}, nil
	})
	br.Register("run_agent", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &evalStaticFactory{name: "run_agent"}, nil
	})
	br.Register("check_results", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &evalStaticFactory{name: "check_results"}, nil
	})
	br.Register("collect_metrics", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &evalStaticFactory{name: "collect_metrics"}, nil
	})
	br.Register("summarize", func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &evalStaticFactory{name: "summarize"}, nil
	})
}

// evalStaticFactory is a placeholder builder for eval tools. The eval
// mode uses RegisterExperimentTools (in pkg/eval/experiment.go) which
// replaces these with PointContext-bound builders before the loop runs.
type evalStaticFactory struct {
	name string
}

func (f *evalStaticFactory) Build(_ core.Result) core.Command {
	return &failCmd{err: fmt.Errorf("eval tool %q: PointContext not initialized (use eval session mode)", f.name)}
}

// failCmd immediately returns CommandError.
type failCmd struct {
	err error
}

func (f *failCmd) Name() string { return "fail" }
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

func (t *tracedToolCmd) Name() string { return t.inner.Name() }
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
