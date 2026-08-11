// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"encoding/json"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolexec "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/exec"
	toolotlp "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/otlp"
)

// reverserStub is a core.Reverser whose receipt-driven Undo succeeds or fails on
// demand, so the receipt walk can be exercised without a real transport.
type reverserStub struct {
	name                 string
	undoFails            bool
	compensationRequired bool
}

func (b reverserStub) Build(core.Result) core.Command { return undoStub(b) }
func (b reverserStub) BuildReverser() core.Command    { return undoStub(b) }

type undoStub reverserStub

func (c undoStub) Name() string { return c.name }
func (c undoStub) Execute() core.Result {
	return core.Result{Signal: core.ToolDone, CommandName: c.name}
}
func (c undoStub) Undo(core.Result) core.Result {
	if c.compensationRequired {
		return core.Result{
			Signal: core.CompensationRequired, CommandName: c.name,
			Output: "operator action required",
		}
	}
	if c.undoFails {
		return core.Result{Signal: core.CommandError, CommandName: c.name, Output: "undo boom", Err: errors.New("undo boom")}
	}
	return core.Result{Signal: core.ToolDone, CommandName: c.name, Output: "undone"}
}

// recordingReverter is a CheckpointReverter that only records the Revert call;
// the receipt walk's inputs come from the Execution passed to rollbackViaReceipts.
type recordingReverter struct {
	reverted bool
	step     int
}

func (r *recordingReverter) Save(core.Position, core.Execution) error {
	return nil
}
func (r *recordingReverter) Load() (core.Position, core.Execution, error) {
	return core.Position{}, nil, nil
}
func (r *recordingReverter) Revert(_ string, step int) error {
	r.reverted = true
	r.step = step
	return nil
}

var _ core.CheckpointReverter = (*recordingReverter)(nil)

// TestRollbackViaReceiptsContinuesPastFailure proves that a failing receipt-walk
// Undo does not stop the walk, that a later entry is still reversed, and that
// the whole rollback is reported as a partial failure carrying the reversed
// count and the failed entry (srd026 R3.7, R6.3, R6.4; GH-491).
func TestRollbackViaReceiptsContinuesPastFailure(t *testing.T) {
	t.Parallel()

	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "fails", Visibility: core.Internal}, reverserStub{name: "fails", undoFails: true})
	reg.Register(core.ToolSpec{Name: "ok", Visibility: core.Internal}, reverserStub{name: "ok"})

	// Steps after the target (iteration 1) are walked in reverse: step 2 "fails"
	// then step 1 "ok". The failure must not prevent "ok" from being reversed.
	execution := core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "ok", Signal: core.ToolDone, Receipt: "r-ok"},
		{Iteration: 3, CommandName: "fails", Signal: core.ToolDone, Receipt: "r-fail"},
	}

	report, err := rollbackViaReceipts(rollbackViaReceiptsOptions{
		Reverter:        &recordingReverter{},
		Registry:        reg,
		RunID:           "run-mix",
		Execution:       execution,
		TargetIteration: 1,
	})

	var partial *PartialRollbackError
	require.ErrorAs(t, err, &partial)
	require.Equal(t, 1, partial.Reverted, report.Detail)
	require.Len(t, partial.Failures, 1)
	require.Equal(t, "fails", partial.Failures[0].CommandName)
	require.Equal(t, "undo boom", partial.Failures[0].Detail)

	require.Equal(t, 1, report.Reverted)
	require.Contains(t, report.Detail, "step=1 ok: undone")
	require.Contains(t, report.Detail, "step=2 fails: undo failed")
	require.Contains(t, report.Detail, "reversed 1, pending compensation 0, skipped 0, failed 1")
}

// TestRollbackViaReceiptsCleanWhenAllReverse proves a fully reversible run
// returns no error and a clean tally.
func TestRollbackViaReceiptsCleanWhenAllReverse(t *testing.T) {
	t.Parallel()

	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "ok", Visibility: core.Internal}, reverserStub{name: "ok"})

	execution := core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "ok", Signal: core.ToolDone, Receipt: "r-ok"},
	}

	report, err := rollbackViaReceipts(rollbackViaReceiptsOptions{
		Reverter:        &recordingReverter{},
		Registry:        reg,
		RunID:           "run-clean",
		Execution:       execution,
		TargetIteration: 1,
	})
	require.NoError(t, err)
	require.Equal(t, 1, report.Reverted)
	require.Contains(t, report.Detail, "reversed 1, pending compensation 0, skipped 0, failed 0")
}

