// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
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
		"jurist":    {},
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

func TestBuiltinFactoryCatalogSelectsEntriesByInit(t *testing.T) {
	t.Parallel()

	catalog := builtinFactoryCatalog(&agentState{})
	byName := make(map[string]builtinFactoryCatalogEntry, len(catalog))
	for _, entry := range catalog {
		byName[entry.Name] = entry
	}

	require.True(t, byName["planning"].selectedBy(map[string]bool{"execute_task": true}))
	require.True(t, byName["evaluation"].selectedBy(map[string]bool{"run_point": true}))
	require.True(t, byName["bench"].selectedBy(map[string]bool{"launch_eval": true}))
	require.True(t, byName["spec_validation"].selectedBy(map[string]bool{"validate_specs": true}))
	require.True(t, byName["lifecycle"].selectedBy(map[string]bool{"checkpoint_history": true}))
	require.True(t, byName["lifecycle"].selectedBy(map[string]bool{"checkpoint_rollback": true}))
	require.False(t, byName["planning"].selectedBy(map[string]bool{"launch_eval": true}))
}

func TestBuiltinFactoryCatalogCoversSelectedActiveInits(t *testing.T) {
	t.Parallel()

	catalog := builtinFactoryCatalog(&agentState{})
	covered := make(map[string]bool)
	for _, entry := range catalog {
		for _, init := range entry.Inits {
			covered[init] = true
		}
	}

	for _, init := range []string{
		"file_read", "file_write", "file_edit", "file_find", "file_list",
		"invoke_llm", "parse_response", "report_parse_error", "reset_history",
		"nudge_reread", "done", "suspend", "checkpoint_history",
		"checkpoint_rollback", "validate", "self_invoke",
		"extract_task", "extract_all", "assemble_prompt", "parse_plan",
		"create_issue", "execute_task", "check_result",
		"parse_suite_config", "discover_suite_samples", "expand_eval_grid",
		"init_eval_session", "report_suite_summary", "next_point", "run_point",
		"report_session", "run_agent", "run_oracle_check", "collect_trace_tokens",
		"check_agent_version", "summarize_point_results", "collect_metrics",
		"dump_config", "serve_ui", "launch_eval", "load_corpus", "validate_specs",
		"format_report",
	} {
		require.True(t, covered[init], "catalog should cover init %q", init)
	}
}

func TestRootCommandHasNoLifecycleSubcommands(t *testing.T) {
	t.Parallel()

	for _, cmd := range rootCmd.Commands() {
		require.NotContains(t, []string{"history", "rollback"}, cmd.Name())
	}
	assertMainDeclsAbsent(t, map[string]bool{
		"historyCmd":     true,
		"rollbackCmd":    true,
		"runHistory":     true,
		"runRollback":    true,
		"lifecycleStore": true,
	})
}

func TestRootCommandHasNoLifecycleOnlyFlags(t *testing.T) {
	t.Parallel()

	for _, flag := range []string{
		"checkpoint", "to-iteration", "machine", "tools",
		"tools-declaration", "tool-config-dir", "profiles-dir", "input",
	} {
		require.Nil(t, rootCmd.PersistentFlags().Lookup(flag), "flag %q must not be public", flag)
	}
	for _, flag := range []string{"profile", "state-store-dir", "resume-checkpoint", "resume-signal", "directory", "request"} {
		require.NotNil(t, rootCmd.PersistentFlags().Lookup(flag), "universal flag %q should remain", flag)
	}
	assertMainDeclsAbsent(t, map[string]bool{
		"flagHistoryCheckpoint":   true,
		"flagRollbackCheckpoint":  true,
		"flagRollbackToIteration": true,
		"flagMachine":             true,
		"flagTools":               true,
		"flagToolDeclarations":    true,
		"flagToolConfigDirs":      true,
		"flagProfilesDir":         true,
		"flagInput":               true,
	})
}

func TestRootCommandHelpShowsProfileOnlyRuntimeFlags(t *testing.T) {
	t.Parallel()

	usage := rootCmd.UsageString()

	for _, text := range []string{"--machine", "--tools", "--tools-declaration", "--tool-config-dir", "--profiles-dir", "--input"} {
		require.NotContains(t, usage, text)
	}
	for _, text := range []string{"--profile", "--request", "--output", "--directory"} {
		require.Contains(t, usage, text)
	}
}

func TestProfileStartupLoadsActiveProfiles(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	root := repoRootFromTest(t)
	profiles := []string{
		"agents/generator/profile.yaml",
		"agents/evaluator/profile.yaml",
		"agents/bench/profile.yaml",
		"agents/jurist/profile.yaml",
		"agents/lifecycle/history/profile.yaml",
		"agents/lifecycle/rollback/profile.yaml",
		"agents/lifecycle/approval/profile.yaml",
	}
	for _, rel := range profiles {
		t.Run(rel, func(t *testing.T) {
			clearAgentFlags()
			flagProfile = filepath.Join(root, rel)

			cfg, err := loadRuntimeConfig()
			require.NoError(t, err)
			defs, err := loadProfileToolDefs(cfg)
			require.NoError(t, err)
			spec, err := core.LoadMachineSpec(cfg.Machine)
			require.NoError(t, err)
			require.NoError(t, catalog.ValidateToolEmits(spec, defs))
		})
	}
}

