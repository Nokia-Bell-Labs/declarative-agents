// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/support/execute"
)

// BuiltinFactory creates a Builder from a tool definition's config map.
// Each builtin tool type registers a factory under a unique init name.
type BuiltinFactory func(def ToolDef, vars map[string]string) (core.Builder, error)

// BuiltinRegistry maps init names to their factory functions.
type BuiltinRegistry struct {
	factories map[string]BuiltinFactory
}

// NewBuiltinRegistry creates an empty builtin registry.
func NewBuiltinRegistry() *BuiltinRegistry {
	return &BuiltinRegistry{factories: make(map[string]BuiltinFactory)}
}

// Register adds a factory under the given init name.
func (br *BuiltinRegistry) Register(initName string, factory BuiltinFactory) {
	if _, exists := br.factories[initName]; exists {
		panic(fmt.Sprintf("builtin registry: duplicate init name %q", initName))
	}
	br.factories[initName] = factory
}

// Override replaces the factory for the given init name, or registers
// it if not present. Use this in tests to replace real factories with
// stubs.
func (br *BuiltinRegistry) Override(initName string, factory BuiltinFactory) {
	br.factories[initName] = factory
}

// Resolve looks up a factory by init name.
func (br *BuiltinRegistry) Resolve(initName string) (BuiltinFactory, bool) {
	f, ok := br.factories[initName]
	return f, ok
}

// Names returns all registered init names sorted.
func (br *BuiltinRegistry) Names() []string {
	names := make([]string, 0, len(br.factories))
	for n := range br.factories {
		names = append(names, n)
	}
	return names
}

// RegisterUnifiedTools loads a tools YAML file that may contain both
// exec and builtin tool definitions, and registers them all with the
// core registry.
//
// For exec tools (type: exec or no type): creates ExecBuilder as before.
// For builtin tools (type: builtin): looks up the init name in the
// BuiltinRegistry and calls the factory to create a Builder.
//
// The vars map provides template variable resolution (e.g. "model",
// "directory") for tool config values.
// RegisterSingleBuiltin resolves a single builtin tool from the
// BuiltinRegistry and registers (or overrides) it in the core Registry.
func RegisterSingleBuiltin(reg *core.Registry, builtins *BuiltinRegistry, td ToolDef, vars map[string]string) error {
	if td.Init == "" {
		return fmt.Errorf("builtin tool %q has no init field", td.Name)
	}
	factory, ok := builtins.Resolve(td.Init)
	if !ok {
		return fmt.Errorf("builtin tool %q: unknown init %q", td.Name, td.Init)
	}
	builder, err := factory(td, vars)
	if err != nil {
		return fmt.Errorf("builtin tool %q init: %w", td.Name, err)
	}
	reg.Override(td.ToToolSpec(), builder)
	return nil
}

func RegisterUnifiedTools(reg *core.Registry, builtins *BuiltinRegistry, root string, defs []ToolDef, vars map[string]string) error {
	for _, td := range defs {
		spec := td.ToToolSpec()

		switch td.Type {
		case "builtin":
			if td.Init == "" {
				return fmt.Errorf("builtin tool %q has no init field", td.Name)
			}
			factory, ok := builtins.Resolve(td.Init)
			if !ok {
				return fmt.Errorf("builtin tool %q: unknown init %q", td.Name, td.Init)
			}
			builder, err := factory(td, vars)
			if err != nil {
				return fmt.Errorf("builtin tool %q init: %w", td.Name, err)
			}
			reg.Register(spec, builder)

		case "exec", "":
			if td.Binary == "" {
				return fmt.Errorf("exec tool %q has no binary", td.Name)
			}
			builder := &ExecBuilder{Def: td, Root: root}
			reg.Register(spec, builder)

		default:
			return fmt.Errorf("tool %q: unknown type %q", td.Name, td.Type)
		}
	}
	return nil
}

// StandardFactoryDeps are runtime ports needed by the standard builtin
// factories. Package-family hooks avoid import cycles with packages that
// already depend on stl.
type StandardFactoryDeps struct {
	Conversation       *llm.Conversation
	Registry           *core.Registry
	Parser             func() llm.ResponseParser
	Tracer             tracing.Tracer
	ProfilesDir        string
	Verbose            bool
	Ctx                context.Context
	Directory          string
	StateStore         core.StateStore
	Tracker            *ToolTracker
	ParseRetries       *ParseErrorRetryTracker
	OnModelResolved    func(InvokeLLMResolvedConfig)
	RegisterPlanning   func(*BuiltinRegistry)
	RegisterEvaluation func(*BuiltinRegistry)
	RegisterBench      func(*BuiltinRegistry)
}

