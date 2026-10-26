// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package exec

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestExecBuilder_MissingRequired(t *testing.T) {
	td := catalog.ToolDef{
		Name:   "greet",
		Binary: "echo",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{"type": "string", "flag": "--name"},
			},
			"required": []interface{}{"name"},
		},
	}
	builder := &ExecBuilder{Def: td, Root: "/tmp"}
	res := builder.Build(core.Result{Output: `{"parameters":{}}`}).Execute()
	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "name")
}

func TestExecBuilder_WithDefault(t *testing.T) {
	td := catalog.ToolDef{
		Name:   "list",
		Binary: "echo",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{"type": "string", "positional": true, "default": "."},
			},
		},
	}
	cmd := (&ExecBuilder{Def: td, Root: "/tmp"}).Build(core.Result{Output: `{"parameters":{}}`})
	ec := cmd.(*ExecCmd)
	assert.Equal(t, ".", ec.params["path"])
}

func TestExecCmd_BuildArgs(t *testing.T) {
	t.Parallel()
	def := catalog.ToolDef{
		Name:   "test",
		Binary: "go",
		Args:   []string{"test", "-count=1"},
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"source":      map[string]interface{}{"type": "string", "positional": true, "position": 1},
				"verbose":     map[string]interface{}{"type": "boolean", "flag": "-v", "bool_flag": true, "position": 2},
				"destination": map[string]interface{}{"type": "string", "positional": true, "position": 3},
			},
		},
	}
	cmd := &ExecCmd{def: def, root: "/tmp", params: map[string]string{
		"source": "A", "verbose": "true", "destination": "B",
	}}
	want := []string{"test", "-count=1", "A", "-v", "B"}
	for range 500 {
		assert.Equal(t, want, cmd.buildArgs())
	}
}

func TestExecCmd_BuildArgs_FlagParams(t *testing.T) {
	def := catalog.ToolDef{
		Name:   "create",
		Binary: "tracker",
		Args:   []string{"create", "--json"},
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"title": map[string]interface{}{"type": "string", "flag": "--title"},
				"body":  map[string]interface{}{"type": "string", "flag": "--body"},
			},
		},
	}
	cmd := &ExecCmd{def: def, root: "/tmp", params: map[string]string{"title": "fix bug"}}
	args := cmd.buildArgs()
	assert.Contains(t, args, "--title")
	assert.Contains(t, args, "fix bug")
	assert.NotContains(t, args, "--body")
}

func TestExecCmdUndoWorkspaceRestoreIsHandledByWorkspaceLayer(t *testing.T) {
	cmd := &ExecCmd{def: catalog.ToolDef{Name: "copy_dir", Undo: catalog.ToolUndoContract{Strategy: "workspace_restore"}}}
	res := cmd.Undo(core.Result{})
	require.Equal(t, core.ToolDone, res.Signal)
	assert.Contains(t, res.Output, "workspace restore")
}

func TestExecCmdUndoCompensatingActionReportsGap(t *testing.T) {
	cmd := &ExecCmd{def: catalog.ToolDef{
		Name: "issue_create",
		Undo: catalog.ToolUndoContract{Strategy: "compensating_action", Description: "close created issue"},
	}}
	res := cmd.Undo(core.Result{})
	require.Equal(t, core.CompensationRequired, res.Signal)
	require.NoError(t, res.Err)
	assert.Contains(t, res.Output, "requires compensating action")
}

func TestExecCmdReceiptEncodesWorkspaceRestore(t *testing.T) {
	cmd := &ExecCmd{def: catalog.ToolDef{
		Name: "copy_dir",
		SideEffects: catalog.ToolSideEffects{Items: []catalog.ToolSideEffect{{
			Kind: "filesystem_write", Paths: []string{"out"},
		}}},
		Undo: catalog.ToolUndoContract{Strategy: "workspace_restore"},
	}}
	receipt := cmd.encodeReceipt("")
	assert.Contains(t, receipt, `"strategy":"workspace_restore"`)
	assert.Contains(t, receipt, `"out"`)
}

func TestExecCmdReceiptEncodesBoundaryCompensation(t *testing.T) {
	cmd := &ExecCmd{
		def: catalog.ToolDef{
			Name: "issue_close",
			SideEffects: catalog.ToolSideEffects{Items: []catalog.ToolSideEffect{{
				Kind: "filesystem_write", Paths: []string{".data"},
			}}},
			Undo: catalog.ToolUndoContract{
				Strategy: "compensating_action", Description: "reopen closed issue",
				Payload: "boundary_compensation", Captures: []string{"issue_id"},
				Requires: []string{"issue_id"},
			},
		},
	}
	receipt := cmd.encodeReceipt(`{"issue_id":"agent-core-123"}`)
	assert.Contains(t, receipt, `"strategy":"compensating_action"`)
	assert.Contains(t, receipt, `"captures":{"issue_id":"agent-core-123"}`)
	assert.Contains(t, receipt, `".data"`)
	assert.Contains(t, receipt, `"issue_id"`)
	assert.Contains(t, receipt, `"reopen closed issue"`)
}