func TestApprovalLifecycleProfileSuspendsAndResumesApproved(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	profilePath := filepath.Join(repoRootFromTest(t), "agents", "lifecycle", "approval", "profile.yaml")
	storeDir := t.TempDir()

	clearAgentFlags()
	flagProfile = profilePath
	flagStateStoreDir = storeDir
	firstStderr, err := captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.NoError(t, err)
	require.Contains(t, firstStderr, "terminal state: suspended")

	store := core.NewFileStore(storeDir)
	keys, err := store.List(context.Background(), "checkpoint/")
	require.NoError(t, err)
	require.Len(t, keys, 1)
	checkpointID := strings.TrimPrefix(keys[0], "checkpoint/")

	clearAgentFlags()
	flagProfile = profilePath
	flagStateStoreDir = storeDir
	flagResumeCheckpoint = checkpointID
	flagResumeSignal = string(core.Approved)
	secondStderr, err := captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.NoError(t, err)
	require.Contains(t, secondStderr, "terminal state: succeeded")
}

func assertMainDeclsAbsent(t *testing.T, forbidden map[string]bool) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(filepath.Dir(currentFile), "main.go"), nil, 0)
	require.NoError(t, err)
	for _, decl := range parsed.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			require.False(t, forbidden[d.Name.Name], "main.go must not declare %s", d.Name.Name)
		case *ast.GenDecl:
			assertGenDeclNamesAbsent(t, d, forbidden)
		}
	}
}

func assertGenDeclNamesAbsent(t *testing.T, decl *ast.GenDecl, forbidden map[string]bool) {
	t.Helper()
	for _, spec := range decl.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range value.Names {
			require.False(t, forbidden[name.Name], "main.go must not declare %s", name.Name)
		}
	}
}

type agentFlagSnapshot struct {
	profile          string
	otelLog          string
	otelParent       string
	directory        string
	verboseTrace     bool
	request          string
	output           string
	stateStoreDir    string
	resumeCheckpoint string
	resumeSignal     string
}

func snapshotAgentFlags() agentFlagSnapshot {
	return agentFlagSnapshot{
		profile:          flagProfile,
		otelLog:          flagOTelLog,
		otelParent:       flagOTelParent,
		directory:        flagDirectory,
		verboseTrace:     flagVerboseTrace,
		request:          flagRequest,
		output:           flagOutput,
		stateStoreDir:    flagStateStoreDir,
		resumeCheckpoint: flagResumeCheckpoint,
		resumeSignal:     flagResumeSignal,
	}
}

func restoreAgentFlags(s agentFlagSnapshot) {
	flagProfile = s.profile
	flagOTelLog = s.otelLog
	flagOTelParent = s.otelParent
	flagDirectory = s.directory
	flagVerboseTrace = s.verboseTrace
	flagRequest = s.request
	flagOutput = s.output
	flagStateStoreDir = s.stateStoreDir
	flagResumeCheckpoint = s.resumeCheckpoint
	flagResumeSignal = s.resumeSignal
}

func clearAgentFlags() {
	restoreAgentFlags(agentFlagSnapshot{resumeSignal: string(core.Approved)})
}

func repoRootFromTest(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
}

func captureStderr(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	defer func() { os.Stderr = old }()

	runErr := fn()
	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, readErr := buf.ReadFrom(r)
	require.NoError(t, readErr)
	require.NoError(t, r.Close())
	return buf.String(), runErr
}

func TestFormatCheckpointHistory(t *testing.T) {
	cp := sampleCheckpoint("cp-1", time.Unix(100, 0).UTC())

	out := core.FormatCheckpointHistory(cp)

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

	id, err := core.ResolveLatestCheckpointID(ctx, store, "latest")

	require.NoError(t, err)
	require.Equal(t, "newer", id)
}

func TestRollbackCheckpointToIteration(t *testing.T) {
	cp := sampleCheckpoint("cp-1", time.Unix(100, 0).UTC())

	result, err := core.RollbackCheckpoint(cp, 1)

	require.NoError(t, err)
	require.Equal(t, "ref-1", result.WorkspaceRef)
	require.Equal(t, 1, result.Checkpoint.Iteration)
	require.Equal(t, 1, result.Checkpoint.AgentState.Iteration)
	require.Equal(t, core.State("Reading"), result.Checkpoint.AgentState.State)
	require.Equal(t, "ref-1", result.Checkpoint.WorkspaceRef)
	require.Len(t, result.Checkpoint.History, 1)
	require.JSONEq(t, `{"conversation_len":1}`, string(result.Checkpoint.DomainState))
	require.True(t, strings.HasPrefix(result.Checkpoint.ID, "rollback-cp-1-to-1-"))
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

	result, err := core.RollbackCheckpoint(cp, 1)

	require.NoError(t, err)
	require.JSONEq(t, `[{"role":"user","content":"before"}]`, string(result.Checkpoint.ConversationLog))
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

	result, err := core.RollbackCheckpoint(cp, 1)

	require.NoError(t, err)
	require.JSONEq(t, `{"retry_count":3,"issue_id":"old"}`, string(result.Checkpoint.DomainState))
}

func TestRollbackCheckpointToIterationReportsMissingUndoMemento(t *testing.T) {
	cp := sampleCheckpoint("cp-1", time.Unix(100, 0).UTC())
	cp.History[1].Undo = nil

	_, err := core.RollbackCheckpoint(cp, 1)

	require.Contains(t, err.Error(), "rollback command restore")
	require.Contains(t, err.Error(), core.ErrUndoMementoMissing.Error())
	require.Contains(t, err.Error(), "write")
}

func TestRollbackCheckpointToIterationReportsIrreversibleUndoMemento(t *testing.T) {
	cp := sampleCheckpoint("cp-1", time.Unix(100, 0).UTC())
	irreversible := core.IrreversibleUndoMemento("write", "already published externally")
	cp.History[1].Undo = &irreversible

	_, err := core.RollbackCheckpoint(cp, 1)

	require.Contains(t, err.Error(), core.ErrUndoMementoIncompatible.Error())
	require.Contains(t, err.Error(), "irreversible")
	require.Contains(t, err.Error(), "already published externally")
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
