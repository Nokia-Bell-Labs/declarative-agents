// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package exec

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestExecEnvOverlaysResolvedParamsOntoInheritedEnvironment(t *testing.T) {
	t.Setenv("GH1884_PARENT", "from-parent")
	cmd := envCommand("run_corpus_ingest", []string{
		"GH1884_CHILD={{ params.collection }}",
	}, map[string]string{"collection": "wiki"})

	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal, res.Output)
	require.Equal(t, "from-parent|wiki", res.Output)
}

func TestExecEnvOnlyParamDoesNotReachArgv(t *testing.T) {
	cmd := &ExecCmd{
		def: catalog.ToolDef{
			Name:   "run_corpus_ingest",
			Binary: "sh",
			Args:   []string{"-c", `printf '%s|%s' "$#" "$GH1884_CHILD"`},
			Env:    []string{"GH1884_CHILD={{ params.collection }}"},
			Parameters: map[string]interface{}{
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string"},
				},
			},
		},
		params: map[string]string{"collection": "wiki"},
	}

	require.Equal(t, cmd.def.Args, cmd.buildArgs())
	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal, res.Output)
	require.Equal(t, "0|wiki", res.Output)
}

func TestExecEnvDeclaredKeyOverlaysParent(t *testing.T) {
	t.Setenv("GH1884_PARENT", "from-parent")
	t.Setenv("GH1884_CHILD", "parent-child")
	cmd := envCommand("run_corpus_ingest", []string{
		"GH1884_CHILD=declared",
	}, nil)

	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal, res.Output)
	require.Equal(t, "from-parent|declared", res.Output)
}

func TestExecEnvExpandsCommandStateSourcedParam(t *testing.T) {
	td := catalog.ToolDef{
		Name:   "run_corpus_ingest",
		Binary: "sh",
		Args:   []string{"-c", `printf %s "$GH1884_PARENT|$GH1884_CHILD"`},
		Env:    []string{"GH1884_CHILD={{ params.collection }}"},
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"collection": map[string]interface{}{
					"type": "string", "source": "$from(seed).parameters.collection",
				},
			},
			"required": []interface{}{"collection"},
		},
	}
	t.Setenv("GH1884_PARENT", "from-parent")
	cmd := (&ExecBuilder{Def: td, Root: t.TempDir()}).Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(core.NewCommandStateView(core.Execution{{
		CommandName: "seed",
		Label:       "seed",
		Result: core.ResultDigest{
			Output:           `{"parameters":{"collection":"wiki"}}`,
			RedactionVersion: core.OutputRedactionVersion1,
			RedactionStatus:  core.OutputRedactionApplied,
		},
	}}))

	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal, res.Output)
	require.Equal(t, "from-parent|wiki", res.Output)
}

func TestExecEnvFailuresPreventLaunch(t *testing.T) {
	tests := []struct {
		name   string
		env    []string
		params map[string]string
		want   string
	}{
		{name: "missing param", env: []string{"FOO={{ params.collection }}"}, want: "parameter \"collection\" is missing"},
		{name: "empty param", env: []string{"FOO={{ params.collection }}"}, params: map[string]string{"collection": ""}, want: "empty string"},
		{name: "malformed entry", env: []string{"NOTKEYVALUE"}, want: "must be KEY=VALUE"},
		{name: "unknown token", env: []string{"FOO={{params.collection}}"}, params: map[string]string{"collection": "wiki"}, want: "unknown env template token"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			marker := t.TempDir() + "/launched"
			cmd := &ExecCmd{
				def: catalog.ToolDef{
					Name: "consume", Binary: "touch", Args: []string{marker}, Env: tc.env,
				},
				params: tc.params,
			}

			res := cmd.Execute()

			require.Equal(t, core.CommandError, res.Signal)
			require.ErrorContains(t, res.Err, tc.want)
			_, err := os.Stat(marker)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestExecEnvOmitsValuesFromTelemetryAndReceipt(t *testing.T) {
	secret := "env-secret-value"
	cmd := &ExecCmd{
		def: catalog.ToolDef{
			Name:   "consume",
			Binary: "true",
			Env:    []string{"GH1884_SECRET={{ params.token }}"},
			Undo:   catalog.ToolUndoContract{Strategy: "compensating_action", Description: "undo env consumer"},
		},
		params: map[string]string{"token": secret},
	}
	rec := &execRuntimeRecorder{}
	observer := &execExecutionObserver{}
	tracer := tracing.NewRecordingTracer()

	run, err := core.Loop(core.LoopParams{
		InitialState: "Start",
		Registry:     core.NewRegistry(),
		Table: core.TransitionTable{
			{State: "Start", Signal: core.Seed}: {
				NextState: "Running",
				Action:    func(core.Result) core.Command { return cmd },
			},
			{State: "Running", Signal: core.ToolDone}: {NextState: "Done"},
		},
		IsTerminal:           func(state core.State) bool { return state == "Done" },
		Trace:                tracer,
		Budget:               core.Budget{MaxIterations: 10},
		CommandTimeout:       time.Second,
		MonitorRecorder:      rec,
		CommandStateObserver: observer,
		Hooks: core.LoopHooks{
			TerminalStatus: func(core.State) core.RunStatus { return core.StatusSucceeded },
		},
	}, context.Background())

	require.NoError(t, err)
	require.Equal(t, core.StatusSucceeded, run.Status)
	require.Len(t, observer.execution, 1)
	executed := observer.execution[0]
	require.Equal(t, core.ToolDone, executed.Result.Signal)
	require.NotContains(t, executed.Receipt, secret)
	telemetry, marshalErr := json.Marshal(struct {
		Spans   any
		Metrics any
	}{Spans: tracer.Spans, Metrics: rec.samples})
	require.NoError(t, marshalErr)
	require.NotContains(t, string(telemetry), secret)
}

func TestExecEnvAbsentKeepsArgvOnlyBehavior(t *testing.T) {
	cmd := &ExecCmd{
		def:    catalog.ToolDef{Name: "echo_hi", Binary: "printf", Args: []string{"hi"}},
		params: map[string]string{"unused": "value"},
	}

	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal, res.Output)
	require.Equal(t, "hi", res.Output)
}

func envCommand(name string, env []string, params map[string]string) *ExecCmd {
	return &ExecCmd{
		def: catalog.ToolDef{
			Name:   name,
			Binary: "sh",
			Args:   []string{"-c", `printf %s "$GH1884_PARENT|$GH1884_CHILD"`},
			Env:    env,
		},
		params: params,
	}
}
