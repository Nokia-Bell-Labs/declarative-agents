// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package lifecycle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

const (
	InitDelay              = "delay"
	InitSuspend            = "suspend"
	InitExitAgent          = "exit_agent"
	InitCheckpointHistory  = "checkpoint_history"
	InitCheckpointRollback = "checkpoint_rollback"

	suspendReceiptVersion  = 1
	suspendReceiptStrategy = "resume_reject_or_rollback"
	// The loop assigns the durable checkpoint identity after Execute returns.
	// The receipt therefore names its containing execution checkpoint as the
	// authoritative reference context instead of inventing a premature ID.
	suspendCheckpointReferenceContext = "execution_checkpoint"
	maxSuspendReceiptBytes            = 4096
)

// SuspendConfig configures the suspend builtin.
type SuspendConfig struct {
	Label             string `json:"label"`
	Reason            string `json:"reason"`
	RequireCheckpoint bool   `json:"require_checkpoint"`
}

type suspendReceipt struct {
	Version                    int    `json:"version"`
	Strategy                   string `json:"strategy"`
	Declaration                string `json:"declaration"`
	Label                      string `json:"label"`
	Reason                     string `json:"reason"`
	CheckpointRequired         bool   `json:"checkpoint_required"`
	CheckpointConfigured       bool   `json:"checkpoint_configured"`
	CheckpointReferenceContext string `json:"checkpoint_reference_context"`
}

// FactoryDeps holds shared dependencies for lifecycle builtins.
type FactoryDeps struct {
	Checkpoint    core.Checkpoint
	OpsCheckpoint core.Checkpoint
	Tracer        tracing.Tracer
	Shutdown      func()
	Registry      *core.Registry
}

func (d FactoryDeps) opsCheckpoint() core.Checkpoint {
	if d.OpsCheckpoint != nil {
		return d.OpsCheckpoint
	}
	return d.Checkpoint
}

// RegisterFactories registers lifecycle builtin factories.
func RegisterFactories(br *toolregistry.BuiltinRegistry, deps FactoryDeps) {
	br.Register(InitDelay, func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg DelayConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		return newDelayBuilder(def.Name, cfg)
	})
	br.Register(InitSuspend, func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var cfg SuspendConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		return &SuspendBuilder{
			ToolName: def.Name, Config: cfg,
			Checkpoint: deps.Checkpoint, Tracer: deps.Tracer,
		}, nil
	})
	br.Register(InitExitAgent, func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var cfg ExitConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		return ExitBuilder{
			ToolName: def.Name, Config: cfg,
			Shutdown: deps.Shutdown, Tracer: deps.Tracer,
		}, nil
	})
	br.Register(InitCheckpointHistory, checkpointHistoryFactory(deps))
	br.Register(InitCheckpointRollback, checkpointRollbackFactory(deps))
}

