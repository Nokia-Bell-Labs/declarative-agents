// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/evaluation"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/pipeline"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/compose"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/control"
	tooldolt "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/dolt"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/filesystem"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/lifecycle"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
	toolotlp "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/otlp"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/service"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/validation"
)

func registerBuiltinFactories(br *toolregistry.BuiltinRegistry, st *agentState, selected map[string]bool) {
	st.validationEnabled = validationFactoriesSelected(selected)
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
		RegisterControl:        registerControlFactories(st),
		RegisterPlanning:       registerPlanningFactories(st),
		RegisterEvaluation:     registerEvaluationFactories(st),
		RegisterSpecValidation: registerSpecValidationFactories(st),
		RegisterREST:           registerRESTFactories(st),
		RegisterDolt:           registerDoltFactories(st),
		RegisterCompose:        registerComposeFactories(),
		RegisterOTLP:           registerOTLPFactories(),
		RegisterService:        registerServiceFactories(st),
	}
}

func registerDoltFactories(st *agentState) toolregistry.FactoryRegistrar {
	checkpoint, checkpointErr := doltCheckpointIdentity(st.doltDSN)
	deps := tooldolt.FactoryDeps{
		Connections:        environmentDoltConnections{},
		CheckpointIdentity: checkpoint,
	}
	return func(br *toolregistry.BuiltinRegistry) {
		register := func(init string, factory toolregistry.BuiltinFactory) {
			br.Register(init, func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
				if checkpointErr != nil {
					return nil, checkpointErr
				}
				return factory(def, vars)
			})
		}
		register(tooldolt.InitProvision, tooldolt.ProvisionFactory(deps))
		register(tooldolt.InitQuery, tooldolt.QueryFactory(deps))
		register(tooldolt.InitWrite, tooldolt.WriteFactory(deps))
	}
}

type environmentDoltConnections struct{}

func (environmentDoltConnections) ResolveConnection(
	_ context.Context, ref string, _ map[string]string,
) (string, error) {
	//nolint:forbidigo // Trusted ToolDef config names the environment reference; DSN authority remains at cmd/agent.
	value, ok := os.LookupEnv(ref)
	if !ok || value == "" {
		return "", fmt.Errorf("configured Dolt connection reference %q is unavailable", ref)
	}
	return value, nil
}

func registerComposeFactories() toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		br.Register("compose", func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
			var cfg catalog.ComposeConfig
			if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
				return nil, err
			}
			if err := compose.ValidateConfig(def.Name, cfg.Inputs); err != nil {
				return nil, err
			}
			return compose.Builder{
				ToolName: def.Name,
				Template: cfg.Template,
				Inputs:   cfg.Inputs,
				Signal:   core.Signal(cfg.Signal),
			}, nil
		})
		br.Register("render_each", func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
			var cfg catalog.RenderEachConfig
			if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
				return nil, err
			}
			if err := compose.ValidateRenderEachConfig(def.Name, cfg.Items, cfg.ItemTemplate, cfg.Signal); err != nil {
				return nil, err
			}
			return compose.RenderEachBuilder{
				ToolName: def.Name, Items: cfg.Items, ItemTemplate: cfg.ItemTemplate,
				Separator: cfg.Separator, Signal: core.Signal(cfg.Signal),
			}, nil
		})
	}
}

func registerOTLPFactories() toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		toolotlp.RegisterFactories(br, toolotlp.NewState())
	}
}

func registerFilesystemFactories() toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		fileFactories := []struct {
			init    string
			builder func(string, core.MetricConfig, string) core.Builder
		}{
			{"file_read", func(root string, metrics core.MetricConfig, _ string) core.Builder {
				return &filesystem.ReadBuilder{Root: root, Metrics: metrics}
			}},
			{"file_write", func(root string, metrics core.MetricConfig, strategy string) core.Builder {
				return &filesystem.WriteBuilder{Root: root, UndoStrategy: strategy, Metrics: metrics}
			}},
			{"file_edit", func(root string, metrics core.MetricConfig, strategy string) core.Builder {
				return &filesystem.EditBuilder{Root: root, UndoStrategy: strategy, Metrics: metrics}
			}},
		}
		for _, entry := range fileFactories {
			registerFileFactory(br, entry.init, entry.builder)
		}
		br.Register("file_find", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
			return &filesystem.FindBuilder{Root: vars["directory"], OutputLineCap: def.OutputCap}, nil
		})
		registerResourceFactories(br)
	}
}

func registerFileFactory(br *toolregistry.BuiltinRegistry, init string, builder func(string, core.MetricConfig, string) core.Builder) {
	br.Register(init, func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return builder(vars["directory"], def.Metrics, def.Undo.Strategy), nil
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
			ToolName:   def.Name,
			Config:     cfg,
			Checkpoint: reverter,
			Registry:   st.registry,
			RunID:      cfg.SelectedCheckpoint(),
			Tracer:     st.tracer,
		}, nil
	}
}

func registerControlFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		br.Register("self_invoke", selfInvokeFactory(st))
		br.Register("value_predicate", valuePredicateFactory())
		br.Register("partition", partitionFactory())
		br.Register("select_subset", selectSubsetFactory())
	}
}