// StandardFactoryCatalogEntry describes one selected-init-gated factory family.
type StandardFactoryCatalogEntry struct {
	Name     string
	Inits    []string
	register func(*BuiltinRegistry)
}

// SelectedBy reports whether any entry init is selected.
func (e StandardFactoryCatalogEntry) SelectedBy(selected map[string]bool) bool {
	for _, init := range e.Inits {
		if selected[init] {
			return true
		}
	}
	return false
}

// SelectedBuiltinInits returns the builtin init keys present in selected defs.
func SelectedBuiltinInits(defs []ToolDef) map[string]bool {
	selected := make(map[string]bool)
	for _, def := range defs {
		if def.Type == "builtin" && def.Init != "" {
			selected[def.Init] = true
		}
	}
	return selected
}

// RegisterStandardBuiltinFactories registers only the selected standard
// builtin families.
func RegisterStandardBuiltinFactories(br *BuiltinRegistry, selected map[string]bool, deps StandardFactoryDeps) {
	for _, entry := range StandardFactoryCatalog(deps) {
		if entry.SelectedBy(selected) {
			entry.register(br)
		}
	}
}

// StandardFactoryCatalog returns the standard builtin factory families.
func StandardFactoryCatalog(deps StandardFactoryDeps) []StandardFactoryCatalogEntry {
	return []StandardFactoryCatalogEntry{
		fileFactory("file_read", "file_read", func(root string) core.Builder { return &ReadBuilder{Root: root} }),
		fileFactory("file_write", "file_write", func(root string) core.Builder { return &WriteBuilder{Root: root} }),
		fileFactory("file_edit", "file_edit", func(root string) core.Builder { return &EditBuilder{Root: root} }),
		fileFactory("file_find", "file_find", func(root string) core.Builder { return &FindBuilder{Root: root} }),
		fileFactory("file_list", "file_list", func(root string) core.Builder { return &ListFilesBuilder{Root: root} }),
		invokeLLMFactory(deps),
		parseResponseFactory(deps),
		reportParseErrorFactory(deps),
		resetHistoryFactory(deps),
		nudgeRereadFactory(deps),
		doneFactory(),
		lifecycleFactory(deps),
		validateFactory(deps),
		selfInvokeFactory(deps),
		hookFactory("planning", []string{"extract_task", "extract_all", "assemble_prompt", "parse_plan", "create_issue", "execute_task", "check_result"}, deps.RegisterPlanning),
		hookFactory("evaluation", []string{"parse_suite_config", "discover_suite_samples", "expand_eval_grid", "init_eval_session", "report_suite_summary", "next_point", "run_point", "report_session", "run_agent", "run_oracle_check", "collect_trace_tokens", "check_agent_version", "summarize_point_results", "collect_metrics", "dump_config"}, deps.RegisterEvaluation),
		hookFactory("bench", []string{"serve_ui", "launch_eval"}, deps.RegisterBench),
		specValidationFactory(deps),
	}
}

func fileFactory(name, init string, builder func(string) core.Builder) StandardFactoryCatalogEntry {
	return StandardFactoryCatalogEntry{Name: name, Inits: []string{init}, register: func(br *BuiltinRegistry) {
		br.Register(init, func(def ToolDef, vars map[string]string) (core.Builder, error) {
			return builder(vars["directory"]), nil
		})
	}}
}

func invokeLLMFactory(deps StandardFactoryDeps) StandardFactoryCatalogEntry {
	return StandardFactoryCatalogEntry{Name: "invoke_llm", Inits: []string{"invoke_llm"}, register: func(br *BuiltinRegistry) {
		br.Register("invoke_llm", func(def ToolDef, vars map[string]string) (core.Builder, error) {
			return NewInvokeLLMBuilder(def, InvokeLLMFactoryDeps{
				History:     deps.Conversation,
				Registry:    deps.Registry,
				Tracer:      deps.Tracer,
				ProfilesDir: deps.ProfilesDir,
				Verbose:     deps.Verbose,
				Ctx:         deps.Ctx,
				OnResolved:  deps.OnModelResolved,
			})
		})
	}}
}

