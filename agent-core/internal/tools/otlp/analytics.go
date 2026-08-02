// Copyright (c) 2026 Nokia. All rights reserved.

package otlp

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const (
	// InitSpoolSpanStats identifies the duration-heatmap and group-by factory.
	InitSpoolSpanStats = "spool_span_stats"
	// InitSpoolSpanBreakdown identifies the attribute-divergence factory.
	InitSpoolSpanBreakdown = "spool_span_breakdown"

	defaultTimeBuckets = 24
	defaultTopN        = 20
	defaultMaxTopN     = 100
)

// defaultDurationEdgesMs is the fallback duration-bucket boundary set in
// milliseconds; a span at or above the last edge lands in the overflow bucket.
var defaultDurationEdgesMs = []int64{0, 1, 10, 50, 100, 500, 1000, 5000}

// spanFilter is a conjunctive predicate over enriched spans. A zero-valued
// field is an unset term that matches everything.
type spanFilter struct {
	StartMs  int64
	EndMs    int64
	Service  string
	SpanName string
	MinDurMs int64
	MaxDurMs int64
	Attrs    map[string]string
}

func (f spanFilter) matches(s enrichedSpan) bool {
	if f.StartMs != 0 && s.startMs < f.StartMs {
		return false
	}
	if f.EndMs != 0 && s.startMs >= f.EndMs {
		return false
	}
	if f.Service != "" && s.service != f.Service {
		return false
	}
	if f.SpanName != "" && s.name != f.SpanName {
		return false
	}
	if f.MinDurMs != 0 && s.durMs < f.MinDurMs {
		return false
	}
	if f.MaxDurMs != 0 && s.durMs >= f.MaxDurMs {
		return false
	}
	for k, v := range f.Attrs {
		if s.attrs[k] != v {
			return false
		}
	}
	return true
}

// enrichedSpan is a spool span with its analytics-relevant fields resolved once.
type enrichedSpan struct {
	startMs int64
	durMs   int64
	service string
	name    string
	traceID string
	attrs   map[string]string
}

func enrichSpans(spans []spoolSpan) []enrichedSpan {
	out := make([]enrichedSpan, 0, len(spans))
	for _, s := range spans {
		dur := s.EndTime.Sub(s.StartTime).Milliseconds()
		if dur < 0 {
			dur = 0
		}
		out = append(out, enrichedSpan{
			startMs: s.StartTime.UnixMilli(),
			durMs:   dur,
			service: serviceFromResource(s.Resource),
			name:    s.Name,
			traceID: s.SpanContext.TraceID,
			attrs:   spanAttrs(s.Attributes),
		})
	}
	return out
}

func spanAttrs(raw json.RawMessage) map[string]string {
	out := map[string]string{}
	if raw == nil {
		return out
	}
	var attrs []struct {
		Key   string `json:"Key"`
		Value struct {
			Value any `json:"Value"`
		} `json:"Value"`
	}
	if json.Unmarshal(raw, &attrs) != nil {
		return out
	}
	for _, a := range attrs {
		if a.Key == "" {
			continue
		}
		out[a.Key] = stringifyAttr(a.Value.Value)
	}
	return out
}

func stringifyAttr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		if t == math.Trunc(t) && math.Abs(t) < 1e15 {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'g', -1, 64)
	case nil:
		return ""
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// groupValue resolves the group-by key for one span. "service.name" and
// "span.name" are special; any other key reads a span attribute.
func (s enrichedSpan) groupValue(key string) (string, bool) {
	switch key {
	case "service.name":
		return s.service, s.service != ""
	case "span.name":
		return s.name, s.name != ""
	default:
		v, ok := s.attrs[key]
		return v, ok
	}
}

// SpanStatsConfig configures a duration heatmap plus optional group-by counts.
type SpanStatsConfig struct {
	Path            string
	Filter          spanFilter
	TimeBuckets     int
	DurationEdgesMs []int64
	GroupBy         string
	TopN            int
	MaxTopN         int
}

// SpanBreakdownConfig configures an attribute-divergence comparison of a
// selection against its complement within a baseline.
type SpanBreakdownConfig struct {
	Path      string
	Baseline  spanFilter
	Selection spanFilter
	TopN      int
	MaxTopN   int
}

// SpanStatsBuilder constructs duration-heatmap and group-by commands.
type SpanStatsBuilder struct {
	ToolName string
	Config   SpanStatsConfig
}

// Build creates one stats command, letting a machine_request seed override the
// filter, time-bucket count, group-by key, and top-N.
func (b SpanStatsBuilder) Build(previous core.Result) core.Command {
	cfg := b.Config
	if p := seedParams(previous); p != nil {
		cfg.Filter = seedFilter(p)
		if v, ok := seedInt(p, "time_buckets"); ok && v > 0 {
			cfg.TimeBuckets = v
		}
		if v, ok := p["group_by"].(string); ok {
			cfg.GroupBy = v
		}
		if v, ok := seedInt(p, "top_n"); ok && v > 0 {
			cfg.TopN = v
		}
	}
	return &spanStatsCommand{toolName: b.ToolName, config: cfg}
}

// SpanBreakdownBuilder constructs attribute-divergence commands.
type SpanBreakdownBuilder struct {
	ToolName string
	Config   SpanBreakdownConfig
}

