// Copyright (c) 2026 Nokia. All rights reserved.

package exec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
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
	def := catalog.ToolDef{
		Name:   "test",
		Binary: "go",
		Args:   []string{"test", "-count=1"},
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"package": map[string]interface{}{"type": "string", "positional": true},
				"verbose": map[string]interface{}{"type": "boolean", "flag": "-v", "bool_flag": true},
			},
		},
	}
	cmd := &ExecCmd{def: def, root: "/tmp", params: map[string]string{"package": "./pkg/...", "verbose": "true"}}
	args := cmd.buildArgs()
	assert.Contains(t, args, "test")
	assert.Contains(t, args, "-count=1")
	assert.Contains(t, args, "./pkg/...")
	assert.Contains(t, args, "-v")
}

func TestExecCmd_BuildArgs_FlagParams(t *testing.T) {
	def := catalog.ToolDef{
		Name:   "create",
		Binary: "bd",
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
	res := cmd.Undo()
	require.Equal(t, core.ToolDone, res.Signal)
	assert.Contains(t, res.Output, "workspace restore")
}

func TestExecCmdUndoCompensatingActionReportsGap(t *testing.T) {
	cmd := &ExecCmd{def: catalog.ToolDef{
		Name: "issue_create",
		Undo: catalog.ToolUndoContract{Strategy: "compensating_action", Description: "close created issue"},
	}}
	res := cmd.Undo()
	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	assert.Contains(t, res.Output, "requires compensating action")
}

func TestExecCmdUndoMementoUsesDeclaredStrategy(t *testing.T) {
	cmd := &ExecCmd{def: catalog.ToolDef{
		Name: "copy_dir",
		SideEffects: catalog.ToolSideEffects{Items: []catalog.ToolSideEffect{{
			Kind: "filesystem_write", Paths: []string{"out"},
		}}},
		Undo: catalog.ToolUndoContract{Strategy: "workspace_restore"},
	}}
	memento, err := cmd.UndoMemento()
	require.NoError(t, err)
	require.Equal(t, core.UndoMementoReversible, memento.Kind)
	require.NoError(t, core.ValidateUndoMemento(memento))
	assert.Contains(t, string(memento.Payload), `"out"`)
}

func TestExecCmdUndoMementoUsesBoundaryCompensationPayload(t *testing.T) {
	cmd := &ExecCmd{
		def: catalog.ToolDef{
			Name: "issue_close",
			SideEffects: catalog.ToolSideEffects{Items: []catalog.ToolSideEffect{{
				Kind: "filesystem_write", Paths: []string{".beads"},
			}}},
			Undo: catalog.ToolUndoContract{
				Strategy: "compensating_action", Description: "reopen closed issue",
				Payload: "boundary_compensation", Requires: []string{"issue_id"},
			},
		},
		params: map[string]string{"id": "agent-core-123"},
	}
	memento, err := cmd.UndoMemento()
	require.NoError(t, err)
	require.Equal(t, core.UndoMementoCompensatable, memento.Kind)
	require.NoError(t, core.ValidateUndoMemento(memento))
	assert.Contains(t, string(memento.Payload), `"boundary_compensation"`)
	assert.Contains(t, string(memento.Payload), `"issue_id":"agent-core-123"`)
	assert.Contains(t, string(memento.Payload), `".beads"`)
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
	cmd := &ExecCmd{def: catalog.ToolDef{Name: "greet", Binary: "echo", Args: []string{"hello"}}, root: "/tmp"}
	cmd.SetMonitorRecorder(rec)

	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal)
	requireExecMetric(t, rec.samples, "exec.output_bytes", 6)
	requireExecMetric(t, rec.samples, "exec.exit_code", 0)
	for _, sample := range rec.samples {
		require.Equal(t, "echo", sample.Attributes["binary"])
		require.NotContains(t, sample.Attributes, "output")
	}
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

func TestRegisterToolDefs(t *testing.T) {
	defs := []catalog.ToolDef{{Name: "greet", Binary: "echo", Description: "Say hello"}}
	reg := core.NewRegistry()
	RegisterToolDefs(reg, "/tmp", defs)
	names := reg.ExternalToolNames()
	assert.Contains(t, names, "greet")
	_, ok := reg.Resolve("greet")
	assert.True(t, ok)
}
