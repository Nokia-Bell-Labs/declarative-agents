// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/stretchr/testify/require"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
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
		"executor": {},
		"planner":  {},
		"critic":   {},
		"bench":    {},
		"jurist":   {},
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
	require.True(t, byName["documentation"].selectedBy(map[string]bool{"launch_documentation": true}))
	require.True(t, byName["documentation"].selectedBy(map[string]bool{"stop_documentation": true}))
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
		"format_report", "launch_documentation", "stop_documentation",
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
	for _, flag := range []string{"profile", "dolt-dsn", "resume-checkpoint", "resume-signal", "directory", "request"} {
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

func TestMainWiresExitAgentToDeferredShutdown(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(filepath.Join(repoRootFromTest(t), "cmd", "agent", "main.go"))
	require.NoError(t, err)

	require.Regexp(t, `shutdown:\s+shutdown\.Request`, string(source))
	require.NotRegexp(t, `shutdown:\s+func\(\) \{\}`, string(source))
}

func TestProfileStartupLoadsActiveProfiles(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	profileRoot := profileRootFromTest(t)
	profiles := []string{
		"executor/profile.yaml",
		"critic/profile.yaml",
		"bench/profile.yaml",
		"jurist/profile.yaml",
		"../testdata/conformance/lifecycle/history/profile.yaml",
		"../testdata/conformance/lifecycle/rollback/profile.yaml",
		"../testdata/conformance/lifecycle/approval/profile.yaml",
		"knowledge-manager/documentation-curator/profile.yaml",
	}
	for _, rel := range profiles {
		t.Run(rel, func(t *testing.T) {
			clearAgentFlags()
			flagProfile = filepath.Join(profileRoot, filepath.FromSlash(rel))

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

func TestValidateConfigValidProfileExitsZero(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	clearAgentFlags()
	flagProfile = profilePathFromTest(t, "monitor/profile.yaml")
	flagValidateConfig = true

	stderr, err := captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.NoError(t, err)
	require.Contains(t, stderr, "config valid")
	// Validate mode must not enter the run loop or bind servers.
	require.NotContains(t, stderr, "terminal state")
}

func TestValidateConfigInvalidRestExitsNonZero(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	monitorDir := filepath.Dir(profilePathFromTest(t, "monitor/profile.yaml"))
	dir := t.TempDir()
	badRest := filepath.Join(dir, "rest.yaml")
	require.NoError(t, os.WriteFile(badRest,
		[]byte("rest:\n  version: v1\n  auth:\n    broken:\n      type: totally-unsupported\n"), 0o644))
	profile := filepath.Join(dir, "profile.yaml")
	require.NoError(t, os.WriteFile(profile, []byte(fmt.Sprintf(
		"name: badrest\nmachine: %s\ntools:\n  - %s\ntool_declarations:\n  - %s\nrest_definitions:\n  - %s\n",
		filepath.Join(monitorDir, "machine.yaml"),
		filepath.Join(monitorDir, "tools.yaml"),
		filepath.Join(monitorDir, "declarations.yaml"),
		badRest)), 0o644))

	clearAgentFlags()
	flagProfile = profile
	flagValidateConfig = true

	_, err := captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported type")
}

func TestValidateConfigInvalidReceiptContractExitsNonZero(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	monitorDir := filepath.Dir(profilePathFromTest(t, "monitor/profile.yaml"))
	// Corrupt one already-selected monitor tool back to the inconsistent form
	// GH-494 targets: reversible with a state-mutating effect but a noop undo.
	// --validate-config must reject it (srd025 R3.5; GH-494).
	realDecls, err := os.ReadFile(filepath.Join(monitorDir, "declarations.yaml"))
	require.NoError(t, err)
	corrupted := strings.Replace(string(realDecls),
		"      classification: irreversible\n      undo: noop",
		"      classification: reversible\n      undo: noop", 1)
	require.NotEqual(t, string(realDecls), corrupted, "expected an irreversible+noop tool to corrupt")

	dir := t.TempDir()
	badDecls := filepath.Join(dir, "declarations.yaml")
	require.NoError(t, os.WriteFile(badDecls, []byte(corrupted), 0o644))
	profile := filepath.Join(dir, "profile.yaml")
	require.NoError(t, os.WriteFile(profile, []byte(fmt.Sprintf(
		"name: badreceipt\nmachine: %s\ntools:\n  - %s\ntool_declarations:\n  - %s\nrest_definitions:\n  - %s\n",
		filepath.Join(monitorDir, "machine.yaml"),
		filepath.Join(monitorDir, "tools.yaml"),
		badDecls,
		filepath.Join(monitorDir, "rest.yaml"))), 0o644))

	clearAgentFlags()
	flagProfile = profile
	flagValidateConfig = true

	_, err = captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "receipt-contract validation failed")
	require.Contains(t, err.Error(), "no receipt-consuming undo")
}

func TestCommandFailureMessageReportsCommandErrorDetail(t *testing.T) {
	t.Parallel()

	message := commandFailureMessage(core.Result{
		CommandName: "load_corpus",
		Signal:      core.CommandError,
		Output:      "load corpus failed: parse SRD docs/specs/software-requirements/srd025-rollback-lifecycle.yaml: yaml: line 54",
	})

	require.Contains(t, message, "load_corpus failed")
	require.Contains(t, message, "srd025-rollback-lifecycle.yaml")
}
