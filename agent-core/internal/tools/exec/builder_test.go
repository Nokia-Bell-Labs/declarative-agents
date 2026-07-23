// Copyright (c) 2026 Nokia. All rights reserved.

package exec

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
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

func TestExecBuilderCommandStateSourceAcrossInterveningStep(t *testing.T) {
	td := catalog.ToolDef{
		Name:   "run_corpus_ingest",
		Binary: "printf",
		Args:   []string{"%s|%s"},
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"directory": map[string]interface{}{
					"type": "string", "positional": true, "position": 1,
					"source": "$from(seed).parameters.directory",
				},
				"mode": map[string]interface{}{
					"type": "string", "positional": true, "position": 2,
				},
			},
			"required": []interface{}{"directory", "mode"},
		},
	}

	mappings := td.ExtractParamMappings()
	directory := findParamMapping(t, mappings, "directory")
	require.Equal(t, "$from(seed).parameters.directory", directory.Source)

	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(td.ToToolSpec().InputSchema, &schema))
	properties := schema["properties"].(map[string]interface{})
	require.NotContains(t, properties["directory"].(map[string]interface{}), "source")

	cmd := (&ExecBuilder{Def: td, Root: t.TempDir()}).Build(core.Result{
		Output: `{"parameters":{"mode":"full"}}`,
	})
	aware, ok := cmd.(core.CommandStateAware)
	require.True(t, ok)
	aware.SetCommandState(core.NewCommandStateView(core.Execution{
		{
			CommandName: "seed",
			Label:       "seed",
			Result:      core.ResultDigest{Output: `{"parameters":{"directory":"/tmp/corpus"}}`},
		},
		{
			CommandName: "count_before",
			Label:       "count_before",
			Result:      core.ResultDigest{Output: `{"mapped":{"count":"4"}}`},
		},
	}))

	result := cmd.Execute()

	require.Equal(t, core.ToolDone, result.Signal, result.Output)
	require.Equal(t, "/tmp/corpus|full", result.Output)
}

func TestExecBuilderCommandStateSourceFailures(t *testing.T) {
	t.Run("missing label preserves typed cause and does not launch", func(t *testing.T) {
		marker := t.TempDir() + "/launched"
		cmd := sourceTouchBuilder(marker, "$from(absent).parameters.directory").Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(core.NewCommandStateView(nil))

		result := cmd.Execute()

		require.Equal(t, core.CommandError, result.Signal)
		var target *core.UnresolvedLabelError
		require.ErrorAs(t, result.Err, &target)
		require.Equal(t, "absent", target.Label)
		_, err := os.Stat(marker)
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("missing path preserves typed cause and does not launch", func(t *testing.T) {
		marker := t.TempDir() + "/launched"
		cmd := sourceTouchBuilder(marker, "$from(seed).parameters.absent").Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(core.NewCommandStateView(core.Execution{{
			CommandName: "seed",
			Label:       "seed",
			Result:      core.ResultDigest{Output: `{"parameters":{"directory":"/tmp/corpus"}}`},
		}}))

		result := cmd.Execute()

		require.Equal(t, core.CommandError, result.Signal)
		var target *core.UnresolvedPathError
		require.ErrorAs(t, result.Err, &target)
		require.Equal(t, "parameters.absent", target.Path)
		_, err := os.Stat(marker)
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("malformed source fails declaration parsing", func(t *testing.T) {
		_, err := catalog.ParseToolDefs([]byte(`
tools:
  - name: run_corpus_ingest
    type: exec
    binary: "true"
    parameters:
      type: object
      properties:
        directory:
          type: string
          source: $from(seed
`))
		require.ErrorContains(t, err, `tool "run_corpus_ingest" parameter "directory"`)
		require.ErrorContains(t, err, "must be a $from(label).path selector")
	})
}

func sourceTouchBuilder(marker, source string) *ExecBuilder {
	return &ExecBuilder{Def: catalog.ToolDef{
		Name:   "source_touch",
		Binary: "touch",
		Args:   []string{marker},
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"directory": map[string]interface{}{
					"type": "string", "flag": "--directory", "source": source,
				},
			},
			"required": []interface{}{"directory"},
		},
	}}
}

func findParamMapping(t *testing.T, mappings []catalog.ParamMapping, name string) catalog.ParamMapping {
	t.Helper()
	for _, mapping := range mappings {
		if mapping.Name == name {
			return mapping
		}
	}
	t.Fatalf("parameter mapping %q not found", name)
	return catalog.ParamMapping{}
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
	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
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
	receipt := cmd.encodeReceipt()
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
				Payload: "boundary_compensation", Requires: []string{"issue_id"},
			},
		},
		params: map[string]string{"id": "agent-core-123"},
	}
	receipt := cmd.encodeReceipt()
	assert.Contains(t, receipt, `"strategy":"compensating_action"`)
	assert.Contains(t, receipt, `"issue_id":"agent-core-123"`)
	assert.Contains(t, receipt, `".data"`)
	assert.Contains(t, receipt, `"issue_id"`)
	assert.Contains(t, receipt, `"reopen closed issue"`)
}

