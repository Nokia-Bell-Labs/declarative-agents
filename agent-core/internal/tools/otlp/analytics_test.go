// Copyright (c) 2026 Nokia. All rights reserved.

package otlp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
)

// analyticsSpan is a spool span fixture carrying attributes, which the shared
// writeSpoolSpans helper does not.
type analyticsSpan struct {
	traceID, spanID, service, name string
	start, end                     time.Time
	attrs                          map[string]string
}

func writeAnalyticsSpans(t *testing.T, path string, spans []analyticsSpan) {
	t.Helper()
	var data []byte
	for _, s := range spans {
		attrs := make([]map[string]any, 0, len(s.attrs))
		for k, v := range s.attrs {
			attrs = append(attrs, map[string]any{"Key": k, "Value": map[string]any{"Type": "STRING", "Value": v}})
		}
		line := map[string]any{
			"Name":        s.name,
			"SpanContext": map[string]any{"TraceID": s.traceID, "SpanID": s.spanID},
			"Parent":      map[string]any{"TraceID": s.traceID, "SpanID": ""},
			"StartTime":   s.start,
			"EndTime":     s.end,
			"Status":      map[string]any{"Code": 0, "Description": ""},
			"Attributes":  attrs,
			"Resource": []map[string]any{
				{"Key": "service.name", "Value": map[string]any{"Type": "STRING", "Value": s.service}},
			},
		}
		encoded, err := json.Marshal(line)
		require.NoError(t, err)
		data = append(data, encoded...)
		data = append(data, '\n')
	}
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func seedResult(t *testing.T, params map[string]any) core.Result {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"parameters": params})
	require.NoError(t, err)
	return core.Result{Output: string(encoded)}
}

func at(base time.Time, offsetMs, durMs int) (time.Time, time.Time) {
	start := base.Add(time.Duration(offsetMs) * time.Millisecond)
	return start, start.Add(time.Duration(durMs) * time.Millisecond)
}

type statsOutput struct {
	Heatmap struct {
		TimeBucketBoundaries     []int64 `json:"time_bucket_boundaries"`
		DurationBucketBoundaries []int64 `json:"duration_bucket_boundaries"`
		Cells                    [][]int `json:"cells"`
	} `json:"heatmap"`
	Matched          int          `json:"matched"`
	ExemplarTraceIDs []string     `json:"exemplar_trace_ids"`
	SkippedLines     int          `json:"skipped_lines"`
	GroupBy          string       `json:"group_by"`
	Groups           []groupCount `json:"groups"`
	DroppedGroups    int          `json:"dropped_groups"`
	DroppedSpanTotal int          `json:"dropped_span_total"`
}

func runStats(t *testing.T, path string, seed core.Result) statsOutput {
	t.Helper()
	result := SpanStatsBuilder{
		ToolName: "spool_span_stats",
		Config:   SpanStatsConfig{Path: path, TimeBuckets: 4, MaxTopN: 100},
	}.Build(seed).Execute()
	require.Equal(t, core.Signal("SpanStatsReady"), result.Signal, result.Output)
	var out statsOutput
	require.NoError(t, json.Unmarshal([]byte(result.Output), &out))
	return out
}

func TestSpanStatsFilterConjunction(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	path := filepath.Join(t.TempDir(), "collector.ndjson")
	s0, e0 := at(base, 0, 5)
	s1, e1 := at(base, 10, 5)
	s2, e2 := at(base, 20, 5)
	s3, e3 := at(base, 30, 5)
	writeAnalyticsSpans(t, path, []analyticsSpan{
		{traceID: "t1", spanID: "a", service: "svc-a", name: "handle", start: s0, end: e0, attrs: map[string]string{"phase": "parse"}},
		{traceID: "t1", spanID: "b", service: "svc-a", name: "handle", start: s1, end: e1, attrs: map[string]string{"phase": "emit"}},
		{traceID: "t2", spanID: "c", service: "svc-b", name: "handle", start: s2, end: e2, attrs: map[string]string{"phase": "parse"}},
		{traceID: "t3", spanID: "d", service: "svc-a", name: "other", start: s3, end: e3, attrs: map[string]string{"phase": "parse"}},
	})

	full := runStats(t, path, seedResult(t, map[string]any{
		"service": "svc-a", "span_name": "handle", "attributes": map[string]any{"phase": "parse"},
	}))
	require.Equal(t, 1, full.Matched)

	// Dropping the attribute term admits the emit span too.
	looser := runStats(t, path, seedResult(t, map[string]any{"service": "svc-a", "span_name": "handle"}))
	require.Equal(t, 2, looser.Matched)
}

