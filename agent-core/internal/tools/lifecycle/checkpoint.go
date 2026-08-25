// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

const (
	checkpointRollbackReceiptVersion  = 1
	checkpointRollbackReceiptStrategy = "operator_selected_rollback_recovery"
	maxCheckpointRollbackReceiptBytes = 4096
)

var checkpointRollbackReceiptRequirements = []string{
	"operator_decision",
	"resume_checkpoint_or_rollback_target",
}

type checkpointRollbackReceipt struct {
	Version            int      `json:"version"`
	Strategy           string   `json:"strategy"`
	Declaration        string   `json:"declaration"`
	Run                string   `json:"run"`
	TargetIteration    int      `json:"target_iteration"`
	TargetStep         int      `json:"target_step"`
	PriorBranch        string   `json:"prior_branch,omitempty"`
	PriorCheckpoint    string   `json:"prior_checkpoint,omitempty"`
	RollbackCheckpoint string   `json:"rollback_checkpoint,omitempty"`
	Requires           []string `json:"requires"`
}

// CheckpointHistoryBuilder constructs checkpoint_history commands.
type CheckpointHistoryBuilder struct {
	Config     catalog.CheckpointHistoryConfig
	Checkpoint core.Checkpoint
}

func (b *CheckpointHistoryBuilder) Build(_ core.Result) core.Command {
	return &checkpointHistoryCmd{config: b.Config, checkpoint: b.Checkpoint}
}

type checkpointHistoryCmd struct {
	config     catalog.CheckpointHistoryConfig
	checkpoint core.Checkpoint
}

func (c *checkpointHistoryCmd) Name() string { return "checkpoint_history" }

func (c *checkpointHistoryCmd) Execute() core.Result {
	if c.checkpoint == nil {
		return commandError(c.Name(), fmt.Errorf("checkpoint_history requires a Checkpoint"))
	}
	pos, exec, err := c.checkpoint.Load()
	if err != nil {
		return commandError(c.Name(), err)
	}
	// Structured output matching the declared checkpoint-history schema
	// {run, history}: run echoes the selected run (explicit id or "latest"),
	// history is the stable digest (srd026 R2.1, R2.6; #493).
	out, err := json.Marshal(map[string]any{
		"run":     c.config.SelectedCheckpoint(),
		"history": core.FormatExecutionHistory(pos, exec),
	})
	if err != nil {
		return commandError(c.Name(), fmt.Errorf("encode history output: %w", err))
	}
	return core.Result{Signal: core.ToolDone, CommandName: c.Name(), Output: string(out)}
}

