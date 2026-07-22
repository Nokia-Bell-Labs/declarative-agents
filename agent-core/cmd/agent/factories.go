// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/evaluation"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/evaluation/bench"
	benchui "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/evaluation/bench/ui"
	docsapi "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/knowledge/documentation"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/pipeline"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/execute"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/compose"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/control"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/filesystem"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/lifecycle"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/validation"
)

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
		RegisterREST:           registerRESTFactories(st),
		RegisterDocumentation:  registerDocumentationFactories(),
		RegisterCompose:        registerComposeFactories(),
	}
}

func registerComposeFactories() toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		br.Register("compose", func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
			var cfg catalog.ComposeConfig
			if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
				return nil, err
			}
			return compose.Builder{
				ToolName: def.Name,
				Template: cfg.Template,
				Inputs:   cfg.Inputs,
				Signal:   core.Signal(cfg.Signal),
			}, nil
		})
	}
}

func registerFilesystemFactories() toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		fileFactories := []struct {
			init    string
			builder func(string, core.MetricConfig) core.Builder
		}{
			{"file_read", func(root string, metrics core.MetricConfig) core.Builder {
				return &filesystem.ReadBuilder{Root: root, Metrics: metrics}
			}},
			{"file_write", func(root string, metrics core.MetricConfig) core.Builder {
				return &filesystem.WriteBuilder{Root: root, Metrics: metrics}
			}},
			{"file_edit", func(root string, metrics core.MetricConfig) core.Builder {
				return &filesystem.EditBuilder{Root: root, Metrics: metrics}
			}},
			{"file_find", func(root string, _ core.MetricConfig) core.Builder { return &filesystem.FindBuilder{Root: root} }},
			{"file_list", func(root string, _ core.MetricConfig) core.Builder { return &filesystem.ListFilesBuilder{Root: root} }},
		}
		for _, entry := range fileFactories {
			registerFileFactory(br, entry.init, entry.builder)
		}
		registerResourceFactories(br)
	}
}

func registerFileFactory(br *toolregistry.BuiltinRegistry, init string, builder func(string, core.MetricConfig) core.Builder) {
	br.Register(init, func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return builder(vars["directory"], def.Metrics), nil
	})
}

func registerResourceFactories(br *toolregistry.BuiltinRegistry) {
	br.Register("list_resource", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		cfg, err := resourceConfig(def)
		if err != nil {
			return nil, err
		}
		return &filesystem.ListResourceBuilder{Root: vars["directory"], Resources: cfg}, nil
	})
	br.Register("read_resource", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		cfg, err := resourceConfig(def)
		if err != nil {
			return nil, err
		}
		return &filesystem.ReadResourceBuilder{Root: vars["directory"], Resources: cfg}, nil
	})
}

func resourceConfig(def catalog.ToolDef) (filesystem.ResourceConfig, error) {
	var cfg filesystem.ResourceConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return filesystem.ResourceConfig{}, err
	}
	return cfg, nil
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
		// In an isolated (request-local) state each invoke_llm word gets its own
		// conversation, so a router word's tool call does not pollute the answer
		// word's history. The shared conversation stays the default so host agents
		// keep reset_history and undo coherence across their looped invoke_llm word.
		history := st.conversation
		if st.isolateConversations {
			history = llm.NewConversation(nil, "", llm.ChatOptions{})
		}
		return toollm.NewInvokeLLMBuilder(def, toollm.InvokeLLMFactoryDeps{
			History:    history,
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
		st.manifestState = cfg.ManifestState
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
			StateFunc: func() core.State {
				return st.manifestState
			},
			Verbose: st.verbose,
			Retry:   st.parseRetries,
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
		lifecycle.RegisterFactories(br, lifecycle.FactoryDeps{
			Checkpoint: st.checkpoint, Tracer: st.tracer, Shutdown: st.shutdown,
		})
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
		return &lifecycle.CheckpointHistoryBuilder{Config: cfg, Checkpoint: st.checkpointForOps()}, nil
	}
}

func checkpointRollbackFactory(st *agentState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var cfg catalog.CheckpointRollbackConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		reverter, _ := st.checkpointForOps().(core.CheckpointReverter)
		return &lifecycle.CheckpointRollbackBuilder{
			Config:     cfg,
			Checkpoint: reverter,
			Registry:   st.registry,
			RunID:      cfg.SelectedCheckpoint(),
			Tracer:     st.tracer,
		}, nil
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
			Directory:        st.directory,
			ChildAgentBinary: st.childAgentBinary,
			Tracer:           st.tracer,
			Ctx:              st.ctx,
		})
	}
}

func registerEvaluationFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		evaluation.RegisterEvalFactories(br, evaluation.EvalFactoryDeps{
			Ctx:              st.ctx,
			Registry:         st.registry,
			Stderr:           os.Stderr,
			SuitePath:        st.request,
			OutputDir:        st.output,
			ChildAgentBinary: st.childAgentBinary,
		})
	}
}

func registerBenchFactories() toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		bench.RegisterFactories(br, benchui.Assets())
	}
}

func registerRESTFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		toolrest.RegisterFactories(br, toolrest.FactoryDeps{
			Definitions:        st.restDefs,
			MachineRunner:      profileMachineRequestRunner(st),
			Monitor:            st.monitor,
			CredentialResolver: toolrest.EmptyCredentialResolver{},
		})
	}
}

func profileMachineRequestRunner(st *agentState) toolrest.MachineRequestRunner {
	return toolrest.NewProfileMachineRequestRunner(toolrest.ProfileMachineRequestRunnerDeps{
		BaseDir:   filepath.Dir(flagProfile),
		Directory: st.directory,
		Vars: map[string]string{
			"directory": st.directory,
			"request":   st.request,
		},
		RegisterBuiltins: func(br *toolregistry.BuiltinRegistry, selected map[string]bool, reg *core.Registry) {
			registerBuiltinFactories(br, requestLocalState(st, reg), selected)
		},
		ExecBuilder: execBuilder,
	})
}

// requestLocalState returns a per-request agentState for machine_request tool
// factories. It shares the host's immutable deps (tracer, verbose, ctx,
// directories) but binds tool construction to the request's own registry and a
// fresh conversation and parse-retry and manifest-state tracker, so
// parse_response and $tool resolve the tool vocabulary against the request
// registry and the request's invoke_llm words neither share history with the
// host agent nor leak state across requests.
func requestLocalState(host *agentState, reg *core.Registry) *agentState {
	local := *host
	local.registry = reg
	local.conversation = llm.NewConversation(nil, "", llm.ChatOptions{})
	local.isolateConversations = true
	local.manifestState = ""
	local.tracker = validation.NewToolTracker()
	maxConsecutive := 0
	if host.parseRetries != nil {
		maxConsecutive = host.parseRetries.MaxConsecutive
	}
	local.parseRetries = &toollm.ParseErrorRetryTracker{MaxConsecutive: maxConsecutive}
	return &local
}

func registerDocumentationFactories() toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		docsapi.RegisterFactories(br)
	}
}
