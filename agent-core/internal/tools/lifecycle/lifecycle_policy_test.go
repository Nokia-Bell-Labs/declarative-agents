// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

func TestSuspendAliasEmitsStrictReceiptAndFreshReverserReportsPendingDecision(t *testing.T) {
	t.Parallel()
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{Checkpoint: &core.InMemoryCheckpoint{}})
	factory, ok := br.Resolve("suspend")
	require.True(t, ok)

	const alias = "await_release_approval"
	builder, err := factory(catalog.ToolDef{
		Name: alias, Type: "builtin", Init: "suspend",
		Config: map[string]interface{}{
			"label": "release", "reason": "approve release", "require_checkpoint": true,
		},
	}, nil)
	require.NoError(t, err)

	executed := builder.Build(core.Result{}).Execute()
	require.Equal(t, core.AwaitApproval, executed.Signal)
	require.Equal(t, alias, executed.CommandName)
	require.NotEmpty(t, executed.Receipt)

	receipt, err := decodeSuspendReceipt(executed.Receipt)
	require.NoError(t, err)
	require.Equal(t, alias, receipt.Declaration)
	require.Equal(t, "release", receipt.Label)
	require.Equal(t, "approve release", receipt.Reason)
	require.True(t, receipt.CheckpointRequired)
	require.True(t, receipt.CheckpointConfigured)
	require.Equal(t, suspendCheckpointReferenceContext, receipt.CheckpointReferenceContext)

	reverser, ok := builder.(core.Reverser)
	require.True(t, ok)
	fresh := reverser.BuildReverser()
	require.Equal(t, alias, fresh.Name())
	compensation := fresh.Undo(executed)
	require.Equal(t, core.CompensationRequired, compensation.Signal)
	require.Equal(t, alias, compensation.CommandName)

	pending, ok, err := undo.DecodeBoundaryReceipt(compensation.Output)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, suspendReceiptStrategy, pending.Strategy)
	require.Contains(t, pending.Reason, "resume with Approved")
	require.Contains(t, pending.Reason, "reject with Rejected")
	require.Contains(t, pending.Reason, "roll back")
	require.Equal(t, []string{"operator_decision", "resume_signal_or_rollback_target"}, pending.Requires)
	require.Equal(t, "release", pending.Data["label"])
	require.Equal(t, "approve release", pending.Data["reason"])
	require.Equal(t, true, pending.Data["checkpoint_required"])
	require.Equal(t, true, pending.Data["checkpoint_configured"])
	require.Equal(t, suspendCheckpointReferenceContext, pending.Data["checkpoint_reference_context"])
	require.Equal(t, []interface{}{"resume", "reject", "rollback"}, pending.Data["actions"])
}

func TestSuspendFreshReverserRejectsInvalidReceipts(t *testing.T) {
	t.Parallel()
	builder := &SuspendBuilder{ToolName: "approval_alias"}
	fresh := builder.BuildReverser()

	valid := suspendReceipt{
		Version: suspendReceiptVersion, Strategy: suspendReceiptStrategy,
		Declaration: "approval_alias", Label: "approval", Reason: "review",
		CheckpointRequired: false, CheckpointConfigured: false,
		CheckpointReferenceContext: suspendCheckpointReferenceContext,
	}
	unknown := receiptJSON(t, map[string]interface{}{
		"version": valid.Version, "strategy": valid.Strategy,
		"declaration": valid.Declaration, "label": valid.Label, "reason": valid.Reason,
		"checkpoint_required": false, "checkpoint_configured": false,
		"checkpoint_reference_context": valid.CheckpointReferenceContext,
		"unexpected":                   true,
	})
	wrongDeclaration := valid
	wrongDeclaration.Declaration = "other_alias"

	for _, test := range []struct {
		name    string
		prior   core.Result
		wantErr string
	}{
		{name: "missing", prior: core.Result{CommandName: "approval_alias"}, wantErr: "receipt is required"},
		{name: "unknown field", prior: core.Result{CommandName: "approval_alias", Receipt: unknown}, wantErr: "unknown field"},
		{name: "wrong persisted command", prior: core.Result{CommandName: "other_alias", Receipt: receiptJSON(t, valid)}, wantErr: "does not match declaration"},
		{name: "wrong receipt declaration", prior: core.Result{CommandName: "approval_alias", Receipt: receiptJSON(t, wrongDeclaration)}, wantErr: "does not match"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := fresh.Undo(test.prior)
			require.Equal(t, core.CommandError, result.Signal)
			require.ErrorContains(t, result.Err, test.wantErr)
			require.Empty(t, result.Receipt)
		})
	}
}

func TestExitAliasIsReceiptlessAndHasNoReverser(t *testing.T) {
	t.Parallel()
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{Shutdown: func() {}})
	factory, ok := br.Resolve("exit_agent")
	require.True(t, ok)

	const alias = "shutdown_after_drain"
	builder, err := factory(catalog.ToolDef{
		Name: alias, Type: "builtin", Init: "exit_agent",
		Config: map[string]interface{}{"reason": "finished", "status": "success"},
	}, nil)
	require.NoError(t, err)
	_, reversible := builder.(core.Reverser)
	require.False(t, reversible)

	cmd := builder.Build(core.Result{})
	require.Equal(t, alias, cmd.Name())
	executed := cmd.Execute()
	require.Equal(t, core.Signal("AgentExited"), executed.Signal)
	require.Equal(t, alias, executed.CommandName)
	require.Empty(t, executed.Receipt)

	reversed := cmd.Undo(executed)
	require.Equal(t, core.CommandError, reversed.Signal)
	require.ErrorContains(t, reversed.Err, "irreversible")
	require.Equal(t, alias, reversed.CommandName)
	require.Empty(t, reversed.Receipt)
}