// TestExecCmdReceiptEmptyForNoop verifies read-only / no-op tools carry no receipt.
func TestExecCmdReceiptEmptyForNoop(t *testing.T) {
	cmd := &ExecCmd{def: catalog.ToolDef{Name: "list", Undo: catalog.ToolUndoContract{Strategy: "noop"}}}
	assert.Empty(t, cmd.encodeReceipt(""))
}

func TestExecCmdReceiptEmptyForDeclaredIrreversible(t *testing.T) {
	cmd := &ExecCmd{def: catalog.ToolDef{
		Name:          "publish",
		Reversibility: catalog.ToolReversibility{Classification: "irreversible"},
		Undo:          catalog.ToolUndoContract{Strategy: "irreversible"},
	}}
	assert.Empty(t, cmd.encodeReceipt(""))
}

// TestExecCmdUndoConsumesReceiptStrategy verifies a fresh command instance (no
// def strategy) reverses using the strategy carried on the prior Result receipt.
func TestExecCmdUndoConsumesReceiptStrategy(t *testing.T) {
	origin := &ExecCmd{def: catalog.ToolDef{
		Name: "issue_close",
		Undo: catalog.ToolUndoContract{Strategy: "compensating_action", Description: "reopen closed issue"},
	}}
	receipt := origin.encodeReceipt("")

	fresh := &ExecCmd{def: catalog.ToolDef{Name: "issue_close"}}
	res := fresh.Undo(core.Result{Receipt: receipt})
	require.Equal(t, core.CompensationRequired, res.Signal)
	assert.Contains(t, res.Output, "requires compensating action")
	assert.Contains(t, res.Output, "reopen closed issue")
}

func TestExecCmd_Execute_Success(t *testing.T) {
	def := catalog.ToolDef{Name: "greet", Binary: "echo", Args: []string{"hello"}}
	res := (&ExecCmd{def: def, root: "/tmp", params: map[string]string{}}).Execute()
	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Equal(t, "hello", res.Output)
	assert.Equal(t, "greet", res.CommandName)
}

func TestExecCmdRecordsMonitorMetrics(t *testing.T) {
	t.Parallel()
	rec := &execMetricRecorder{}
	cmd := &ExecCmd{
		def:  catalog.ToolDef{Name: "greet", Binary: "echo", Args: []string{"hello"}, Metrics: execMetrics()},
		root: "/tmp",
	}
	cmd.SetMonitorRecorder(rec)

	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal)
	requireExecMetric(t, rec.samples, "exec.output_bytes", 6)
	requireExecMetric(t, rec.samples, "exec.exit_code", 0)
	for _, sample := range rec.samples {
		require.NotContains(t, sample.Attributes, "output")
	}
}

func TestExecCmdSkipsDisabledMonitorMetrics(t *testing.T) {
	t.Parallel()
	rec := &execMetricRecorder{}
	cmd := &ExecCmd{
		def:  catalog.ToolDef{Name: "greet", Binary: "echo", Args: []string{"hello"}, Metrics: core.MetricConfig{Disabled: true}},
		root: "/tmp",
	}
	cmd.SetMonitorRecorder(rec)

	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal)
	require.Empty(t, rec.samples)
}

func TestExecCmdMetricsCarryDispatchEnvelope(t *testing.T) {
	t.Parallel()
	cmd := &ExecCmd{
		def:  catalog.ToolDef{Name: "greet", Binary: "echo", Args: []string{"hello"}, Metrics: execMetrics()},
		root: "/tmp",
	}

	samples := runExecMetricLoop(t, cmd, core.ToolDone)

	requireExecMetric(t, samples, "exec.output_bytes", 6)
	requireExecEnvelope(t, samples, "exec.output_bytes", cmd.Name())
}

type execMetricRecorder struct {
	samples []monitor.MetricSample
}

func (r *execMetricRecorder) RecordMetric(_ context.Context, sample monitor.MetricSample) error {
	r.samples = append(r.samples, sample)
	return nil
}

func requireExecMetric(t *testing.T, samples []monitor.MetricSample, name string, value float64) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name == name && sample.Value == value {
			return
		}
	}
	t.Fatalf("missing metric %s=%v in %#v", name, value, samples)
}

