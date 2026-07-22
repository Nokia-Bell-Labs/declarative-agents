// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"encoding/json"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCheckpointHistoryExecuteFormatsExecutionLog(t *testing.T) {
	t.Parallel()
	cp := &core.InMemoryCheckpoint{}
	require.NoError(t, cp.Save(
		core.Position{
			CurrentState: "Working",
			LastSignal:   core.ToolDone,
			Snapshot:     core.AgentSnapshot{State: "Working", Signal: core.ToolDone, Iteration: 2},
		},
		core.Execution{
			{Iteration: 1, CommandName: "read", FromState: "Idle", ToState: "Reading", Signal: core.ToolDone},
			{Iteration: 2, CommandName: "write", FromState: "Reading", ToState: "Working", Signal: core.ToolDone, Receipt: `{"path":"a.txt"}`},
		},
	))

	cmd := (&CheckpointHistoryBuilder{Checkpoint: cp}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal)
	require.Equal(t, "checkpoint_history", res.CommandName)

	// Output is the structured checkpoint-history schema {run, history} (#493).
	var out struct {
		Run     string `json:"run"`
		History string `json:"history"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.Output), &out))
	require.Equal(t, "latest", out.Run)
	require.Contains(t, out.History, "state: Working")
	require.Contains(t, out.History, "step=0  iteration=1  read  Idle -> Reading  signal=ToolDone")
	require.Contains(t, out.History, "step=1  iteration=2  write  Reading -> Working  signal=ToolDone  reversible")
}

func TestCheckpointHistoryExecuteRequiresCheckpoint(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointHistoryBuilder{}).Build(core.Result{})

	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "requires a Checkpoint")
}

func TestCheckpointHistoryExecuteReportsNoCheckpoint(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointHistoryBuilder{Checkpoint: &core.InMemoryCheckpoint{}}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "no checkpoint persisted")
}

func TestCheckpointHistoryUndoIsNoop(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointHistoryBuilder{}).Build(core.Result{})
	require.Equal(t, core.ToolDone, cmd.Undo(core.Result{}).Signal)
}

// TestCheckpointHistoryEchoesExplicitRunSelector proves the structured history
// output surfaces the explicitly selected run identity (srd026 R2.1; GH-493).
func TestCheckpointHistoryEchoesExplicitRunSelector(t *testing.T) {
	t.Parallel()
	cp := &core.InMemoryCheckpoint{}
	require.NoError(t, cp.Save(core.Position{CurrentState: "Working"},
		core.Execution{{Iteration: 1, CommandName: "read", Signal: core.ToolDone}}))

	cmd := (&CheckpointHistoryBuilder{
		Config:     catalog.CheckpointHistoryConfig{Checkpoint: "run-42"},
		Checkpoint: cp,
	}).Build(core.Result{})
	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal)

	var out struct {
		Run     string `json:"run"`
		History string `json:"history"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.Output), &out))
	require.Equal(t, "run-42", out.Run)
	require.NotEmpty(t, out.History)
}
