// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

func TestMainRuntimeDoesNotBranchOnAgentModeNames(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}
	path := filepath.Join(filepath.Dir(currentFile), "main.go")
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	modeNames := map[string]struct{}{
		"generator": {},
		"planner":   {},
		"evaluator": {},
		"bench":     {},
		"constitution-auditor": {},
	}
	isModeLiteral := func(expr ast.Expr) (string, bool) {
		lit, ok := expr.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatalf("unquote %s: %v", lit.Value, err)
		}
		_, isMode := modeNames[value]
		return value, isMode
	}

	ast.Inspect(parsed, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BinaryExpr:
			if node.Op != token.EQL && node.Op != token.NEQ {
				return true
			}
			if value, ok := isModeLiteral(node.X); ok {
				t.Fatalf("cmd/agent must not branch on agent mode literal %q at %s; select behavior through machine/tools YAML", value, fset.Position(node.Pos()))
			}
			if value, ok := isModeLiteral(node.Y); ok {
				t.Fatalf("cmd/agent must not branch on agent mode literal %q at %s; select behavior through machine/tools YAML", value, fset.Position(node.Pos()))
			}
		case *ast.CaseClause:
			for _, expr := range node.List {
				if value, ok := isModeLiteral(expr); ok {
					t.Fatalf("cmd/agent must not switch on agent mode literal %q at %s; selected tool init gates are the allowed bootstrap boundary", value, fset.Position(expr.Pos()))
				}
			}
		}
		return true
	})
}

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
	require.JSONEq(t, `{"conversation_len":1}`, string(rolledBack.DomainState))
	require.True(t, strings.HasPrefix(rolledBack.ID, "rollback-cp-1-to-1-"))
}

func TestRollbackCheckpointToIterationRestoresConversationMemento(t *testing.T) {
	cp := sampleCheckpoint("cp-1", time.Unix(100, 0).UTC())
	cp.History[1].CommandName = "invoke_llm"
	cp.History[1].Undo = &core.UndoMemento{
		Version:     core.UndoMementoVersion,
		Kind:        core.UndoMementoReversible,
		CommandName: "invoke_llm",
		Payload:     json.RawMessage(`{"conversation":[{"role":"user","content":"before"}]}`),
	}

	rolledBack, _, err := rollbackCheckpointToIteration(cp, 1)

	require.NoError(t, err)
	require.JSONEq(t, `[{"role":"user","content":"before"}]`, string(rolledBack.ConversationLog))
}

func TestRollbackCheckpointToIterationRestoresPipelineDomainMemento(t *testing.T) {
	cp := sampleCheckpoint("cp-1", time.Unix(100, 0).UTC())
	cp.History[1].CommandName = "parse_plan"
	cp.History[1].Undo = &core.UndoMemento{
		Version:     core.UndoMementoVersion,
		Kind:        core.UndoMementoReversible,
		CommandName: "parse_plan",
		Payload:     json.RawMessage(`{"domain_state":{"retry_count":3,"issue_id":"old"}}`),
	}

	rolledBack, _, err := rollbackCheckpointToIteration(cp, 1)

	require.NoError(t, err)
	require.JSONEq(t, `{"retry_count":3,"issue_id":"old"}`, string(rolledBack.DomainState))
}

func TestRollbackCheckpointToIterationReportsMissingUndoMemento(t *testing.T) {
	cp := sampleCheckpoint("cp-1", time.Unix(100, 0).UTC())
	cp.History[1].Undo = nil

	_, _, err := rollbackCheckpointToIteration(cp, 1)

	require.Contains(t, err.Error(), "rollback command restore")
	require.Contains(t, err.Error(), core.ErrUndoMementoMissing.Error())
	require.Contains(t, err.Error(), "write")
}

func TestRollbackCheckpointToIterationReportsIrreversibleUndoMemento(t *testing.T) {
	cp := sampleCheckpoint("cp-1", time.Unix(100, 0).UTC())
	irreversible := core.IrreversibleUndoMemento("write", "already published externally")
	cp.History[1].Undo = &irreversible

	_, _, err := rollbackCheckpointToIteration(cp, 1)

	require.Contains(t, err.Error(), core.ErrUndoMementoIncompatible.Error())
	require.Contains(t, err.Error(), "irreversible")
	require.Contains(t, err.Error(), "already published externally")
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
		DomainState:  json.RawMessage(`{"conversation_len":2}`),
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
				Iteration:   2,
				CommandName: "write",
				FromState:   "Reading",
				ToState:     "Working",
				Signal:      core.EditDone,
				Undo: &core.UndoMemento{
					Version:     core.UndoMementoVersion,
					Kind:        core.UndoMementoReversible,
					CommandName: "write",
					Payload:     json.RawMessage(`{"domain_state":{"conversation_len":1}}`),
				},
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