func (c *checkpointHistoryCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

// CheckpointRollbackBuilder constructs checkpoint_rollback commands.
type CheckpointRollbackBuilder struct {
	ToolName   string
	Config     catalog.CheckpointRollbackConfig
	Checkpoint core.CheckpointReverter
	Registry   core.CommandResolver
	RunID      string
	Tracer     tracing.Tracer
}

func (b *CheckpointRollbackBuilder) Build(_ core.Result) core.Command {
	return &checkpointRollbackCmd{
		toolName: b.ToolName,
		config:   b.Config, checkpoint: b.Checkpoint, registry: b.Registry,
		runID: b.RunID, tracer: b.Tracer,
	}
}

// BuildReverser constructs an undo-only command from declaration identity alone.
// It validates the successful rollback's persisted receipt and reports operator
// recovery work; it cannot replay the original forward execution.
func (b *CheckpointRollbackBuilder) BuildReverser() core.Command {
	return &checkpointRollbackRecoveryCmd{toolName: b.ToolName}
}

type checkpointRollbackCmd struct {
	toolName   string
	config     catalog.CheckpointRollbackConfig
	checkpoint core.CheckpointReverter
	registry   core.CommandResolver
	runID      string
	tracer     tracing.Tracer
}

func (c *checkpointRollbackCmd) Name() string {
	return lifecycleToolName(c.toolName, "checkpoint_rollback")
}

var _ core.ContextCommand = (*checkpointRollbackCmd)(nil)

// Execute rolls the run back to the target iteration in two parts: (1) the
// CheckpointReverter reverts the persisted DB state git-style to the target
// step, then (2) the reverse receipt walk reverses external effects (files,
// resources) of the entries after the target by rebuilding each tool through
// core.Reverser and calling its receipt-driven Undo (srd036 R6; #44).
func (c *checkpointRollbackCmd) Execute() core.Result {
	return c.ExecuteContext(context.Background())
}

// ExecuteContext preserves the dispatch context across the receipt walk so
// context-aware Undo implementations can cancel in-flight compensation.
func (c *checkpointRollbackCmd) ExecuteContext(ctx context.Context) core.Result {
	if c.checkpoint == nil {
		return commandError(c.Name(), fmt.Errorf("checkpoint_rollback requires a revertible Checkpoint backend"))
	}
	if !c.config.HasTargetIteration() {
		return commandError(c.Name(), fmt.Errorf("checkpoint_rollback requires to_iteration"))
	}
	_, execution, err := c.checkpoint.Load()
	if err != nil {
		return commandError(c.Name(), err)
	}
	priorCheckpoint := rollbackCheckpointReference(c.checkpoint)
	report, err := rollbackViaReceipts(rollbackViaReceiptsOptions{
		Context:         ctx,
		Reverter:        c.checkpoint,
		Registry:        c.registry,
		Tracer:          c.tracer,
		RunID:           c.runID,
		Execution:       execution,
		TargetIteration: *c.config.ToIteration,
	})
	if err != nil {
		var partial *PartialRollbackError
		if errors.As(err, &partial) {
			// The DB Revert succeeded but external effects are only partly
			// reversed; report CommandError and keep the structured report plus
			// failure detail so an operator can choose retry, resume, or stop
			// (srd026 R3.7, R6.3).
			return core.Result{
				Signal:      core.CommandError,
				CommandName: c.Name(),
				Output:      rollbackOutput(report, partial),
				Err:         partial,
			}
		}
		return commandError(c.Name(), err)
	}
	return c.successfulRollbackResult(report, priorCheckpoint)
}

func (c *checkpointRollbackCmd) successfulRollbackResult(
	report rollbackReport,
	priorCheckpoint string,
) core.Result {
	receipt, err := encodeCheckpointRollbackReceipt(checkpointRollbackReceipt{
		Version:            checkpointRollbackReceiptVersion,
		Strategy:           checkpointRollbackReceiptStrategy,
		Declaration:        c.Name(),
		Run:                c.runID,
		TargetIteration:    *c.config.ToIteration,
		TargetStep:         report.TargetStep,
		PriorBranch:        rollbackPriorBranch(c.runID),
		PriorCheckpoint:    priorCheckpoint,
		RollbackCheckpoint: rollbackCheckpointReference(c.checkpoint),
		Requires:           append([]string(nil), checkpointRollbackReceiptRequirements...),
	})
	if err != nil {
		return commandError(c.Name(), fmt.Errorf("encode checkpoint rollback receipt: %w", err))
	}
	return core.Result{
		Signal: core.ToolDone, CommandName: c.Name(),
		Output: rollbackOutput(report, nil), Receipt: receipt,
	}
}

func (c *checkpointRollbackCmd) Undo(prior core.Result) core.Result {
	return (&checkpointRollbackRecoveryCmd{toolName: c.toolName}).Undo(prior)
}

type checkpointRollbackRecoveryCmd struct {
	toolName string
}

func (c *checkpointRollbackRecoveryCmd) Name() string {
	return lifecycleToolName(c.toolName, "checkpoint_rollback")
}

func (c *checkpointRollbackRecoveryCmd) Execute() core.Result {
	return commandError(c.Name(), fmt.Errorf("%s recovery command is undo-only", c.Name()))
}

func (c *checkpointRollbackRecoveryCmd) Undo(prior core.Result) core.Result {
	receipt, err := c.decodeReceipt(prior)
	if err != nil {
		return commandError(c.Name(), fmt.Errorf("decode checkpoint rollback receipt: %w", err))
	}
	data := map[string]interface{}{
		"run":              receipt.Run,
		"target_iteration": receipt.TargetIteration,
		"target_step":      receipt.TargetStep,
		"actions": []string{
			"resume_from_rollback_checkpoint",
			"select_another_rollback_target",
			"restore_prior_checkpoint",
		},
	}
	addRollbackRecoveryData(data, "prior_branch", receipt.PriorBranch)
	addRollbackRecoveryData(data, "prior_checkpoint", receipt.PriorCheckpoint)
	addRollbackRecoveryData(data, "rollback_checkpoint", receipt.RollbackCheckpoint)
	return undo.BoundaryCompensationResult(c.Name(), undo.BoundaryCompensation{
		Strategy: checkpointRollbackReceiptStrategy,
		Reason: fmt.Sprintf(
			"rollback of run %q to iteration %d (step %d) remains applied; "+
				"an operator must choose a checkpoint to resume or another rollback target",
			receipt.Run, receipt.TargetIteration, receipt.TargetStep,
		),
		Requires: append([]string(nil), receipt.Requires...),
		Data:     data,
	})
}

func (c *checkpointRollbackRecoveryCmd) decodeReceipt(
	prior core.Result,
) (checkpointRollbackReceipt, error) {
	if prior.CommandName != "" && prior.CommandName != c.Name() {
		return checkpointRollbackReceipt{}, fmt.Errorf(
			"command %q does not match declaration %q", prior.CommandName, c.Name(),
		)
	}
	receipt, err := decodeCheckpointRollbackReceipt(prior.Receipt)
	if err != nil {
		return checkpointRollbackReceipt{}, err
	}
	if receipt.Declaration != c.Name() {
		return checkpointRollbackReceipt{}, fmt.Errorf(
			"declaration %q does not match %q", receipt.Declaration, c.Name(),
		)
	}
	return receipt, nil
}

func encodeCheckpointRollbackReceipt(receipt checkpointRollbackReceipt) (string, error) {
	if err := validateCheckpointRollbackReceipt(receipt); err != nil {
		return "", err
	}
	value, err := json.Marshal(receipt)
	if err != nil {
		return "", err
	}
	if len(value) > maxCheckpointRollbackReceiptBytes {
		return "", fmt.Errorf("receipt exceeds %d bytes", maxCheckpointRollbackReceiptBytes)
	}
	return string(value), nil
}

func decodeCheckpointRollbackReceipt(value string) (checkpointRollbackReceipt, error) {
	if value == "" {
		return checkpointRollbackReceipt{}, fmt.Errorf("receipt is required")
	}
	if len(value) > maxCheckpointRollbackReceiptBytes {
		return checkpointRollbackReceipt{}, fmt.Errorf(
			"receipt exceeds %d bytes", maxCheckpointRollbackReceiptBytes,
		)
	}
	var receipt checkpointRollbackReceipt
	decoder := json.NewDecoder(bytes.NewReader([]byte(value)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return checkpointRollbackReceipt{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return checkpointRollbackReceipt{}, fmt.Errorf("multiple JSON values")
		}
		return checkpointRollbackReceipt{}, err
	}
	if err := validateCheckpointRollbackReceipt(receipt); err != nil {
		return checkpointRollbackReceipt{}, err
	}
	return receipt, nil
}

func validateCheckpointRollbackReceipt(receipt checkpointRollbackReceipt) error {
	if receipt.Version != checkpointRollbackReceiptVersion {
		return fmt.Errorf("unsupported receipt version %d", receipt.Version)
	}
	if receipt.Strategy != checkpointRollbackReceiptStrategy {
		return fmt.Errorf(
			"strategy %q does not match %q",
			receipt.Strategy, checkpointRollbackReceiptStrategy,
		)
	}
	if err := validateCheckpointRollbackReceiptFields(receipt); err != nil {
		return err
	}
	if receipt.TargetIteration < 0 {
		return fmt.Errorf("target_iteration must be non-negative")
	}
	if receipt.TargetStep < 0 {
		return fmt.Errorf("target_step must be non-negative")
	}
	return validateCheckpointRollbackReceiptRequirements(receipt.Requires)
}

func validateCheckpointRollbackReceiptFields(receipt checkpointRollbackReceipt) error {
	for name, field := range map[string]string{
		"strategy": receipt.Strategy, "declaration": receipt.Declaration, "run": receipt.Run,
	} {
		if !canonicalRollbackReceiptField(field) {
			return fmt.Errorf("%s is required and must be canonical", name)
		}
	}
	for name, field := range map[string]string{
		"prior_branch":        receipt.PriorBranch,
		"prior_checkpoint":    receipt.PriorCheckpoint,
		"rollback_checkpoint": receipt.RollbackCheckpoint,
	} {
		if field != "" && !canonicalRollbackReceiptField(field) {
			return fmt.Errorf("%s must be canonical when present", name)
		}
	}
	return nil
}

func validateCheckpointRollbackReceiptRequirements(requires []string) error {
	if len(requires) != len(checkpointRollbackReceiptRequirements) {
		return fmt.Errorf("requires must name the operator recovery decision")
	}
	for i, requirement := range checkpointRollbackReceiptRequirements {
		if requires[i] != requirement {
			return fmt.Errorf("requires must name the operator recovery decision")
		}
	}
	return nil
}

func canonicalRollbackReceiptField(value string) bool {
	if value == "" || len(value) > maxCheckpointRollbackReceiptBytes ||
		!utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func rollbackPriorBranch(runID string) string {
	if runID == "" || runID == "latest" {
		return ""
	}
	return runID
}

func rollbackCheckpointReference(checkpoint core.Checkpoint) string {
	if provider, ok := checkpoint.(core.ConversationReferenceProvider); ok {
		if reference, available := provider.ConversationReference(); available {
			return reference
		}
	}
	if provider, ok := checkpoint.(core.DomainReferenceProvider); ok {
		if reference, available := provider.DomainReference(); available {
			return reference
		}
	}
	return ""
}

func addRollbackRecoveryData(data map[string]interface{}, key, value string) {
	if value != "" {
		data[key] = value
	}
}

func commandError(commandName string, err error) core.Result {
	return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
}

func checkpointHistoryFactory(deps FactoryDeps) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg catalog.CheckpointHistoryConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		return &CheckpointHistoryBuilder{Config: cfg, Checkpoint: deps.opsCheckpoint()}, nil
	}
}

func checkpointRollbackFactory(deps FactoryDeps) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg catalog.CheckpointRollbackConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		reverter, _ := deps.opsCheckpoint().(core.CheckpointReverter)
		return &CheckpointRollbackBuilder{
			ToolName:   def.Name,
			Config:     cfg,
			Checkpoint: reverter,
			Registry:   deps.Registry,
			RunID:      cfg.SelectedCheckpoint(),
			Tracer:     deps.Tracer,
		}, nil
	}
}