type suspendCmd struct {
	toolName   string
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

func (s *suspendCmd) Name() string { return lifecycleToolName(s.toolName, "suspend") }

func (s *suspendCmd) Undo(prior core.Result) core.Result {
	receipt, err := s.decodeReceipt(prior)
	if err != nil {
		return commandError(s.Name(), fmt.Errorf("decode suspend receipt: %w", err))
	}
	return undo.BoundaryCompensationResult(s.Name(), undo.BoundaryCompensation{
		Strategy: suspendReceiptStrategy,
		Reason: fmt.Sprintf(
			"suspend %q remains pending: resume with Approved, reject with Rejected, or roll back to an earlier checkpoint",
			receipt.Label,
		),
		Requires: []string{"operator_decision", "resume_signal_or_rollback_target"},
		Data: map[string]interface{}{
			"actions":                      []string{"resume", "reject", "rollback"},
			"label":                        receipt.Label,
			"reason":                       receipt.Reason,
			"checkpoint_required":          receipt.CheckpointRequired,
			"checkpoint_configured":        receipt.CheckpointConfigured,
			"checkpoint_reference_context": receipt.CheckpointReferenceContext,
		},
	})
}

func (s *suspendCmd) Execute() core.Result {
	if s.config.RequireCheckpoint && !s.checkpointConfigured() {
		err := fmt.Errorf("suspend requires a persistent checkpoint backend for checkpoint persistence")
		return core.Result{Signal: core.CommandError, CommandName: s.Name(), Err: err, Output: err.Error()}
	}
	label := resolvedSuspendLabel(s.config.Label)
	reason := resolvedSuspendReason(s.config.Reason)
	receipt, err := s.encodeReceipt(label, reason)
	if err != nil {
		return commandError(s.Name(), fmt.Errorf("encode suspend receipt: %w", err))
	}
	if s.tracer != nil {
		s.tracer.Event("suspend.requested",
			attribute.String("label", label),
			attribute.String("reason", reason),
			attribute.Bool("require_checkpoint", s.config.RequireCheckpoint),
			attribute.Bool("checkpoint_configured", s.checkpointConfigured()),
		)
	}
	return core.Result{
		Signal: core.AwaitApproval, CommandName: s.Name(),
		Output: reason, Receipt: receipt,
	}
}

// SuspendBuilder constructs suspend commands.
type SuspendBuilder struct {
	ToolName   string
	Config     SuspendConfig
	Checkpoint core.Checkpoint
	Tracer     tracing.Tracer
}

func (b *SuspendBuilder) Build(_ core.Result) core.Command {
	return &suspendCmd{
		toolName: b.ToolName, config: b.Config,
		checkpoint: b.Checkpoint, tracer: b.Tracer,
	}
}

// BuildReverser constructs a fresh receipt-only suspend command. A rollback
// does not need the original config or checkpoint dependency because the
// successful suspension captured its complete operator decision context.
func (b *SuspendBuilder) BuildReverser() core.Command {
	return &suspendCmd{toolName: b.ToolName}
}

func (s *suspendCmd) encodeReceipt(label, reason string) (string, error) {
	value := suspendReceipt{
		Version:                    suspendReceiptVersion,
		Strategy:                   suspendReceiptStrategy,
		Declaration:                s.Name(),
		Label:                      label,
		Reason:                     reason,
		CheckpointRequired:         s.config.RequireCheckpoint,
		CheckpointConfigured:       s.checkpointConfigured(),
		CheckpointReferenceContext: suspendCheckpointReferenceContext,
	}
	if err := validateSuspendReceipt(value); err != nil {
		return "", err
	}
	receipt, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if len(receipt) > maxSuspendReceiptBytes {
		return "", fmt.Errorf("receipt exceeds %d bytes", maxSuspendReceiptBytes)
	}
	return string(receipt), nil
}

func (s *suspendCmd) decodeReceipt(prior core.Result) (suspendReceipt, error) {
	if prior.CommandName != "" && prior.CommandName != s.Name() {
		return suspendReceipt{}, fmt.Errorf(
			"command %q does not match declaration %q", prior.CommandName, s.Name(),
		)
	}
	receipt, err := decodeSuspendReceipt(prior.Receipt)
	if err != nil {
		return suspendReceipt{}, err
	}
	if receipt.Declaration != s.Name() {
		return suspendReceipt{}, fmt.Errorf(
			"declaration %q does not match %q", receipt.Declaration, s.Name(),
		)
	}
	return receipt, nil
}

func decodeSuspendReceipt(value string) (suspendReceipt, error) {
	if value == "" {
		return suspendReceipt{}, fmt.Errorf("receipt is required")
	}
	if len(value) > maxSuspendReceiptBytes {
		return suspendReceipt{}, fmt.Errorf("receipt exceeds %d bytes", maxSuspendReceiptBytes)
	}
	var receipt suspendReceipt
	decoder := json.NewDecoder(bytes.NewReader([]byte(value)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return suspendReceipt{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return suspendReceipt{}, fmt.Errorf("multiple JSON values")
		}
		return suspendReceipt{}, err
	}
	if err := validateSuspendReceipt(receipt); err != nil {
		return suspendReceipt{}, err
	}
	return receipt, nil
}

func validateSuspendReceipt(receipt suspendReceipt) error {
	if receipt.Version != suspendReceiptVersion {
		return fmt.Errorf("unsupported receipt version %d", receipt.Version)
	}
	for name, field := range map[string]string{
		"strategy": receipt.Strategy, "declaration": receipt.Declaration,
		"label": receipt.Label, "reason": receipt.Reason,
		"checkpoint_reference_context": receipt.CheckpointReferenceContext,
	} {
		if field == "" || strings.TrimSpace(field) != field {
			return fmt.Errorf("%s is required and must be canonical", name)
		}
	}
	if receipt.Strategy != suspendReceiptStrategy {
		return fmt.Errorf(
			"strategy %q does not match %q", receipt.Strategy, suspendReceiptStrategy,
		)
	}
	if receipt.CheckpointReferenceContext != suspendCheckpointReferenceContext {
		return fmt.Errorf(
			"checkpoint reference context %q does not match %q",
			receipt.CheckpointReferenceContext, suspendCheckpointReferenceContext,
		)
	}
	if receipt.CheckpointRequired && !receipt.CheckpointConfigured {
		return fmt.Errorf("required checkpoint was not configured")
	}
	return nil
}

func resolvedSuspendLabel(label string) string {
	if label == "" {
		return "approval"
	}
	return label
}

func resolvedSuspendReason(reason string) string {
	if reason == "" {
		return "awaiting approval"
	}
	return reason
}

func lifecycleToolName(name, fallback string) string {
	if name == "" {
		return fallback
	}
	return name
}
