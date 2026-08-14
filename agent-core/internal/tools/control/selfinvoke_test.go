// Copyright (c) 2026 Nokia. All rights reserved.

package control

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/execute"
)

func TestSelfInvokeUsesSharedExecuteConfigArgs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	builder := &SelfInvokeBuilder{
		Config: execute.Config{
			Binary: "echo", Profile: "agents/executor/profile.yaml",
			CoreRoot: "/checkout/agent-core", Directory: "/workspace",
			OTelDir: dir, Timeout: 5 * time.Second,
		},
		Ctx: context.Background(),
	}

	result := builder.Build(core.Result{Output: `{"parameters":{"run_id":"child-1"}}`}).Execute()

	require.Equal(t, core.ToolDone, result.Signal)
	require.Contains(t, result.Output, "--profile agents/executor/profile.yaml")
	require.Contains(t, result.Output, "--core-root /checkout/agent-core")
	require.Contains(t, result.Output, "--directory /workspace")
	require.Contains(t, result.Output, "--otel-log-file "+dir+"/child-child-1.otel.json")
}

func TestSelfInvokeCancellationAfterChildStartRetainsReceipt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	marker := filepath.Join(dir, "started")
	script := filepath.Join(dir, "child")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
for marker; do :; done
printf started > "$marker"
sleep 30
`), 0o700))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := (&SelfInvokeBuilder{
		ToolName: "launch_evaluator",
		Config: execute.Config{
			Binary: script, Profile: "agents/critic/profile.yaml",
		},
		WorkspacePath: dir,
		ExtraArgs:     []string{marker},
	}).Build(core.Result{Output: `{"parameters":{"run_id":"cancelled-child"}}`})
	resultCh := make(chan core.Result, 1)
	go func() {
		resultCh <- cmd.(core.ContextCommand).ExecuteContext(ctx)
	}()
	require.Eventually(t, func() bool {
		_, err := os.Stat(marker)
		return err == nil
	}, 5*time.Second, 10*time.Millisecond)

	cancel()
	result := <-resultCh

	require.Equal(t, core.ToolFailed, result.Signal)
	require.Equal(t, "launch_evaluator", result.CommandName)
	require.NotEmpty(t, result.Receipt)
}

func TestSelfInvokeResolvesRequestAndOutputFromCommandState(t *testing.T) {
	builder := &SelfInvokeBuilder{
		Config:      execute.Config{Binary: "echo", Profile: "agents/critic/profile.yaml"},
		RequestFrom: "$from(action).suite",
		OutputFrom:  "$from(action).output_dir",
		Ctx:         context.Background(),
	}
	cmd := builder.Build(core.Result{})
	aware := cmd.(core.CommandStateAware)
	aware.SetCommandState(core.NewCommandStateView(core.Execution{{
		CommandName: "await_action",
		Label:       "action",
		Result: core.ResultDigest{
			Output:           `{"suite":"suites/basic.yaml","output_dir":"eval-results"}`,
			RedactionVersion: core.OutputRedactionVersion1,
			RedactionStatus:  core.OutputRedactionApplied,
		},
	}}))

	result := cmd.Execute()

	require.Equal(t, core.ToolDone, result.Signal)
	require.Contains(t, result.Output, "--request suites/basic.yaml")
	require.Contains(t, result.Output, "--output eval-results")
}

func TestSelfInvokeTraceRecordsBoundedChildResult(t *testing.T) {
	recorder := &recordingTracer{}
	cmd := &selfInvokeCmd{
		runID: strings.Repeat("r", selfInvokeTraceAttributeLimit+10), tracer: recorder,
	}
	child := &execute.Result{
		ExitCode: 2,
		Stdout:   strings.Repeat("o", selfInvokeTraceAttributeLimit+20),
		Stderr:   strings.Repeat("e", selfInvokeTraceAttributeLimit+30),
	}
	profile := strings.Repeat("p", selfInvokeTraceAttributeLimit+15)

	cmd.traceResult(execute.Config{Binary: "agent", Profile: profile}, child)

	require.Equal(t, "self_invoke.result", recorder.eventName)
	for _, attrs := range [][]attribute.KeyValue{recorder.attributes, recorder.eventAttrs} {
		values := attributeValues(attrs)
		require.Equal(t, strings.Repeat("p", selfInvokeTraceAttributeLimit)+"... (15 more bytes)",
			values["self_invoke.profile"].AsString())
		require.Equal(t, strings.Repeat("r", selfInvokeTraceAttributeLimit)+"... (10 more bytes)",
			values["self_invoke.run_id"].AsString())
		require.Equal(t, int64(2), values["self_invoke.exit_code"].AsInt64())
		require.Equal(t, strings.Repeat("o", selfInvokeTraceAttributeLimit)+"... (20 more bytes)",
			values["self_invoke.output"].AsString())
		require.Equal(t, strings.Repeat("o", selfInvokeTraceAttributeLimit)+"... (20 more bytes)",
			values["self_invoke.stdout"].AsString())
		require.Equal(t, strings.Repeat("e", selfInvokeTraceAttributeLimit)+"... (30 more bytes)",
			values["self_invoke.stderr"].AsString())
	}
}

type recordingTracer struct {
	attributes []attribute.KeyValue
	eventName  string
	eventAttrs []attribute.KeyValue
}

func (r *recordingTracer) Push(string, ...attribute.KeyValue) (tracing.Tracer, func()) {
	return r, func() {}
}

func (r *recordingTracer) Event(name string, attrs ...attribute.KeyValue) {
	r.eventName = name
	r.eventAttrs = append([]attribute.KeyValue(nil), attrs...)
}

func (r *recordingTracer) SetAttributes(attrs ...attribute.KeyValue) {
	r.attributes = append([]attribute.KeyValue(nil), attrs...)
}

func (*recordingTracer) RecordError(error) {}

func (*recordingTracer) Context() context.Context { return context.Background() }

func attributeValues(attrs []attribute.KeyValue) map[string]attribute.Value {
	values := make(map[string]attribute.Value, len(attrs))
	for _, attr := range attrs {
		values[string(attr.Key)] = attr.Value
	}
	return values
}