// TestExecCmdReceiptEmptyForNoop verifies read-only / no-op tools carry no receipt.
func TestExecCmdReceiptEmptyForNoop(t *testing.T) {
	cmd := &ExecCmd{def: catalog.ToolDef{Name: "list", Undo: catalog.ToolUndoContract{Strategy: "noop"}}}
	assert.Empty(t, cmd.encodeReceipt())
}

// TestExecCmdUndoConsumesReceiptStrategy verifies a fresh command instance (no
// def strategy) reverses using the strategy carried on the prior Result receipt.
func TestExecCmdUndoConsumesReceiptStrategy(t *testing.T) {
	origin := &ExecCmd{def: catalog.ToolDef{
		Name: "issue_close",
		Undo: catalog.ToolUndoContract{Strategy: "compensating_action", Description: "reopen closed issue"},
	}}
	receipt := origin.encodeReceipt()

	fresh := &ExecCmd{def: catalog.ToolDef{Name: "issue_close"}}
	res := fresh.Undo(core.Result{Receipt: receipt})
	require.Equal(t, core.CommandError, res.Signal)
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

func runExecMetricLoop(t *testing.T, cmd core.Command, signal core.Signal) []monitor.MetricSample {
	t.Helper()
	// Keep this fixture package-local so exec assertions name exec commands and signals.
	store := monitor.NewStore(monitor.Limits{Samples: 10})
	params := execMetricLoopParams(cmd, signal, monitor.NewRecorder(store, nil))
	_, err := core.Loop(params, context.Background())
	require.NoError(t, err)
	return store.Snapshot().RecentSamples
}

func execMetricLoopParams(cmd core.Command, signal core.Signal, rec monitor.RuntimeRecorder) core.LoopParams {
	spec := &core.MachineSpec{
		Name:           "exec-metrics",
		InitialState:   "Start",
		MetricLabels:   core.MetricLabels{"use_case": "rel04.0-monitor"},
		States:         core.StateSpecsFromNames("Start", "Working", "Done"),
		TerminalStates: []string{"Done"},
		Signals:        core.SignalSpecsFromNames(string(core.Seed), string(signal)),
		Transitions: []core.TransitionSpec{
			{State: "Start", Signal: string(core.Seed), Next: "Working", Action: cmd.Name(), MetricLabels: core.MetricLabels{"phase": "dispatch"}},
			{State: "Working", Signal: string(signal), Next: "Done"},
		},
	}
	return core.LoopParams{
		MachineSpec:     spec,
		AgentName:       "exec-run",
		Trace:           tracing.NoopTracer{},
		Budget:          core.Budget{MaxIterations: 3},
		MonitorRecorder: rec,
		InitFunc: func(reg *core.Registry) error {
			reg.Register(core.ToolSpec{Name: cmd.Name(), Visibility: core.Internal}, execMetricBuilder{cmd: cmd})
			return nil
		},
		Hooks: core.LoopHooks{TerminalStatus: func(core.State) core.RunStatus { return core.StatusSucceeded }},
	}
}

type execMetricBuilder struct {
	cmd core.Command
}

func (b execMetricBuilder) Build(core.Result) core.Command {
	return b.cmd
}

func requireExecEnvelope(t *testing.T, samples []monitor.MetricSample, name string, toolName string) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name != name {
			continue
		}
		require.Equal(t, toolName, sample.ToolName)
		require.Equal(t, "exec-run", sample.RunID)
		require.Equal(t, "Working", sample.State)
		require.Equal(t, string(core.ToolDone), sample.Signal)
		require.Equal(t, "success", sample.Status)
		require.Equal(t, "rel04.0-monitor", sample.Attributes["use_case"])
		require.Equal(t, "dispatch", sample.Attributes["phase"])
		return
	}
	t.Fatalf("missing metric %s in %#v", name, samples)
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

func TestRegisterToolDefs(t *testing.T) {
	defs := []catalog.ToolDef{{Name: "greet", Binary: "echo", Description: "Say hello"}}
	reg := core.NewRegistry()
	RegisterToolDefs(reg, "/tmp", defs)
	names := reg.ExternalToolNames()
	assert.Contains(t, names, "greet")
	_, ok := reg.Resolve("greet")
	assert.True(t, ok)
}
