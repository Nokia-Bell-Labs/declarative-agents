// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

type topologySpan struct {
	name   string
	parent *topologySpan
	attrs  map[string]any
}

type topologyTracer struct {
	current *topologySpan
	spans   *[]*topologySpan
}

func newTopologyTracer() *topologyTracer {
	spans := []*topologySpan{}
	return &topologyTracer{spans: &spans}
}

func (t *topologyTracer) Push(
	name string,
	attrs ...attribute.KeyValue,
) (tracing.Tracer, func()) {
	span := &topologySpan{name: name, parent: t.current, attrs: map[string]any{}}
	setTopologyAttrs(span.attrs, attrs)
	*t.spans = append(*t.spans, span)
	return &topologyTracer{current: span, spans: t.spans}, func() {}
}

func (t *topologyTracer) Event(string, ...attribute.KeyValue) {}

func (t *topologyTracer) SetAttributes(attrs ...attribute.KeyValue) {
	if t.current != nil {
		setTopologyAttrs(t.current.attrs, attrs)
	}
}

func (t *topologyTracer) RecordError(error)        {}
func (t *topologyTracer) Context() context.Context { return context.Background() }

func setTopologyAttrs(target map[string]any, attrs []attribute.KeyValue) {
	for _, attr := range attrs {
		target[string(attr.Key)] = attr.Value.AsInterface()
	}
}

func TestLoopTraceShapeIsRunWithDirectToolChildren(t *testing.T) {
	tracer := newTopologyTracer()
	params := simpleLoopParams(tracer)
	params.AgentName = "trace-shape"
	params.RunID = "run-123"

	result, err := Loop(params, context.Background())
	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, result.Status)
	require.Len(t, *tracer.spans, 3, "run plus two dispatched tool spans")

	run := (*tracer.spans)[0]
	require.Equal(t, "invoke_agent trace-shape", run.name)
	require.Nil(t, run.parent)
	require.EqualValues(t, 2, run.attrs["run.iterations"])
	require.EqualValues(t, 2, run.attrs["iteration"],
		"the final iteration is an attribute on the run span")
	require.Equal(t, "TaskCompleted", run.attrs["signal"])
	for _, attributeName := range []string{
		"run.duration_ms", "gen_ai.usage.input_tokens", "gen_ai.usage.output_tokens",
	} {
		require.Contains(t, run.attrs, attributeName)
	}

	for index, wantName := range []string{
		"execute_tool step_a", "execute_tool step_b",
	} {
		tool := (*tracer.spans)[index+1]
		require.Equal(t, wantName, tool.name)
		require.Same(t, run, tool.parent)
		require.NotEmpty(t, tool.attrs["command.name"])
		require.NotEmpty(t, tool.attrs["command.signal"])
		require.Contains(t, tool.attrs, "command.duration_ms")
		require.Contains(t, tool.attrs, "gen_ai.usage.input_tokens")
		require.Contains(t, tool.attrs, "gen_ai.usage.output_tokens")
	}
	for _, span := range *tracer.spans {
		require.NotEqual(t, "invoke_agent", span.name,
			"the loop creates no per-iteration invoke_agent child spans")
	}
}

var _ tracing.Tracer = (*topologyTracer)(nil)