func execMetrics() core.MetricConfig {
	return core.MetricConfig{Instruments: []core.MetricInstrument{
		{Name: "exec.process_duration", Kind: "histogram", Unit: "ms", Description: "Process duration.", ValueSource: "process_duration"},
		{Name: "exec.output_bytes", Kind: "histogram", Unit: "By", Description: "Output bytes.", ValueSource: "output_bytes"},
		{Name: "exec.exit_code", Kind: "gauge", Unit: "1", Description: "Exit code.", ValueSource: "exit_code"},
	}}
}

func TestExecCmd_Execute_Failure(t *testing.T) {
	def := catalog.ToolDef{Name: "fail", Binary: "false"}
	res := (&ExecCmd{def: def, root: "/tmp", params: map[string]string{}}).Execute()
	assert.Equal(t, core.ToolFailed, res.Signal)
}

func TestExecCmd_Execute_WithOutputCap(t *testing.T) {
	def := catalog.ToolDef{Name: "seq", Binary: "seq", Args: []string{"100"}, OutputCap: 5}
	res := (&ExecCmd{def: def, root: "/tmp", params: map[string]string{}}).Execute()
	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Contains(t, res.Output, "omitted")
}

func TestExecCmd_Precondition_GitRepo(t *testing.T) {
	def := catalog.ToolDef{Name: "status", Binary: "git", Args: []string{"status"}, Precondition: "git_repo"}
	res := (&ExecCmd{def: def, root: t.TempDir(), params: map[string]string{}}).Execute()
	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "not a git repository")
}

// A subdirectory of a real repository resolves: the old os.Stat on <dir>/.git
// rejected it, git rev-parse accepts it (GH-1381).
func TestExecCmd_Precondition_GitRepo_Subdirectory(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, RunProcGroup(context.Background(), gitPreconditionTimeout, root, "git", "init").Err)
	sub := filepath.Join(root, "nested", "deeper")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	def := catalog.ToolDef{Name: "rev", Binary: "git", Args: []string{"rev-parse", "--git-dir"}, Precondition: "git_repo"}
	res := (&ExecCmd{def: def, root: sub, params: map[string]string{}}).Execute()
	assert.Equal(t, core.ToolDone, res.Signal, "subdirectory of a repo must pass the git precondition")
}

// An unknown precondition is an error at dispatch, not a silent fall-through to
// the git check (GH-1381).
func TestExecCmd_Precondition_Unknown(t *testing.T) {
	def := catalog.ToolDef{Name: "typo", Binary: "echo", Args: []string{"hi"}, Precondition: "git-repo"}
	res := (&ExecCmd{def: def, root: t.TempDir(), params: map[string]string{}}).Execute()
	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "unknown precondition")
}

func TestRegisterToolDefs(t *testing.T) {
	defs := []catalog.ToolDef{{Name: "greet", Binary: "echo", Description: "Say hello"}}
	reg := core.NewRegistry()
	RegisterToolDefs(reg, "/tmp", defs)
	names := reg.ExternalToolNames()
	assert.Contains(t, names, "greet")
	_, ok := reg.Resolve("greet")
	assert.True(t, ok)
}

// An array parameter maps each element to its own flag occurrence, and a
// lone string to one; a bool_flag maps a JSON true, never false
// (srd023 R2.4, R2.12).
func TestExecBuilderMapsArraysAndBooleans(t *testing.T) {
	td := catalog.ToolDef{
		Name:   "file_issue",
		Binary: "echo",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "flag": "--path", "position": 1},
				"closes": map[string]interface{}{"type": "array", "flag": "--closes", "position": 2},
				"push":   map[string]interface{}{"type": "boolean", "flag": "--push", "bool_flag": true, "position": 3},
				"update": map[string]interface{}{"type": "boolean", "flag": "--update", "bool_flag": true, "position": 4},
			},
		},
	}
	build := func(request string) []string {
		return (&ExecBuilder{Def: td, Root: "/tmp"}).Build(core.Result{Output: request}).(*ExecCmd).buildArgs()
	}
	assert.Equal(t, []string{"--path", "docs/a.md", "--path", "docs/b.md", "--closes", "g/p#3", "--push"},
		build(`{"parameters":{"path":["docs/a.md","docs/b.md"],"closes":"g/p#3","push":true,"update":false}}`))
	assert.Equal(t, []string{"--update"}, build(`{"parameters":{"push":"false","update":"true"}}`))
	assert.Empty(t, build(`{"parameters":{"path":[]}}`))
}