func TestSpanStatsEmptyFilterMatchesAll(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	path := filepath.Join(t.TempDir(), "collector.ndjson")
	s0, e0 := at(base, 0, 5)
	s1, e1 := at(base, 10, 5)
	s2, e2 := at(base, 20, 5)
	writeAnalyticsSpans(t, path, []analyticsSpan{
		{traceID: "t1", spanID: "a", service: "svc-a", name: "x", start: s0, end: e0},
		{traceID: "t1", spanID: "b", service: "svc-b", name: "y", start: s1, end: e1},
		{traceID: "t2", spanID: "c", service: "svc-c", name: "z", start: s2, end: e2},
	})
	out := runStats(t, path, core.Result{})
	require.Equal(t, 3, out.Matched)
}

func TestSpanStatsHeatmapGrid(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	path := filepath.Join(t.TempDir(), "collector.ndjson")
	// Four spans spread across time with durations that land in distinct buckets,
	// plus one very long span for the overflow bucket.
	var spans []analyticsSpan
	for i, dur := range []int{2, 20, 200, 9000} {
		s, e := at(base, i*100, dur)
		spans = append(spans, analyticsSpan{traceID: "t", spanID: string(rune('a' + i)), service: "svc", name: "op", start: s, end: e})
	}
	writeAnalyticsSpans(t, path, spans)

	result := SpanStatsBuilder{
		ToolName: "spool_span_stats",
		Config: SpanStatsConfig{
			Path: path, TimeBuckets: 4, DurationEdgesMs: []int64{0, 10, 100, 1000}, MaxTopN: 100,
		},
	}.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("SpanStatsReady"), result.Signal, result.Output)
	var out statsOutput
	require.NoError(t, json.Unmarshal([]byte(result.Output), &out))

	require.Equal(t, 4, out.Matched)
	require.Len(t, out.Heatmap.TimeBucketBoundaries, 5)
	for i := 1; i < len(out.Heatmap.TimeBucketBoundaries); i++ {
		require.Greater(t, out.Heatmap.TimeBucketBoundaries[i], out.Heatmap.TimeBucketBoundaries[i-1])
	}
	sum := 0
	for _, row := range out.Heatmap.Cells {
		for _, c := range row {
			sum += c
		}
	}
	require.Equal(t, out.Matched, sum)
	// The 9000ms span lands in the overflow (last) duration bucket.
	overflow := 0
	for _, row := range out.Heatmap.Cells {
		overflow += row[len(row)-1]
	}
	require.Equal(t, 1, overflow)
}

func TestSpanStatsGroupByTopN(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	path := filepath.Join(t.TempDir(), "collector.ndjson")
	// Five distinct tool.name values with counts 5,4,3,2,1.
	counts := map[string]int{"a": 5, "b": 4, "c": 3, "d": 2, "e": 1}
	var spans []analyticsSpan
	idx := 0
	for val, n := range counts {
		for i := 0; i < n; i++ {
			s, e := at(base, idx*10, 5)
			spans = append(spans, analyticsSpan{traceID: "t", spanID: string(rune('A' + idx)), service: "svc", name: "op", start: s, end: e, attrs: map[string]string{"tool.name": val}})
			idx++
		}
	}
	writeAnalyticsSpans(t, path, spans)

	out := runStats(t, path, seedResult(t, map[string]any{"group_by": "tool.name", "top_n": 3}))
	require.Equal(t, "tool.name", out.GroupBy)
	require.Len(t, out.Groups, 3)
	require.Equal(t, "a", out.Groups[0].Value)
	require.Equal(t, 5, out.Groups[0].Count)
	require.Equal(t, 2, out.DroppedGroups)
	require.Equal(t, 3, out.DroppedSpanTotal) // d(2) + e(1)
}