// TestRollbackViaReceiptsHonorsExecReversibilityTiers exercises the real exec
// builder for all three declared tiers. Reversible effects run Undo,
// compensatable effects become operator work rather than failures, and
// irreversible effects are skipped by declaration even without a receipt.
func TestRollbackViaReceiptsHonorsExecReversibilityTiers(t *testing.T) {
	t.Parallel()

	reversible := rollbackExecDef(
		"reversible_exec", "reversible", "workspace_restore", "restore workspace", nil,
	)
	compensatable := rollbackExecDef(
		"compensatable_exec", "compensatable", "compensating_action",
		"close created issue", []string{"issue_id"},
	)
	irreversible := rollbackExecDef(
		"irreversible_exec", "irreversible", "irreversible", "published externally", nil,
	)

	reversibleResult := (&toolexec.ExecBuilder{Def: reversible}).Build(core.Result{}).Execute()
	require.Equal(t, core.ToolDone, reversibleResult.Signal, reversibleResult.Output)
	require.NotEmpty(t, reversibleResult.Receipt)
	compensatableResult := (&toolexec.ExecBuilder{Def: compensatable}).Build(core.Result{}).Execute()
	require.Equal(t, core.ToolDone, compensatableResult.Signal, compensatableResult.Output)
	require.NotEmpty(t, compensatableResult.Receipt)

	reg := core.NewRegistry()
	for _, def := range []catalog.ToolDef{reversible, compensatable, irreversible} {
		reg.Register(def.ToToolSpec(), &toolexec.ExecBuilder{Def: def})
	}
	execution := core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{Iteration: 2, CommandName: reversible.Name, Signal: core.ToolDone, Receipt: reversibleResult.Receipt},
		{Iteration: 3, CommandName: compensatable.Name, Signal: core.ToolDone, Receipt: compensatableResult.Receipt},
		{Iteration: 4, CommandName: irreversible.Name, Signal: core.ToolDone},
	}

	report, err := rollbackViaReceipts(rollbackViaReceiptsOptions{
		Reverter: &recordingReverter{}, Registry: reg,
		RunID: "run-tiers", Execution: execution, TargetIteration: 1,
	})
	require.NoError(t, err, report.Detail)
	require.Equal(t, 1, report.Reverted)
	require.Equal(t, []string{"irreversible_exec"}, report.Skipped)
	require.Len(t, report.PendingCompensation, 1)
	require.Equal(t, "compensatable_exec", report.PendingCompensation[0].CommandName)
	require.Equal(t, "close created issue", report.PendingCompensation[0].Description)
	require.Equal(t, []string{"issue_id"}, report.PendingCompensation[0].Requires)
	require.Contains(t, report.Detail, "irreversible by declaration")
	require.Contains(t, report.Detail, "pending compensation 1")
}

func rollbackExecDef(name, classification, strategy, description string, requires []string) catalog.ToolDef {
	return catalog.ToolDef{
		Name: name, Type: "exec", Binary: "true", Args: []string{},
		Reversibility: catalog.ToolReversibility{Classification: classification},
		Undo: catalog.ToolUndoContract{
			Strategy: strategy, Description: description, Requires: requires,
		},
	}
}

func TestRollbackViaReceiptsStopsOTLPReceiver(t *testing.T) {
	t.Parallel()
	state := toolotlp.NewState()
	def := catalog.ToolDef{
		Name: "otlp_receiver_launch", Type: "builtin",
		Reversibility: catalog.ToolReversibility{Classification: "compensatable"},
		Undo:          catalog.ToolUndoContract{Strategy: "receiver_stop"},
	}
	builder := toolotlp.ReceiverBuilder{
		ToolName: def.Name, Init: toolotlp.InitReceiverLaunch,
		Config: toolotlp.ReceiverConfig{Name: "rollback", Address: "127.0.0.1:0"},
		State:  state,
	}
	launch := builder.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("ReceiverLaunched"), launch.Signal, launch.Output)
	require.NotEmpty(t, launch.Receipt)
	var output struct {
		Address string `json:"address"`
	}
	require.NoError(t, json.Unmarshal([]byte(launch.Output), &output))

	reg := core.NewRegistry()
	reg.Register(def.ToToolSpec(), builder)
	report, err := rollbackViaReceipts(rollbackViaReceiptsOptions{
		Reverter: &recordingReverter{}, Registry: reg, RunID: "run-otlp",
		Execution: core.Execution{
			{Iteration: 1, CommandName: "read"},
			{Iteration: 2, CommandName: def.Name, Receipt: launch.Receipt},
		},
		TargetIteration: 1,
	})
	require.NoError(t, err, report.Detail)
	require.Equal(t, 1, report.Reverted)
	listener, err := net.Listen("tcp", output.Address)
	require.NoError(t, err, "rollback must release the receiver address")
	require.NoError(t, listener.Close())
}

// TestRollbackViaReceiptsSurfacesEveryCompensation proves GH-1377's fallback:
// each receipt-walk Undo — reversed, skipped, or failed — is recorded on the
// rollback span, so the compensation fan-out (e.g. REST Undo re-entering the
// HTTP client) is observable rather than invisible.
func TestRollbackViaReceiptsSurfacesEveryCompensation(t *testing.T) {
	t.Parallel()

	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "ok", Visibility: core.Internal}, reverserStub{name: "ok"})
	reg.Register(core.ToolSpec{Name: "fails", Visibility: core.Internal}, reverserStub{name: "fails", undoFails: true})
	reg.Register(
		core.ToolSpec{Name: "pending", Visibility: core.Internal},
		reverserStub{name: "pending", compensationRequired: true},
	)

	execution := core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "ok", Signal: core.ToolDone, Receipt: "r-ok"},
		{Iteration: 3, CommandName: "fails", Signal: core.ToolDone, Receipt: "r-fail"},
		{Iteration: 4, CommandName: "irreversible", Signal: core.ToolDone},
		{Iteration: 5, CommandName: "pending", Signal: core.ToolDone, Receipt: "r-pending"},
	}

	tr := tracing.NewRecordingTracer()
	_, _ = rollbackViaReceipts(rollbackViaReceiptsOptions{
		Reverter:        &recordingReverter{},
		Registry:        reg,
		Tracer:          tr,
		RunID:           "run-events",
		Execution:       execution,
		TargetIteration: 1,
	})

	reversed := tr.FindEvent("rollback.entry_reversed")
	require.NotNil(t, reversed, "a successful compensation must emit rollback.entry_reversed")
	require.Equal(t, "ok", reversed.Attrs["command"])
	require.NotNil(t, tr.FindEvent("rollback.entry_compensation_required"))
	require.NotNil(t, tr.FindEvent("rollback.entry_undo_failed"))
	require.NotNil(t, tr.FindEvent("rollback.entry_skipped"))
}
