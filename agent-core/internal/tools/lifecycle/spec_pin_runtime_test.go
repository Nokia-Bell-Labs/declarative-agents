// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package lifecycle

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// undoCall is one receipt-walk Undo as the originating tool saw it.
type undoCall struct {
	name    string
	receipt string
}

type undoLog struct {
	mu    sync.Mutex
	calls []undoCall
}

func (l *undoLog) record(call undoCall) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, call)
}

// recordingReverserStub records each Undo it is driven with.
type recordingReverserStub struct {
	name string
	log  *undoLog
}

func (b recordingReverserStub) Build(core.Result) core.Command { return recordingUndo(b) }
func (b recordingReverserStub) BuildReverser() core.Command    { return recordingUndo(b) }

type recordingUndo recordingReverserStub

func (c recordingUndo) Name() string { return c.name }
func (c recordingUndo) Execute() core.Result {
	return core.Result{Signal: core.ToolDone, CommandName: c.name}
}
func (c recordingUndo) Undo(prior core.Result) core.Result {
	c.log.record(undoCall{name: c.name, receipt: prior.Receipt})
	return core.Result{Signal: core.ToolDone, CommandName: c.name, Output: "undone"}
}

// TestSpecPinRollbackWalksReceiptsInReverseUnchanged pins R-LCM-005's order
// and receipt opacity: the walk visits the steps after the target from the
// latest back to the earliest, and hands each step's receipt to its own tool's
// Undo byte for byte, whatever the receipt holds.
func TestSpecPinRollbackWalksReceiptsInReverseUnchanged(t *testing.T) {
	t.Parallel()
	log := &undoLog{}
	reg := core.NewRegistry()
	for _, name := range []string{"first", "second", "third"} {
		reg.Register(core.ToolSpec{Name: name, Visibility: core.Internal}, recordingReverserStub{name: name, log: log})
	}
	// Receipts no engine format describes: not JSON, binary, and empty-looking.
	receipts := map[string]string{
		"first":  "\x00\x01 not json {",
		"second": "]]] opaque [[[",
		"third":  "  ",
	}
	execution := core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "first", Signal: core.ToolDone, Receipt: receipts["first"]},
		{Iteration: 3, CommandName: "second", Signal: core.ToolDone, Receipt: receipts["second"]},
		{Iteration: 4, CommandName: "third", Signal: core.ToolDone, Receipt: receipts["third"]},
	}

	report, err := rollbackViaReceipts(rollbackViaReceiptsOptions{
		Reverter:        &recordingReverter{},
		Registry:        reg,
		RunID:           "run-order",
		Execution:       execution,
		TargetIteration: 1,
	})

	require.NoError(t, err)
	require.Equal(t, 3, report.Reverted)
	require.Equal(t, []undoCall{
		{name: "third", receipt: receipts["third"]},
		{name: "second", receipt: receipts["second"]},
		{name: "first", receipt: receipts["first"]},
	}, log.calls)
}