type breakdownOutput struct {
	InsideTotal      int               `json:"inside_total"`
	OutsideTotal     int               `json:"outside_total"`
	ExemplarTraceIDs []string          `json:"exemplar_trace_ids"`
	Ranked           []divergenceEntry `json:"ranked"`
	Dropped          int               `json:"dropped"`
	SkippedLines     int               `json:"skipped_lines"`
}

func runBreakdown(t *testing.T, path string, seed core.Result) breakdownOutput {
	t.Helper()
	result := SpanBreakdownBuilder{
		ToolName: "spool_span_breakdown",
		Config:   SpanBreakdownConfig{Path: path, MaxTopN: 100},
	}.Build(seed).Execute()
	require.Equal(t, core.Signal("SpanBreakdownReady"), result.Signal, result.Output)
	var out breakdownOutput
	require.NoError(t, json.Unmarshal([]byte(result.Output), &out))
	return out
}

func TestSpanBreakdownDivergenceRanking(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	path := filepath.Join(t.TempDir(), "collector.ndjson")
	var spans []analyticsSpan
	// Fast spans: culprit=no. Slow spans (>=500ms): culprit=yes.
	for i := 0; i < 6; i++ {
		s, e := at(base, i*10, 10)
		spans = append(spans, analyticsSpan{traceID: "t", spanID: string(rune('a' + i)), service: "svc", name: "op", start: s, end: e, attrs: map[string]string{"culprit": "no"}})
	}
	for i := 0; i < 4; i++ {
		s, e := at(base, 100+i*10, 800)
		spans = append(spans, analyticsSpan{traceID: "t", spanID: string(rune('m' + i)), service: "svc", name: "op", start: s, end: e, attrs: map[string]string{"culprit": "yes"}})
	}
	writeAnalyticsSpans(t, path, spans)

	out := runBreakdown(t, path, seedResult(t, map[string]any{
		"selection_min_duration_ms": 500,
		"top_n":                     5,
	}))
	require.Equal(t, 4, out.InsideTotal)
	require.Equal(t, 6, out.OutsideTotal)
	require.NotEmpty(t, out.Ranked)
	require.Equal(t, "culprit", out.Ranked[0].Key)
	require.Equal(t, "yes", out.Ranked[0].Value)
	require.Equal(t, 4, out.Ranked[0].InsideCount)
	require.Equal(t, 0, out.Ranked[0].OutsideCount)
	require.Greater(t, out.Ranked[0].Score, 0.0)
}

func TestSpanBreakdownEmptySelection(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	path := filepath.Join(t.TempDir(), "collector.ndjson")
	s0, e0 := at(base, 0, 5)
	writeAnalyticsSpans(t, path, []analyticsSpan{
		{traceID: "t", spanID: "a", service: "svc", name: "op", start: s0, end: e0, attrs: map[string]string{"k": "v"}},
	})
	out := runBreakdown(t, path, seedResult(t, map[string]any{
		"selection_service": "nonexistent",
	}))
	require.Equal(t, 0, out.InsideTotal)
	require.Empty(t, out.Ranked)
}

func TestExemplarTraceIDsDedupeSortCap(t *testing.T) {
	spans := []enrichedSpan{
		{traceID: "tc"}, {traceID: "ta"}, {traceID: "tb"},
		{traceID: "ta"}, // duplicate collapses
		{traceID: ""},   // blank is skipped
		{traceID: "td"},
	}
	// Deduped and lexicographically ordered, independent of input order.
	require.Equal(t, []string{"ta", "tb", "tc", "td"}, exemplarTraceIDs(spans, 0))
	// Cap keeps the first N of the deterministic order.
	require.Equal(t, []string{"ta", "tb"}, exemplarTraceIDs(spans, 2))
	// Empty set yields an empty (non-nil-required) list.
	require.Empty(t, exemplarTraceIDs(nil, 5))
}

func TestSpanStatsExemplarsDedupedAndSorted(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	path := filepath.Join(t.TempDir(), "collector.ndjson")
	var spans []analyticsSpan
	// Three distinct traces, one with two spans, written out of sorted order.
	for i, id := range []string{"tc", "ta", "ta", "tb"} {
		s, e := at(base, i*10, 5)
		spans = append(spans, analyticsSpan{traceID: id, spanID: string(rune('a' + i)), service: "svc", name: "op", start: s, end: e})
	}
	writeAnalyticsSpans(t, path, spans)

	out := runStats(t, path, core.Result{})
	require.Equal(t, 4, out.Matched)
	require.Equal(t, []string{"ta", "tb", "tc"}, out.ExemplarTraceIDs)
}

