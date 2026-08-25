// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/evaluation"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/pipeline"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/checkpoint"
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
		RegisterFilesystem: filesystem.RegisterFactories,
		RegisterLLM:        registerLLMFactories(st),
		RegisterLifecycle:  registerLifecycleFactories(st),
		RegisterControl: func(br *toolregistry.BuiltinRegistry) {
			control.RegisterFactories(br, control.FactoryDeps{
				Ctx: st.ctx, Tracer: st.tracer, CoreRoot: st.coreRoot, ChildAgentBinary: st.childAgentBinary,
			})
		},
		RegisterPlanning:       registerPlanningFactories(st),
		RegisterEvaluation:     registerEvaluationFactories(st),
		RegisterSpecValidation: registerSpecValidationFactories(st),
		RegisterREST:           registerRESTFactories(st),
		RegisterDolt: func(br *toolregistry.BuiltinRegistry) {
			identity, identityErr := checkpoint.Config{DoltDSN: st.doltDSN}.DatabaseIdentity()
			tooldolt.RegisterFactories(br, tooldolt.FactoryDeps{
				Connections:           tooldolt.EnvironmentConnections{},
				CheckpointIdentity:    identity,
				CheckpointIdentityErr: identityErr,
			})
		},
		RegisterCompose: compose.RegisterFactories,
		RegisterOTLP:    registerOTLPFactories(),
		RegisterService: registerServiceFactories(st),
	}
}

func registerOTLPFactories() toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		toolotlp.RegisterFactories(br, toolotlp.NewState())
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
			ToolName:   def.Name,
			Config:     cfg,
			Checkpoint: reverter,
			Registry:   st.registry,
			RunID:      cfg.SelectedCheckpoint(),
			Tracer:     st.tracer,
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
			SignalSourceRunner: st.signalSourceRunner,
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
