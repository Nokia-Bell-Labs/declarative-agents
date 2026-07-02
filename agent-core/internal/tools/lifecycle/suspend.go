// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

// SuspendConfig configures the suspend builtin.
type SuspendConfig struct {
	Label             string `json:"label"`
	Reason            string `json:"reason"`
	RequireCheckpoint bool   `json:"require_checkpoint"`
}

// FactoryDeps holds shared dependencies for lifecycle builtins.
type FactoryDeps struct {
	StateStore core.StateStore
	Workspace  core.Workspace
	Tracer     tracing.Tracer
	Shutdown   func()
}

// RegisterFactories registers lifecycle builtin factories.
func RegisterFactories(br *toolregistry.BuiltinRegistry, deps FactoryDeps) {
	br.Register("suspend", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var cfg SuspendConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		return &SuspendBuilder{Config: cfg, StateStore: deps.StateStore, Workspace: deps.Workspace, Tracer: deps.Tracer}, nil
	})
	br.Register("exit_agent", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var cfg ExitConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		return ExitBuilder{Config: cfg, Shutdown: deps.Shutdown, Tracer: deps.Tracer}, nil
	})
}

type suspendCmd struct {
	config     SuspendConfig
	stateStore core.StateStore
	workspace  core.Workspace
	tracer     tracing.Tracer
}

func (s *suspendCmd) Name() string { return "suspend" }
func (s *suspendCmd) Undo() core.Result {
	return undo.BoundaryCompensationUndo(s.Name(), "resume with an explicit approval/rejection signal or roll back to an earlier checkpoint")
}
func (s *suspendCmd) UndoMemento() (core.UndoMemento, error) {
	payload := undo.BoundaryCompensationPayload{BoundaryCompensation: undo.BoundaryCompensation{
		Strategy:           "resume_or_checkpoint_rollback",
		Reason:             s.config.Reason,
		Requires:           []string{"approval_decision", "checkpoint_id"},
		CheckpointRequired: s.config.RequireCheckpoint,
	}}
	return undo.BoundaryCompensationMemento(s.Name(), payload, "resume with an explicit approval/rejection signal or roll back to an earlier checkpoint")
}

func (s *suspendCmd) Execute() core.Result {
	if s.config.RequireCheckpoint && s.stateStore == nil {
		err := fmt.Errorf("suspend requires StateStore for checkpoint persistence")
		return core.Result{Signal: core.CommandError, CommandName: s.Name(), Err: err, Output: err.Error()}
	}
	reason := s.config.Reason
	if reason == "" {
		reason = "awaiting approval"
	}
	if s.tracer != nil {
		s.tracer.Event("suspend.requested",
			attribute.String("label", s.config.Label),
			attribute.String("reason", reason),
			attribute.Bool("require_checkpoint", s.config.RequireCheckpoint),
			attribute.Bool("state_store_configured", s.stateStore != nil),
			attribute.Bool("workspace_configured", s.workspace != nil),
		)
	}
	return core.Result{Signal: core.AwaitApproval, CommandName: s.Name(), Output: reason}
}

// SuspendBuilder constructs suspend commands.
type SuspendBuilder struct {
	Config     SuspendConfig
	StateStore core.StateStore
	Workspace  core.Workspace
	Tracer     tracing.Tracer
}

func (b *SuspendBuilder) Build(_ core.Result) core.Command {
	return &suspendCmd{config: b.Config, stateStore: b.StateStore, workspace: b.Workspace, tracer: b.Tracer}
}
