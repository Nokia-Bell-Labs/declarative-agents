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
	Checkpoint core.Checkpoint
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
		return &SuspendBuilder{Config: cfg, Checkpoint: deps.Checkpoint, Tracer: deps.Tracer}, nil
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
	checkpoint core.Checkpoint
	tracer     tracing.Tracer
}

// checkpointConfigured reports whether a persistent checkpoint backend is wired.
// The loop always resolves a Checkpoint port, defaulting to NoopCheckpoint when
// persistence is disabled, so require_checkpoint gates on a non-nil, non-noop
// backend (srd018 R5, srd035-checkpoint-port R5.1).
func (s *suspendCmd) checkpointConfigured() bool {
	if s.checkpoint == nil {
		return false
	}
	_, noop := s.checkpoint.(core.NoopCheckpoint)
	return !noop
}

func (s *suspendCmd) Name() string { return "suspend" }
func (s *suspendCmd) Undo(_ core.Result) core.Result {
	return undo.BoundaryCompensationUndo(s.Name(), "resume with an explicit approval/rejection signal or roll back to an earlier checkpoint")
}

func (s *suspendCmd) Execute() core.Result {
	if s.config.RequireCheckpoint && !s.checkpointConfigured() {
		err := fmt.Errorf("suspend requires a persistent checkpoint backend for checkpoint persistence")
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
			attribute.Bool("checkpoint_configured", s.checkpointConfigured()),
		)
	}
	return core.Result{Signal: core.AwaitApproval, CommandName: s.Name(), Output: reason}
}

// SuspendBuilder constructs suspend commands.
type SuspendBuilder struct {
	Config     SuspendConfig
	Checkpoint core.Checkpoint
	Tracer     tracing.Tracer
}

func (b *SuspendBuilder) Build(_ core.Result) core.Command {
	return &suspendCmd{config: b.Config, checkpoint: b.Checkpoint, tracer: b.Tracer}
}
