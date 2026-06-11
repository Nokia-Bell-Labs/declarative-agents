// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

func TestFormatCheckpointHistory(t *testing.T) {
	cp := sampleCheckpoint("cp-1", time.Unix(100, 0).UTC())

	out := formatCheckpointHistory(cp)

	require.Contains(t, out, "checkpoint: cp-1")
	require.Contains(t, out, "iteration: 2")
	require.Contains(t, out, "state: Working")
	require.Contains(t, out, "1  read  Start -> Reading  signal=ToolDone  workspace=ref-1")
	require.Contains(t, out, "2  write  Reading -> Working  signal=EditDone  workspace=ref-2")
}

func TestResolveCheckpointIDLatest(t *testing.T) {
	ctx := context.Background()
	store := core.NewFileStore(t.TempDir())
	saveAgentCheckpoint(t, store, sampleCheckpoint("older", time.Unix(100, 0).UTC()))
	saveAgentCheckpoint(t, store, sampleCheckpoint("newer", time.Unix(200, 0).UTC()))

	id, err := resolveCheckpointID(ctx, store, "latest")

	require.NoError(t, err)
	require.Equal(t, "newer", id)
}

func TestRollbackCheckpointToIteration(t *testing.T) {
	cp := sampleCheckpoint("cp-1", time.Unix(100, 0).UTC())

	rolledBack, ref, err := rollbackCheckpointToIteration(cp, 1)

	require.NoError(t, err)
	require.Equal(t, "ref-1", ref)
	require.Equal(t, 1, rolledBack.Iteration)
	require.Equal(t, 1, rolledBack.AgentState.Iteration)
	require.Equal(t, core.State("Reading"), rolledBack.AgentState.State)
	require.Equal(t, "ref-1", rolledBack.WorkspaceRef)
	require.Len(t, rolledBack.History, 1)
	require.True(t, strings.HasPrefix(rolledBack.ID, "rollback-cp-1-to-1-"))
}

func TestRunHistoryPrintsCheckpointHistory(t *testing.T) {
	stateStoreDir := t.TempDir()
	store := core.NewFileStore(stateStoreDir)
	saveAgentCheckpoint(t, store, sampleCheckpoint("cp-1", time.Unix(100, 0).UTC()))
	flagStateStoreDir = stateStoreDir
	flagHistoryCheckpoint = "cp-1"
	t.Cleanup(resetLifecycleFlags)

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)

	err := runHistory(cmd, nil)

	require.NoError(t, err)
	require.Contains(t, out.String(), "checkpoint: cp-1")
	require.Contains(t, out.String(), "history:")
}

func TestRunRollbackRefusesWorkspaceRestoreWithoutDirectory(t *testing.T) {
	stateStoreDir := t.TempDir()
	store := core.NewFileStore(stateStoreDir)
	saveAgentCheckpoint(t, store, sampleCheckpoint("cp-1", time.Unix(100, 0).UTC()))
	flagStateStoreDir = stateStoreDir
	flagRollbackCheckpoint = "cp-1"
	flagRollbackToIteration = 1
	flagDirectory = ""
	t.Cleanup(resetLifecycleFlags)

	err := runRollback(&cobra.Command{}, nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "--directory is required")
}

func TestRunRollbackPersistsNewCheckpointWithoutWorkspaceRestore(t *testing.T) {
	stateStoreDir := t.TempDir()
	store := core.NewFileStore(stateStoreDir)
	cp := sampleCheckpoint("cp-1", time.Unix(100, 0).UTC())
	cp.History[0].WorkspaceRef = ""
	saveAgentCheckpoint(t, store, cp)
	flagStateStoreDir = stateStoreDir
	flagRollbackCheckpoint = "cp-1"
	flagRollbackToIteration = 1
	flagDirectory = ""
	t.Cleanup(resetLifecycleFlags)

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	err := runRollback(cmd, nil)

	require.NoError(t, err)
	require.Contains(t, out.String(), "new checkpoint:")
	keys, err := store.List(context.Background(), "checkpoint/")
	require.NoError(t, err)
	require.Len(t, keys, 2)
}

func sampleCheckpoint(id string, ts time.Time) core.Checkpoint {
	return core.Checkpoint{
		ID:        id,
		Iteration: 2,
		Timestamp: ts,
		AgentState: core.AgentSnapshot{
			State:     "Working",
			Signal:    core.EditDone,
			Iteration: 2,
		},
		WorkspaceRef: "ref-2",
		History: []core.HistoryDigest{
			{
				Iteration:    1,
				CommandName:  "read",
				FromState:    "Start",
				ToState:      "Reading",
				Signal:       core.ToolDone,
				WorkspaceRef: "ref-1",
			},
			{
				Iteration:    2,
				CommandName:  "write",
				FromState:    "Reading",
				ToState:      "Working",
				Signal:       core.EditDone,
				WorkspaceRef: "ref-2",
			},
		},
	}
}

func saveAgentCheckpoint(t *testing.T, store core.StateStore, cp core.Checkpoint) {
	t.Helper()
	data, err := json.Marshal(cp)
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), "checkpoint/"+cp.ID, data))
}

func resetLifecycleFlags() {
	flagStateStoreDir = ""
	flagHistoryCheckpoint = "latest"
	flagRollbackCheckpoint = "latest"
	flagRollbackToIteration = -1
	flagDirectory = ""
}