func TestSpanStatsExemplarCapLimitsList(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	path := filepath.Join(t.TempDir(), "collector.ndjson")
	var spans []analyticsSpan
	for i, id := range []string{"t5", "t1", "t4", "t2", "t3"} {
		s, e := at(base, i*10, 5)
		spans = append(spans, analyticsSpan{traceID: id, spanID: string(rune('a' + i)), service: "svc", name: "op", start: s, end: e})
	}
	writeAnalyticsSpans(t, path, spans)

	result := SpanStatsBuilder{
		ToolName: "spool_span_stats",
		Config:   SpanStatsConfig{Path: path, TimeBuckets: 4, MaxTopN: 100, ExemplarCap: 2},
	}.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("SpanStatsReady"), result.Signal, result.Output)
	var out statsOutput
	require.NoError(t, json.Unmarshal([]byte(result.Output), &out))
	require.Equal(t, []string{"t1", "t2"}, out.ExemplarTraceIDs)
}

func TestSpanBreakdownExemplarsFromInsideOnly(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	path := filepath.Join(t.TempDir(), "collector.ndjson")
	var spans []analyticsSpan
	// Fast spans (outside): traces o1, o2. Slow spans >=500ms (inside): i2, i1.
	for i, id := range []string{"o1", "o2"} {
		s, e := at(base, i*10, 10)
		spans = append(spans, analyticsSpan{traceID: id, spanID: string(rune('a' + i)), service: "svc", name: "op", start: s, end: e})
	}
	for i, id := range []string{"i2", "i1"} {
		s, e := at(base, 100+i*10, 800)
		spans = append(spans, analyticsSpan{traceID: id, spanID: string(rune('m' + i)), service: "svc", name: "op", start: s, end: e})
	}
	writeAnalyticsSpans(t, path, spans)

	out := runBreakdown(t, path, seedResult(t, map[string]any{"selection_min_duration_ms": 500}))
	require.Equal(t, 2, out.InsideTotal)
	// Exemplars are the inside (selection) traces only, deduped and sorted.
	require.Equal(t, []string{"i1", "i2"}, out.ExemplarTraceIDs)
}

func TestSpanStatsMalformedAndDeterministic(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	path := filepath.Join(t.TempDir(), "collector.ndjson")
	s0, e0 := at(base, 0, 5)
	s1, e1 := at(base, 10, 5)
	writeAnalyticsSpans(t, path, []analyticsSpan{
		{traceID: "t", spanID: "a", service: "svc", name: "op", start: s0, end: e0},
		{traceID: "t", spanID: "b", service: "svc", name: "op", start: s1, end: e1},
	})
	// Prepend two malformed lines.
	existing, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append([]byte("not json\n{bad\n"), existing...), 0o600))

	first := SpanStatsBuilder{ToolName: "spool_span_stats", Config: SpanStatsConfig{Path: path, TimeBuckets: 4, MaxTopN: 100}}.Build(core.Result{}).Execute()
	second := SpanStatsBuilder{ToolName: "spool_span_stats", Config: SpanStatsConfig{Path: path, TimeBuckets: 4, MaxTopN: 100}}.Build(core.Result{}).Execute()
	require.Equal(t, first.Output, second.Output, "output must be deterministic for a fixed spool")

	var out statsOutput
	require.NoError(t, json.Unmarshal([]byte(first.Output), &out))
	require.Equal(t, 2, out.Matched)
	require.Equal(t, 2, out.SkippedLines)

	// An absent spool is empty, not an error.
	empty := runStats(t, filepath.Join(t.TempDir(), "absent.ndjson"), core.Result{})
	require.Equal(t, 0, empty.Matched)
}

func TestSpanStatsEmptyPathError(t *testing.T) {
	result := SpanStatsBuilder{ToolName: "spool_span_stats", Config: SpanStatsConfig{Path: ""}}.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("CommandError"), result.Signal, result.Output)
}
