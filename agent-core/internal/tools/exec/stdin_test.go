// Copyright (c) 2026 Nokia. All rights reserved.

package exec

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestExecStdinSourceFeedsSeparateBinary(t *testing.T) {
	producer := &ExecCmd{def: catalog.ToolDef{
		Name: "produce_words", Binary: "printf", Args: []string{"alpha beta"},
		Output: catalog.ToolOutputContract{Mode: "structured"},
	}}
	produced := producer.Execute()
	require.Equal(t, core.ToolDone, produced.Signal)
	require.JSONEq(t, `{"output":"alpha beta","exit_code":0}`, produced.Output)

	consumer := stdinCommand("count_words", "wc", []string{"-w"}, "$from(produce).output")
	consumer.SetCommandState(viewForResult("produce", produced, "receipt-must-not-flow"))
	consumed := consumer.Execute()

	require.Equal(t, core.ToolDone, consumed.Signal, consumed.Output)
	require.Equal(t, "2", strings.TrimSpace(consumed.Output))
}

func TestExecStdinSourceFailuresPreventLaunch(t *testing.T) {
	tests := []struct {
		name   string
		source string
		output string
		limit  int
		want   string
	}{
		{name: "missing label", source: "$from(absent).output", output: `{"output":"safe"}`, want: "no prior step"},
		{name: "missing path", source: "$from(produce).missing", output: `{"output":"safe"}`, want: "path"},
		{name: "non string", source: "$from(produce).output", output: `{"output":7}`, want: "want string"},
		{name: "empty", source: "$from(produce).output", output: `{"output":""}`, want: "empty string"},
		{name: "oversized", source: "$from(produce).output", output: `{"output":"12345"}`, limit: 4, want: "exceeds limit 4"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			marker := t.TempDir() + "/launched"
			cmd := stdinCommand("consume", "touch", []string{marker}, tc.source)
			cmd.def.StdinMaxBytes = tc.limit
			cmd.SetCommandState(viewForOutput("produce", tc.output, "hidden-receipt"))

			res := cmd.Execute()

			require.Equal(t, core.CommandError, res.Signal)
			require.ErrorContains(t, res.Err, tc.want)
			_, err := os.Stat(marker)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestExecStdinReceiptIsUnaddressable(t *testing.T) {
	marker := t.TempDir() + "/launched"
	cmd := stdinCommand("consume", "touch", []string{marker}, "$from(produce).receipt")
	cmd.SetCommandState(viewForOutput("produce", `{"output":"safe"}`, "receipt-secret"))

	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.NotContains(t, res.Output, "receipt-secret")
	_, err := os.Stat(marker)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestExecStdinPersistsRedactedInputWithoutTelemetryOrReceiptLeak(t *testing.T) {
	secret := "stdin-secret-value"
	entry := core.Entry{
		CommandName: "produce",
		Label:       "produce",
		Result: core.DigestResult(core.Result{
			Output: `{"output":"safe input","secret":"` + secret + `"}`,
			Signal: core.ToolDone,
			Redaction: core.OutputRedaction{
				Version: core.OutputRedactionVersion1,
				Paths:   []core.OutputRedactionPath{{"secret"}},
			},
		}),
		Receipt: `{"opaque":"receipt-secret"}`,
	}
	checkpoint := &core.InMemoryCheckpoint{}
	require.NoError(t, checkpoint.Save(core.Position{}, core.Execution{entry}))
	_, restored, err := checkpoint.Load()
	require.NoError(t, err)
	require.NotContains(t, restored[0].Result.Output, secret)

	cmd := stdinCommand("consume", "cat", nil, "$from(produce).output")
	cmd.def.Undo = catalog.ToolUndoContract{Strategy: "compensating_action", Description: "undo consumer"}
	rec := &execRuntimeRecorder{}
	observer := &execExecutionObserver{}
	tracer := tracing.NewRecordingTracer()

	run, err := core.Loop(core.LoopParams{
		InitialState:     "Start",
		InitialExecution: restored,
		Registry:         core.NewRegistry(),
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
	}, context.Background())

	require.NoError(t, err)
	require.Equal(t, core.StatusSucceeded, run.Status)
	require.Len(t, observer.execution, 2)
	executed := observer.execution[1]
	require.Equal(t, core.ToolDone, executed.Result.Signal)
	require.Equal(t, "safe input", executed.Result.Output)
	require.NotContains(t, executed.Receipt, secret)
	require.NotContains(t, executed.Receipt, "safe input")
	telemetry, marshalErr := json.Marshal(struct {
		Spans   any
		Metrics any
	}{Spans: tracer.Spans, Metrics: rec.samples})
	require.NoError(t, marshalErr)
	require.NotContains(t, string(telemetry), secret)
	require.NotContains(t, string(telemetry), "safe input")
}

func TestExecStdinCancellationClosesAndJoinsProcess(t *testing.T) {
	input := strings.Repeat("x", 256<<10)
	cmd := stdinCommand("blocked_consumer", "sleep", []string{"30"}, "$from(produce).output")
	cmd.SetCommandState(viewForOutput("produce", `{"output":"`+input+`"}`, ""))
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)

	start := time.Now()
	res := core.SafeExecuteContext(ctx, cmd, 0)

	require.Equal(t, core.CommandError, res.Signal)
	require.ErrorIs(t, res.Err, context.Canceled)
	require.Less(t, time.Since(start), 2*time.Second)
}

func stdinCommand(name, binary string, args []string, source string) *ExecCmd {
	return &ExecCmd{def: catalog.ToolDef{
		Name: name, Binary: binary, Args: args, StdinSource: source,
	}}
}

func viewForOutput(label, output, receipt string) core.CommandStateView {
	result := core.Result{Output: output, Signal: core.ToolDone}
	return viewForResult(label, result, receipt)
}

func viewForResult(label string, result core.Result, receipt string) core.CommandStateView {
	return core.NewCommandStateView(core.Execution{{
		CommandName: label,
		Label:       label,
		Result:      core.DigestResult(result),
		Receipt:     receipt,
	}})
}

type execRuntimeRecorder struct {
	samples []monitor.MetricSample
}

func (r *execRuntimeRecorder) RecordMetric(_ context.Context, sample monitor.MetricSample) error {
	r.samples = append(r.samples, sample)
	return nil
}

func (*execRuntimeRecorder) RecordEvent(context.Context, monitor.RunEvent) error {
	return nil
}

func (*execRuntimeRecorder) RecordRun(context.Context, monitor.RunSnapshot) error {
	return nil
}

type execExecutionObserver struct {
	execution core.Execution
}

func (o *execExecutionObserver) ObserveCommandState(execution core.Execution) {
	o.execution = append(core.Execution(nil), execution...)
}