func TestLifecycleAliasesAndFailuresRemainReceiptless(t *testing.T) {
	t.Parallel()
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{})

	suspendFactory, ok := br.Resolve("suspend")
	require.True(t, ok)
	suspendBuilder, err := suspendFactory(catalog.ToolDef{
		Name: "required_checkpoint_alias", Type: "builtin", Init: "suspend",
		Config: map[string]interface{}{"require_checkpoint": true},
	}, nil)
	require.NoError(t, err)
	suspendFailure := suspendBuilder.Build(core.Result{}).Execute()
	require.Equal(t, core.CommandError, suspendFailure.Signal)
	require.Equal(t, "required_checkpoint_alias", suspendFailure.CommandName)
	require.Empty(t, suspendFailure.Receipt)

	exitFactory, ok := br.Resolve("exit_agent")
	require.True(t, ok)
	exitBuilder, err := exitFactory(catalog.ToolDef{
		Name: "missing_shutdown_alias", Type: "builtin", Init: "exit_agent",
	}, nil)
	require.NoError(t, err)
	exitFailure := exitBuilder.Build(core.Result{}).Execute()
	require.Equal(t, core.CommandError, exitFailure.Signal)
	require.Equal(t, "missing_shutdown_alias", exitFailure.CommandName)
	require.Empty(t, exitFailure.Receipt)
}

func TestRollbackReportsSuspendPendingAndExitIrreversibleSkip(t *testing.T) {
	t.Parallel()
	defs := lifecyclePolicyDeclarations(t)
	suspendDef := renamedLifecycleDef(t, defs, "suspend", "approval_alias")
	exitDef := renamedLifecycleDef(t, defs, "exit_agent", "exit_alias")

	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{
		Checkpoint: &core.InMemoryCheckpoint{},
		Shutdown:   func() {},
	})
	suspendFactory, ok := br.Resolve("suspend")
	require.True(t, ok)
	exitFactory, ok := br.Resolve("exit_agent")
	require.True(t, ok)
	suspendBuilder, err := suspendFactory(suspendDef, nil)
	require.NoError(t, err)
	exitBuilder, err := exitFactory(exitDef, nil)
	require.NoError(t, err)
	suspended := suspendBuilder.Build(core.Result{}).Execute()
	require.Equal(t, core.AwaitApproval, suspended.Signal)
	exited := exitBuilder.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("AgentExited"), exited.Signal)

	registry := core.NewRegistry()
	registry.Register(suspendDef.ToToolSpec(), suspendBuilder)
	registry.Register(exitDef.ToToolSpec(), exitBuilder)
	report, err := rollbackViaReceipts(rollbackViaReceiptsOptions{
		Reverter: &recordingReverter{}, Registry: registry, RunID: "run-lifecycle-policy",
		Execution: core.Execution{
			{Iteration: 1, CommandName: "seed"},
			{Iteration: 2, CommandName: suspendDef.Name, Receipt: suspended.Receipt},
			{Iteration: 3, CommandName: exitDef.Name},
		},
		TargetIteration: 1,
	})

	require.NoError(t, err, report.Detail)
	require.Zero(t, report.Reverted)
	require.Equal(t, []string{exitDef.Name}, report.Skipped)
	require.Len(t, report.PendingCompensation, 1)
	require.Equal(t, suspendDef.Name, report.PendingCompensation[0].CommandName)
	require.Equal(t, suspendCheckpointReferenceContext,
		report.PendingCompensation[0].Data["checkpoint_reference_context"])
	require.Contains(t, report.Detail, "exit_alias: skipped (irreversible by declaration)")
	require.Contains(t, report.Detail, "approval_alias: compensation required")
}

func TestLifecyclePolicyDeclarationsMatchRuntimeContracts(t *testing.T) {
	t.Parallel()
	defs := lifecyclePolicyDeclarations(t)
	require.NoError(t, catalog.ValidateReceiptContracts(defs))

	suspendDef := lifecycleDef(t, defs, "suspend")
	require.Equal(t, "compensatable", suspendDef.Reversibility.Classification)
	require.Equal(t, "compensating_action", suspendDef.Undo.Strategy)
	require.Contains(t, suspendDef.Undo.Captures, "checkpoint_reference_context")

	exitDef := lifecycleDef(t, defs, "exit_agent")
	require.Equal(t, "irreversible", exitDef.Reversibility.Classification)
	require.True(t, exitDef.Reversibility.RequiresConfirmation)
	require.Equal(t, "irreversible", exitDef.Undo.Strategy)
	require.Empty(t, exitDef.Undo.Captures)
	require.Empty(t, exitDef.Undo.Requires)
}

func lifecyclePolicyDeclarations(t *testing.T) []catalog.ToolDef {
	t.Helper()
	root := repoRootFromLifecycleRuntime()
	defs, err := catalog.LoadToolDeclarations([]string{
		filepath.Join(root, "tools", "builtin", "suspend.yaml"),
		filepath.Join(root, "tools", "builtin", "lifecycle", "exit-agent.yaml"),
	})
	require.NoError(t, err)
	return defs
}

func lifecycleDef(t *testing.T, defs []catalog.ToolDef, name string) catalog.ToolDef {
	t.Helper()
	for _, def := range defs {
		if def.Name == name {
			return def
		}
	}
	t.Fatalf("lifecycle declaration %q not found", name)
	return catalog.ToolDef{}
}

func renamedLifecycleDef(t *testing.T, defs []catalog.ToolDef, name, alias string) catalog.ToolDef {
	t.Helper()
	def := lifecycleDef(t, defs, name)
	def.Name = alias
	return def
}

func receiptJSON(t *testing.T, value interface{}) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}
