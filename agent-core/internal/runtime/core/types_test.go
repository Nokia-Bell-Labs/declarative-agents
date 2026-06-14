// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
)

type spanOverrideCmd struct{}

func (spanOverrideCmd) Name() string    { return "override_cmd" }
func (spanOverrideCmd) Execute() Result { return Result{Signal: ToolDone} }
func (spanOverrideCmd) Undo() Result    { return NoopUndo("override_cmd") }

func (spanOverrideCmd) SpanName() string { return "custom-span" }

func (spanOverrideCmd) SpanCreationAttrs() []attribute.KeyValue {
	return []attribute.KeyValue{attribute.String("custom.attr", "value")}
}

type spanRecorder struct {
	name  string
	attrs map[string]string
}

func (r *spanRecorder) Push(name string, attrs ...attribute.KeyValue) (tracing.Tracer, func()) {
	r.name = name
	r.attrs = make(map[string]string, len(attrs))
	for _, attr := range attrs {
		r.attrs[string(attr.Key)] = attr.Value.AsString()
	}
	return r, func() {}
}

func (r *spanRecorder) Event(_ string, _ ...attribute.KeyValue) {}
func (r *spanRecorder) SetAttributes(_ ...attribute.KeyValue)   {}
func (r *spanRecorder) RecordError(_ error)                     {}
func (r *spanRecorder) Context() context.Context                { return context.Background() }
func TestSpanOverrideCustomizesDispatchSpan(t *testing.T) {
	t.Parallel()
	tr := &spanRecorder{}

	res := Dispatch(spanOverrideCmd{}, tr, 0)

	require.Equal(t, ToolDone, res.Signal)
	require.Equal(t, "override_cmd", res.CommandName)
	require.Equal(t, "custom-span", tr.name)
	require.Equal(t, "value", tr.attrs["custom.attr"])
}

var _ SpanOverride = spanOverrideCmd{}
var _ tracing.Tracer = (*spanRecorder)(nil)

func TestToolMetricsJSONTagsAndOmitEmptyDetails(t *testing.T) {
	t.Parallel()

	withDetails, err := json.Marshal(ToolMetrics{
		Total:  2,
		Passed: 1,
		Failed: 1,
		Details: map[string]any{
			"failed_tests": []string{"TestA"},
		},
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"total":2,"passed":1,"failed":1,"details":{"failed_tests":["TestA"]}}`, string(withDetails))

	withoutDetails, err := json.Marshal(ToolMetrics{Total: 1, Passed: 1})
	require.NoError(t, err)
	require.JSONEq(t, `{"total":1,"passed":1,"failed":0}`, string(withoutDetails))
}