func parseResponseFactory(deps StandardFactoryDeps) StandardFactoryCatalogEntry {
	return StandardFactoryCatalogEntry{Name: "parse_response", Inits: []string{"parse_response"}, register: func(br *BuiltinRegistry) {
		br.Register("parse_response", func(def ToolDef, vars map[string]string) (core.Builder, error) {
			return &ParseResponseBuilder{
				Registry: deps.Registry,
				Parser:   currentParser(deps),
				Tracer:   deps.Tracer,
				Verbose:  deps.Verbose,
				Retry:    deps.ParseRetries,
			}, nil
		})
	}}
}

func currentParser(deps StandardFactoryDeps) llm.ResponseParser {
	if deps.Parser == nil {
		return nil
	}
	return deps.Parser()
}

func reportParseErrorFactory(deps StandardFactoryDeps) StandardFactoryCatalogEntry {
	return StandardFactoryCatalogEntry{Name: "report_parse_error", Inits: []string{"report_parse_error"}, register: func(br *BuiltinRegistry) {
		br.Register("report_parse_error", func(def ToolDef, vars map[string]string) (core.Builder, error) {
			return &ReportParseErrorBuilder{Tracer: deps.Tracer, Retry: deps.ParseRetries}, nil
		})
	}}
}

func resetHistoryFactory(deps StandardFactoryDeps) StandardFactoryCatalogEntry {
	return StandardFactoryCatalogEntry{Name: "reset_history", Inits: []string{"reset_history"}, register: func(br *BuiltinRegistry) {
		br.Register("reset_history", func(def ToolDef, vars map[string]string) (core.Builder, error) {
			return &ResetHistoryBuilder{History: deps.Conversation, Tracer: deps.Tracer}, nil
		})
	}}
}

func nudgeRereadFactory(deps StandardFactoryDeps) StandardFactoryCatalogEntry {
	return StandardFactoryCatalogEntry{Name: "nudge_reread", Inits: []string{"nudge_reread"}, register: func(br *BuiltinRegistry) {
		br.Register("nudge_reread", func(def ToolDef, vars map[string]string) (core.Builder, error) {
			return &NudgeRereadBuilder{Tracer: deps.Tracer}, nil
		})
	}}
}

func doneFactory() StandardFactoryCatalogEntry {
	return StandardFactoryCatalogEntry{Name: "done", Inits: []string{"done"}, register: func(br *BuiltinRegistry) {
		br.Register("done", func(def ToolDef, vars map[string]string) (core.Builder, error) {
			return DoneBuilder{}, nil
		})
	}}
}

func lifecycleFactory(deps StandardFactoryDeps) StandardFactoryCatalogEntry {
	return StandardFactoryCatalogEntry{Name: "lifecycle", Inits: []string{"suspend", "checkpoint_history", "checkpoint_rollback"}, register: func(br *BuiltinRegistry) {
		RegisterLifecycleFactories(br, LifecycleFactoryDeps{StateStore: deps.StateStore, Tracer: deps.Tracer})
		registerCheckpointLifecycleFactories(br, deps)
	}}
}

func registerCheckpointLifecycleFactories(br *BuiltinRegistry, deps StandardFactoryDeps) {
	br.Register("checkpoint_history", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		var cfg CheckpointHistoryConfig
		if err := DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		return &CheckpointHistoryBuilder{Config: cfg, StateStore: deps.StateStore, Ctx: deps.Ctx}, nil
	})
	br.Register("checkpoint_rollback", func(def ToolDef, vars map[string]string) (core.Builder, error) {
		var cfg CheckpointRollbackConfig
		if err := DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		return &CheckpointRollbackBuilder{
			Config:     cfg,
			StateStore: deps.StateStore,
			Directory:  deps.Directory,
			Tracer:     deps.Tracer,
			Ctx:        deps.Ctx,
		}, nil
	})
}

func validateFactory(deps StandardFactoryDeps) StandardFactoryCatalogEntry {
	return StandardFactoryCatalogEntry{Name: "validate", Inits: []string{"validate"}, register: func(br *BuiltinRegistry) {
		br.Register("validate", func(def ToolDef, vars map[string]string) (core.Builder, error) {
			return &ValidateBuilder{Tracker: deps.Tracker, Registry: deps.Registry, Tracer: deps.Tracer, Verbose: deps.Verbose}, nil
		})
	}}
}