func partitionFactory() toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg catalog.PartitionConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if err := control.ValidatePartitionConfig(
			def.Name, cfg.Items, cfg.Field, cfg.Op, cfg.Right, cfg.OperandType, cfg.Satisfied,
		); err != nil {
			return nil, err
		}
		return control.PartitionBuilder{
			ToolName: def.Name, Items: cfg.Items, Field: cfg.Field, Op: cfg.Op,
			Right: cfg.Right, OperandType: cfg.OperandType, Satisfied: core.Signal(cfg.Satisfied),
		}, nil
	}
}

func selectSubsetFactory() toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg catalog.SelectSubsetConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if err := control.ValidateSelectSubsetConfig(
			def.Name, cfg.Candidates, cfg.Vocabulary, cfg.MatchField,
			cfg.AllMatched, cfg.Partial, cfg.Empty,
		); err != nil {
			return nil, err
		}
		return control.SelectSubsetBuilder{
			ToolName: def.Name, Candidates: cfg.Candidates, Vocabulary: cfg.Vocabulary,
			MatchField: cfg.MatchField, AllMatched: core.Signal(cfg.AllMatched),
			Partial: core.Signal(cfg.Partial), Empty: core.Signal(cfg.Empty),
		}, nil
	}
}

// valuePredicateFactory builds the value predicate word (srd041). Config is
// validated here rather than at dispatch, so an unknown operator or a missing
// signal name fails registration before a run reaches the branch it names.
func valuePredicateFactory() toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg catalog.ValuePredicateConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if err := control.ValidateValuePredicateConfig(
			def.Name, cfg.Left, cfg.Op, cfg.Right, cfg.OperandType, cfg.Satisfied, cfg.Unsatisfied,
		); err != nil {
			return nil, err
		}
		return control.ValuePredicateBuilder{
			ToolName:    def.Name,
			Left:        cfg.Left,
			Op:          cfg.Op,
			Right:       cfg.Right,
			OperandType: cfg.OperandType,
			Satisfied:   core.Signal(cfg.Satisfied),
			Unsatisfied: core.Signal(cfg.Unsatisfied),
		}, nil
	}
}

// registerServiceFactories registers the rig's service words. One service
// state and one scenario session are shared across the family, so every child
// a run starts stays reachable for teardown.
func registerServiceFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		state := service.NewStateWithContext(st.ctx)
		st.reapServices = func() { state.Reap() }
		service.RegisterBuiltins(br, service.FactoryDeps{
			State:    state,
			Session:  service.NewScenarioSession(state),
			CoreRoot: st.coreRoot,
		})
	}
}

func selfInvokeFactory(st *agentState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		parsed, err := decodeChildAgent(def)
		if err != nil {
			return nil, err
		}
		config := childExecuteConfig(parsed, st.coreRoot)
		config.Binary = st.childAgentBinary
		return &control.SelfInvokeBuilder{
			ToolName:      def.Name,
			Config:        config,
			RequestFrom:   parsed.RequestFrom,
			OutputFrom:    parsed.OutputFrom,
			WorkspacePath: vars["directory"],
			ExtraArgs:     directoryArgs(vars["directory"]),
			Ctx:           st.ctx,
			Tracer:        st.tracer,
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

func directoryArgs(directory string) []string {
	if directory == "" {
		return nil
	}
	return []string{"--directory", directory}
}

func registerSpecValidationFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		state := st.validation
		if state == nil {
			state = &validation.SpecState{
				Directory:       st.directory,
				TargetDirectory: st.directory,
			}
			if st.validationEnabled {
				st.validation = state
			}
		}
		provider, resolver := validationReferencePorts(st)
		validation.RegisterSpecFactories(br, validation.FactoryDeps{
			Directory: st.directory, State: state,
			ReferenceProvider: provider, SnapshotResolver: resolver,
		})
	}
}

func validationFactoriesSelected(selected map[string]bool) bool {
	for _, name := range []string{
		"load_corpus",
		"load_test_claims",
		"validate_specs",
		"reduce_consistency_checks",
		"reduce_ref_checks",
		"reduce_grep_checks",
		"resolve_test_evidence",
		"reduce_test_evidence_run",
		"format_report",
	} {
		if selected[name] {
			return true
		}
	}
	return false
}

func registerPlanningFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		pipeline.RegisterFactories(br, pipeline.FactoryDeps{
			Directory:    st.directory,
			Tracer:       st.tracer,
			Ctx:          st.ctx,
			ParseRetries: st.parseRetries,
		})
	}
}

func registerEvaluationFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		evaluation.RegisterEvalFactories(br, evaluation.EvalFactoryDeps{
			Ctx:              st.ctx,
			Registry:         st.registry,
			Stderr:           os.Stderr,
			OutputDir:        st.output,
			Directory:        st.directory,
			Tracer:           st.tracer,
			ChildAgentBinary: st.childAgentBinary,
			CoreRoot:         st.coreRoot,
		})
	}
}

func registerRESTFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		toolrest.RegisterFactories(br, toolrest.FactoryDeps{
			Definitions:        st.restDefs,
			MachineRunner:      profileMachineRequestRunner(st),
			Monitor:            st.monitor,
			RunID:              st.runID,
			CredentialResolver: toolrest.EnvironmentCredentials{},
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
// factories. It shares the host's immutable deps (tracer, capture level, ctx,
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
	local.validation = nil
	maxConsecutive := 0
	if host.parseRetries != nil {
		maxConsecutive = host.parseRetries.MaxConsecutive
	}
	local.parseRetries = &toollm.ParseErrorRetryTracker{MaxConsecutive: maxConsecutive}
	return &local
}
