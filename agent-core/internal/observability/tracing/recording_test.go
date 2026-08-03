// Copyright (c) 2026 Nokia. All rights reserved.

package tracing

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

func TestRecordingTracerEventOnRootIsVisible(t *testing.T) {
	t.Parallel()
	tr := NewRecordingTracer()
	tr.Event("root.event", attribute.String("k", "v"), attribute.Int("n", 3))

	require.Len(t, tr.Events, 1)
	event := tr.FindEvent("root.event")
	require.NotNil(t, event)
	require.Equal(t, "v", event.Attrs["k"])
	require.Equal(t, int64(3), event.Attrs["n"])
}

// TestRecordingTracerChildEventVisibleOnRoot is the GH-1358 regression guard:
// an event emitted through the child scope Push returns must be observable on
// the root recorder the test holds.
func TestRecordingTracerChildEventVisibleOnRoot(t *testing.T) {
	t.Parallel()
	tr := NewRecordingTracer()
	child, done := tr.Push("child.span")
	defer done()

	child.Event("child.event", attribute.String("origin", "child"))

	require.NotNil(t, tr.FindEvent("child.event"), "child event must reach the root recorder")
	require.Equal(t, "child", tr.FindEvent("child.event").Attrs["origin"])
	require.Nil(t, tr.FindEvent("missing"))
}

// TestRecordingTracerNestedSpansAndCompletion proves nested Push scopes all
// append to the same recorder, target their own span for attributes/errors,
// and mark completion on the correct span.
func TestRecordingTracerNestedSpansAndCompletion(t *testing.T) {
	t.Parallel()
	tr := NewRecordingTracer()

	outer, doneOuter := tr.Push("outer", attribute.String("level", "0"))
	inner, doneInner := outer.Push("inner", attribute.String("level", "1"))

	outer.SetAttributes(attribute.Int("outer.attr", 10))
	inner.SetAttributes(attribute.Int("inner.attr", 20))
	inner.RecordError(errors.New("boom"))

	require.Len(t, tr.Spans, 2, "both nested spans recorded on the root")
	require.Equal(t, "outer", tr.Spans[0].Name)
	require.Equal(t, "inner", tr.Spans[1].Name)
	require.Equal(t, "0", tr.Spans[0].Attrs["level"])
	require.Equal(t, "1", tr.Spans[1].Attrs["level"])

	// Each scope's SetAttributes lands on its own span, not the other's.
	require.Equal(t, int64(10), tr.Spans[0].SetAttrs["outer.attr"])
	require.Equal(t, int64(20), tr.Spans[1].SetAttrs["inner.attr"])
	require.NotContains(t, tr.Spans[0].SetAttrs, "inner.attr")

	// RecordError marks only the inner span.
	require.True(t, tr.Spans[1].HasError)
	require.False(t, tr.Spans[0].HasError)

	require.False(t, tr.Spans[0].Completed)
	require.False(t, tr.Spans[1].Completed)
	doneInner()
	require.True(t, tr.Spans[1].Completed)
	require.False(t, tr.Spans[0].Completed)
	doneOuter()
	require.True(t, tr.Spans[0].Completed)
}

func TestRecordingTracerEventAndSpanOrdering(t *testing.T) {
	t.Parallel()
	tr := NewRecordingTracer()
	tr.Event("first")
	c1, _ := tr.Push("span.a")
	c1.Event("second")
	_, _ = tr.Push("span.b")
	tr.Event("third")

	require.Equal(t, []string{"first", "second", "third"},
		[]string{tr.Events[0].Name, tr.Events[1].Name, tr.Events[2].Name})
	require.Equal(t, []string{"span.a", "span.b"},
		[]string{tr.Spans[0].Name, tr.Spans[1].Name})
}

// TestRecordingTracerConcurrentRecordingIsRaceSafe drives concurrent events and
// spans; run with -race to catch unsynchronized shared-state mutation.
func TestRecordingTracerConcurrentRecordingIsRaceSafe(t *testing.T) {
	t.Parallel()
	tr := NewRecordingTracer()

	const workers = 8
	const perWorker = 50
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				tr.Event("concurrent")
				child, done := tr.Push("concurrent.span")
				child.SetAttributes(attribute.Int("i", i))
				child.RecordError(errors.New("x"))
				done()
			}
		}()
	}
	wg.Wait()

	require.Len(t, tr.Events, workers*perWorker)
	require.Len(t, tr.Spans, workers*perWorker)
	for i := range tr.Spans {
		require.True(t, tr.Spans[i].Completed)
		require.True(t, tr.Spans[i].HasError)
	}
}

func TestRecordingTracerContextIsNonNil(t *testing.T) {
	t.Parallel()
	require.NotNil(t, NewRecordingTracer().Context())
}

// TestRecordingTracerZeroValueRecordsOnItself covers the base() fallback for a
// tracer that was not built through the constructor (root unset): it must still
// record onto itself rather than panic.
func TestRecordingTracerZeroValueRecordsOnItself(t *testing.T) {
	t.Parallel()
	var tr RecordingTracer
	tr.cur = -1
	tr.Event("zero.event")
	require.NotNil(t, tr.FindEvent("zero.event"))
}

func TestAttrValueCoversEveryKind(t *testing.T) {
	t.Parallel()
	require.Equal(t, "s", AttrValue(attribute.StringValue("s")))
	require.Equal(t, int64(7), AttrValue(attribute.IntValue(7)))
	require.Equal(t, true, AttrValue(attribute.BoolValue(true)))
	// Non-scalar kinds fall through to the string rendering.
	sliceRendered, ok := AttrValue(attribute.IntSliceValue([]int{1, 2})).(string)
	require.True(t, ok, "non-scalar attribute renders to a string")
	require.Contains(t, sliceRendered, "1")
}