// Build creates one breakdown command, letting a machine_request seed override
// the baseline filter, selection filter, and top-N.
func (b SpanBreakdownBuilder) Build(previous core.Result) core.Command {
	cfg := b.Config
	if p := seedParams(previous); p != nil {
		if base, ok := p["baseline"].(map[string]interface{}); ok {
			cfg.Baseline = seedFilter(base)
		}
		if sel, ok := p["selection"].(map[string]interface{}); ok {
			cfg.Selection = seedFilter(sel)
		}
		if v, ok := seedInt(p, "top_n"); ok && v > 0 {
			cfg.TopN = v
		}
	}
	return &spanBreakdownCommand{toolName: b.ToolName, config: cfg}
}

// seedFilter reads a spanFilter from a seed parameter map. Absent keys leave
// the corresponding term unset.
func seedFilter(p map[string]interface{}) spanFilter {
	f := spanFilter{Attrs: map[string]string{}}
	if v, ok := seedInt64(p, "start_ms"); ok {
		f.StartMs = v
	}
	if v, ok := seedInt64(p, "end_ms"); ok {
		f.EndMs = v
	}
	if v, ok := p["service"].(string); ok {
		f.Service = v
	}
	if v, ok := p["span_name"].(string); ok {
		f.SpanName = v
	}
	if v, ok := seedInt64(p, "min_duration_ms"); ok {
		f.MinDurMs = v
	}
	if v, ok := seedInt64(p, "max_duration_ms"); ok {
		f.MaxDurMs = v
	}
	if attrs, ok := p["attributes"].(map[string]interface{}); ok {
		for k, v := range attrs {
			f.Attrs[k] = stringifyAttr(v)
		}
	}
	return f
}

func seedInt64(params map[string]interface{}, key string) (int64, bool) {
	v, ok := params[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case string:
		parsed, err := strconv.ParseInt(n, 10, 64)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

type heatmapPayload struct {
	TimeBucketBoundaries     []int64 `json:"time_bucket_boundaries"`
	DurationBucketBoundaries []int64 `json:"duration_bucket_boundaries"`
	Cells                    [][]int `json:"cells"`
}

type groupCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type spanStatsCommand struct {
	toolName string
	config   SpanStatsConfig
}

func (c *spanStatsCommand) Name() string { return c.toolName }

func (c *spanStatsCommand) Execute() core.Result {
	spans, skipped, err := readSpoolFiles(c.config.Path)
	if err != nil {
		return receiverError(c.Name(), fmt.Errorf("%s: %w", c.Name(), err))
	}
	matched := filterEnriched(enrichSpans(spans), c.config.Filter)
	heatmap := buildHeatmap(matched, c.config)
	groups, groupBy, droppedGroups, droppedSpans := buildGroupBy(matched, c.config)

	output := struct {
		Heatmap          heatmapPayload `json:"heatmap"`
		Matched          int            `json:"matched"`
		SkippedLines     int            `json:"skipped_lines"`
		GroupBy          string         `json:"group_by"`
		Groups           []groupCount   `json:"groups"`
		DroppedGroups    int            `json:"dropped_groups"`
		DroppedSpanTotal int            `json:"dropped_span_total"`
	}{
		Heatmap: heatmap, Matched: len(matched), SkippedLines: skipped,
		GroupBy: groupBy, Groups: groups,
		DroppedGroups: droppedGroups, DroppedSpanTotal: droppedSpans,
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return receiverError(c.Name(), err)
	}
	return core.Result{Signal: core.Signal("SpanStatsReady"), CommandName: c.Name(), Output: string(encoded)}
}

func (c *spanStatsCommand) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.Name())
}

type divergenceEntry struct {
	Key               string  `json:"key"`
	Value             string  `json:"value"`
	InsideCount       int     `json:"inside_count"`
	OutsideCount      int     `json:"outside_count"`
	InsideProportion  float64 `json:"inside_proportion"`
	OutsideProportion float64 `json:"outside_proportion"`
	Score             float64 `json:"score"`
}

type spanBreakdownCommand struct {
	toolName string
	config   SpanBreakdownConfig
}

func (c *spanBreakdownCommand) Name() string { return c.toolName }

func (c *spanBreakdownCommand) Execute() core.Result {
	spans, skipped, err := readSpoolFiles(c.config.Path)
	if err != nil {
		return receiverError(c.Name(), fmt.Errorf("%s: %w", c.Name(), err))
	}
	baseline := filterEnriched(enrichSpans(spans), c.config.Baseline)
	var inside, outside []enrichedSpan
	for _, s := range baseline {
		if c.config.Selection.matches(s) {
			inside = append(inside, s)
		} else {
			outside = append(outside, s)
		}
	}
	ranked, dropped := rankDivergence(inside, outside, effectiveTopN(c.config.TopN, c.config.MaxTopN))

	output := struct {
		InsideTotal  int               `json:"inside_total"`
		OutsideTotal int               `json:"outside_total"`
		Ranked       []divergenceEntry `json:"ranked"`
		Dropped      int               `json:"dropped"`
		SkippedLines int               `json:"skipped_lines"`
	}{
		InsideTotal: len(inside), OutsideTotal: len(outside),
		Ranked: ranked, Dropped: dropped, SkippedLines: skipped,
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return receiverError(c.Name(), err)
	}
	return core.Result{Signal: core.Signal("SpanBreakdownReady"), CommandName: c.Name(), Output: string(encoded)}
}

func (c *spanBreakdownCommand) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.Name())
}
