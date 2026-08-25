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
	toolregistry.RegisterStandardBuiltinFactories(br, selected, standardFactoryDeps(st))
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

func registerLLMFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		provider, resolver := llmConversationReferencePorts(st)
		toollm.RegisterFactories(br, toollm.FactoryDeps{
			Ctx:                  st.ctx,
			Tracer:               st.tracer,
			Registry:             st.registry,
			Conversation:         st.conversation,
			IsolateConversations: st.isolateConversations,
			CaptureLevel:         st.captureLevel,
			ParseRetries:         st.parseRetries,
			ConversationRefs: toollm.ReferencePorts{
				Provider: provider, Resolver: resolver,
			},
			Resolved: st.ensureResolved(),
		})
	}
}

func registerLifecycleFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		lifecycle.RegisterFactories(br, lifecycle.FactoryDeps{
			Checkpoint:    st.checkpoint,
			OpsCheckpoint: st.checkpointForOps(),
			Tracer:        st.tracer,
			Shutdown:      st.shutdown,
			Registry:      st.registry,
		})
	}
}

func bindServiceState(st *agentState) {
	state := service.NewStateWithContext(st.ctx)
	st.services = state
	st.reapServices = func() { state.Reap() }
}

// registerServiceFactories registers the rig's service words against the
// service state allocated on agentState, so every child a run starts stays
// reachable for teardown.
func registerServiceFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		service.RegisterBuiltins(br, service.FactoryDeps{
			State: st.services, CoreRoot: st.coreRoot,
		})
	}
}

func registerSpecValidationFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		provider, resolver := validationReferencePorts(st)
		validation.RegisterSpecFactories(br, validation.FactoryDeps{
			Directory: st.directory, State: st.validation,
			ReferenceProvider: provider, SnapshotResolver: resolver,
		})
	}
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
// fresh conversation, parse-retry tracker, ResolvedModel, and service State, so
// parse_response and $tool resolve the tool vocabulary against the request
// registry and the request's invoke_llm words neither share history with the
// host agent nor leak parser, model, or child-process state across requests.
func requestLocalState(host *agentState, reg *core.Registry) *agentState {
	local := *host
	local.registry = reg
	local.conversation = llm.NewConversation(nil, "", llm.ChatOptions{})
	local.isolateConversations = true
	local.resolved = &toollm.ResolvedModel{}
	local.validation = &validation.SpecState{Directory: host.directory, TargetDirectory: host.directory}
	maxConsecutive := 0
	if host.parseRetries != nil {
		maxConsecutive = host.parseRetries.MaxConsecutive
	}
	local.parseRetries = &toollm.ParseErrorRetryTracker{MaxConsecutive: maxConsecutive}
	bindServiceState(&local)
	return &local
}