func selfInvokeFactory(deps StandardFactoryDeps) StandardFactoryCatalogEntry {
	return StandardFactoryCatalogEntry{Name: "self_invoke", Inits: []string{"self_invoke"}, register: func(br *BuiltinRegistry) {
		br.Register("self_invoke", func(def ToolDef, vars map[string]string) (core.Builder, error) {
			parsed, err := decodeChildAgent(def)
			if err != nil {
				return nil, err
			}
			return &SelfInvokeBuilder{
				Config:    childExecuteConfig(parsed, vars["model"]),
				ExtraArgs: directoryArgs(vars["directory"]),
				Ctx:       deps.Ctx,
				Tracer:    deps.Tracer,
			}, nil
		})
	}}
}

func decodeChildAgent(def ToolDef) (ChildAgentConfig, error) {
	var parsed ChildAgentConfig
	if err := DecodeToolConfig(def, &parsed); err != nil {
		return ChildAgentConfig{}, err
	}
	if err := ValidateChildAgentConfig(def.Name, parsed); err != nil {
		return ChildAgentConfig{}, err
	}
	return parsed, nil
}

func childExecuteConfig(parsed ChildAgentConfig, model string) execute.Config {
	return execute.Config{
		Profile:          parsed.Profile,
		Machine:          parsed.Machine,
		Tools:            parsed.Tools,
		ToolDeclarations: parsed.ToolDeclarations,
		Model:            model,
	}
}

func directoryArgs(directory string) []string {
	if directory == "" {
		return nil
	}
	return []string{"--directory", directory}
}

func hookFactory(name string, inits []string, hook func(*BuiltinRegistry)) StandardFactoryCatalogEntry {
	return StandardFactoryCatalogEntry{Name: name, Inits: inits, register: func(br *BuiltinRegistry) {
		if hook != nil {
			hook(br)
		}
	}}
}

func specValidationFactory(deps StandardFactoryDeps) StandardFactoryCatalogEntry {
	return StandardFactoryCatalogEntry{Name: "spec_validation", Inits: []string{"load_corpus", "validate_specs", "format_report"}, register: func(br *BuiltinRegistry) {
		RegisterValidateFactories(br, deps.Directory)
	}}
}

// DynamicToolActionDeps are the runtime ports for $tool dispatch.
type DynamicToolActionDeps struct {
	Registry *core.Registry
	Tracker  *ToolTracker
	Tracer   tracing.Tracer
	Verbose  bool
}

// BuildDynamicToolAction builds the ActionFunc used by dynamic $tool dispatch.
func BuildDynamicToolAction(deps DynamicToolActionDeps) core.ActionFunc {
	return func(r core.Result) core.Command {
		var treq llm.ToolRequest
		if err := json.Unmarshal([]byte(r.Output), &treq); err != nil {
			return &standardFailCmd{err: fmt.Errorf("failed to unmarshal ToolRequest: %w", err)}
		}
		builder, ok := deps.Registry.Resolve(treq.ToolName)
		if !ok {
			return &standardFailCmd{err: fmt.Errorf("no builder for tool %q", treq.ToolName)}
		}
		if deps.Tracker != nil {
			deps.Tracker.Record(treq.ToolName)
		}
		cmd := builder.Build(core.Result{Output: r.Output})
		if !deps.Verbose {
			return cmd
		}
		return &tracedDynamicToolCmd{inner: cmd, tracer: deps.Tracer, toolName: treq.ToolName, params: string(treq.Params)}
	}
}

type standardFailCmd struct {
	err error
}

func (f *standardFailCmd) Name() string      { return "fail" }
func (f *standardFailCmd) Undo() core.Result { return core.NoopUndo(f.Name()) }

func (f *standardFailCmd) Execute() core.Result {
	return core.Result{Signal: core.CommandError, Err: f.err, Output: f.err.Error(), CommandName: "fail"}
}

type tracedDynamicToolCmd struct {
	inner    core.Command
	tracer   tracing.Tracer
	toolName string
	params   string
}

func (t *tracedDynamicToolCmd) Name() string      { return t.inner.Name() }
func (t *tracedDynamicToolCmd) Undo() core.Result { return t.inner.Undo() }

func (t *tracedDynamicToolCmd) Execute() core.Result {
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
