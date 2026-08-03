// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// serveCollectorWithSpool launches the shipped collector with patched ports,
// optionally seeding the trace spool, and returns the query origin, control
// origin, and the profile directory (whose traces/collector.ndjson is the spool
// a test may seed itself).
func serveCollectorWithSpool(t *testing.T, seed bool, patches map[string]string) (queryAddr, controlAddr, profileDir string) {
	t.Helper()
	controlAddr = FreeAddr(t)
	monitorAddr := FreeAddr(t)
	queryAddr = FreeAddr(t)
	receiverAddr := FreeAddr(t)
	p := map[string]string{
		"127.0.0.1:${COLLECTOR_CONTROL_PORT:-18191}":                         controlAddr,
		"127.0.0.1:${COLLECTOR_MONITOR_PORT:-18192}":                         monitorAddr,
		"127.0.0.1:${COLLECTOR_QUERY_PORT:-18193}":                           queryAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_CONTROL_PORT:-18191}": controlAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_MONITOR_PORT:-18192}": monitorAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_QUERY_PORT:-18193}":   queryAddr,
		"0.0.0.0:4317": receiverAddr,
	}
	for k, v := range patches {
		p[k] = v
	}
	profilePath := CopyShippedProfile(t, filepath.Join("agents", "collector", "profile.yaml"), p)
	profileDir = filepath.Dir(profilePath)
	if seed {
		seedCollectorSpool(t, filepath.Join(profileDir, "traces", "collector.ndjson"))
	}
	server := Serve(t, ServeConfig{Profile: profilePath, Env: collectorEnv(receiverAddr)})
	server.WaitHealthy("http://"+controlAddr+"/api/lifecycle/health", 15*time.Second)
	t.Cleanup(func() {
		_ = server.Post("http://"+controlAddr+"/api/lifecycle/exit", `{"reason":"conformance"}`)
	})
	return queryAddr, controlAddr, profileDir
}

// writeAttributedSpool writes spans carrying a single attribute so a breakdown
// has an attribute value to rank. durMs sets the span duration.
func writeAttributedSpool(t *testing.T, path string, spans []struct {
	traceID, spanID, service string
	offsetMs, durMs          int
	attrKey, attrValue       string
}) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir spool: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var data []byte
	for _, s := range spans {
		start := base.Add(time.Duration(s.offsetMs) * time.Millisecond)
		line := map[string]any{
			"Name":        "op",
			"SpanContext": map[string]any{"TraceID": s.traceID, "SpanID": s.spanID},
			"Parent":      map[string]any{"TraceID": s.traceID, "SpanID": ""},
			"StartTime":   start.Format(time.RFC3339Nano),
			"EndTime":     start.Add(time.Duration(s.durMs) * time.Millisecond).Format(time.RFC3339Nano),
			"Status":      map[string]any{"Code": 0, "Description": ""},
			"Attributes":  []map[string]any{{"Key": s.attrKey, "Value": map[string]any{"Type": "STRING", "Value": s.attrValue}}},
			"Resource":    []map[string]any{{"Key": "service.name", "Value": map[string]any{"Type": "STRING", "Value": s.service}}},
		}
		encoded, err := json.Marshal(line)
		if err != nil {
			t.Fatalf("marshal span: %v", err)
		}
		data = append(data, encoded...)
		data = append(data, '\n')
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write spool: %v", err)
	}
}

func getExplore(t *testing.T, url string) (map[string]json.RawMessage, int) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode %s: %v; body:\n%s", url, err, body)
	}
	return payload, resp.StatusCode
}

func requireKeys(t *testing.T, payload map[string]json.RawMessage, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := payload[k]; !ok {
			t.Errorf("response missing key %q (keep agents/collector/ui/src/api/client.ts in sync)", k)
		}
	}
}

// TestCollectorSpanStatsContract pins the /query/spans/stats response contract:
// a heatmap with axis boundaries and cells whose counts sum to the matched
// total, plus group-by counts (srd020 R8, AC6).
func TestCollectorSpanStatsContract(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	queryAddr, _, _ := serveCollectorWithSpool(t, true, nil)

	payload, status := getExplore(t, "http://"+queryAddr+"/query/spans/stats?group_by=service.name")
	if status != http.StatusOK {
		t.Fatalf("stats status = %d, want 200", status)
	}
	requireKeys(t, payload, "heatmap", "matched", "exemplar_trace_ids", "skipped_lines", "group_by", "groups", "dropped_groups", "dropped_span_total")
	var statsExemplars []string
	_ = json.Unmarshal(payload["exemplar_trace_ids"], &statsExemplars)
	if len(statsExemplars) == 0 {
		t.Errorf("stats exemplar_trace_ids empty; want exemplars for the matched set")
	}

	var heatmap struct {
		TimeBucketBoundaries     []int64 `json:"time_bucket_boundaries"`
		DurationBucketBoundaries []int64 `json:"duration_bucket_boundaries"`
		Cells                    [][]int `json:"cells"`
	}
	if err := json.Unmarshal(payload["heatmap"], &heatmap); err != nil {
		t.Fatalf("decode heatmap: %v", err)
	}
	var matched int
	_ = json.Unmarshal(payload["matched"], &matched)
	if matched != 3 {
		t.Fatalf("matched = %d, want 3 (seeded fixture)", matched)
	}
	sum := 0
	for _, row := range heatmap.Cells {
		for _, c := range row {
			sum += c
		}
	}
	if sum != matched {
		t.Errorf("heatmap cells sum = %d, want matched %d", sum, matched)
	}
	var groups []struct {
		Value string `json:"value"`
		Count int    `json:"count"`
	}
	_ = json.Unmarshal(payload["groups"], &groups)
	if len(groups) != 2 || groups[0].Value != "svc-a" || groups[0].Count != 2 {
		t.Errorf("group-by service.name = %+v, want svc-a(2) first then svc-b(1)", groups)
	}
}

// TestCollectorSpanBreakdownContract pins the /query/spans/breakdown response
// contract: inside and outside totals and a ranked attribute divergence
// (srd020 R8, AC6).
func TestCollectorSpanBreakdownContract(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	queryAddr, _, profileDir := serveCollectorWithSpool(t, false, nil)

	// Slow spans (>=500ms) carry culprit=yes; fast spans carry culprit=no. The
	// selection is the slow band, so culprit=yes should rank first.
	type row = struct {
		traceID, spanID, service string
		offsetMs, durMs          int
		attrKey, attrValue       string
	}
	var spans []row
	for i := 0; i < 5; i++ {
		spans = append(spans, row{"t", string(rune('a' + i)), "svc", i * 10, 10, "culprit", "no"})
	}
	for i := 0; i < 3; i++ {
		spans = append(spans, row{"t", string(rune('m' + i)), "svc", 100 + i*10, 800, "culprit", "yes"})
	}
	writeAttributedSpool(t, filepath.Join(profileDir, "traces", "collector.ndjson"), spans)

	payload, status := getExplore(t, "http://"+queryAddr+"/query/spans/breakdown?selection_min_duration_ms=500")
	if status != http.StatusOK {
		t.Fatalf("breakdown status = %d, want 200", status)
	}
	requireKeys(t, payload, "inside_total", "outside_total", "exemplar_trace_ids", "ranked", "dropped", "skipped_lines")
	var breakdownExemplars []string
	_ = json.Unmarshal(payload["exemplar_trace_ids"], &breakdownExemplars)
	if len(breakdownExemplars) == 0 {
		t.Errorf("breakdown exemplar_trace_ids empty; want exemplars for the inside (selection) set")
	}
	var inside, outside int
	_ = json.Unmarshal(payload["inside_total"], &inside)
	_ = json.Unmarshal(payload["outside_total"], &outside)
	if inside != 3 || outside != 5 {
		t.Fatalf("inside=%d outside=%d, want 3/5", inside, outside)
	}
	var ranked []struct {
		Key               string  `json:"key"`
		Value             string  `json:"value"`
		InsideCount       int     `json:"inside_count"`
		OutsideCount      int     `json:"outside_count"`
		InsideProportion  float64 `json:"inside_proportion"`
		OutsideProportion float64 `json:"outside_proportion"`
		Score             float64 `json:"score"`
	}
	if err := json.Unmarshal(payload["ranked"], &ranked); err != nil {
		t.Fatalf("decode ranked: %v", err)
	}
	if len(ranked) == 0 || ranked[0].Key != "culprit" || ranked[0].Value != "yes" {
		t.Fatalf("ranked[0] = %+v, want culprit=yes first", ranked)
	}
	if ranked[0].InsideCount != 3 || ranked[0].OutsideCount != 0 || ranked[0].Score <= 0 {
		t.Errorf("ranked[0] contract = %+v, want inside 3 / outside 0 / score>0", ranked[0])
	}
}

// TestCollectorExploreCapsFromConfig proves the configured top-N cap beats a
// larger request value: with max_top_n patched to 1, a request for top_n 1000
// still returns one group and reports the dropped remainder (srd020 R8.3, AC6).
func TestCollectorExploreCapsFromConfig(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	queryAddr, _, _ := serveCollectorWithSpool(t, true, map[string]string{"max_top_n: 100": "max_top_n: 1"})

	payload, status := getExplore(t, "http://"+queryAddr+"/query/spans/stats?group_by=service.name&top_n=1000")
	if status != http.StatusOK {
		t.Fatalf("stats status = %d, want 200", status)
	}
	var groups []json.RawMessage
	_ = json.Unmarshal(payload["groups"], &groups)
	var droppedGroups int
	_ = json.Unmarshal(payload["dropped_groups"], &droppedGroups)
	if len(groups) != 1 {
		t.Errorf("groups returned = %d, want 1 (config cap beats request top_n)", len(groups))
	}
	if droppedGroups != 1 {
		t.Errorf("dropped_groups = %d, want 1", droppedGroups)
	}
}

// TestCollectorExploreEmptySpool proves both Explore routes answer a well-formed
// empty aggregate rather than an error when no spans are spooled (srd020 AC7).
// It inspects the full response schema — the heatmap shape and the group/rank
// lists — with checked typed decoding, so a bare {} or a wrong-typed field
// fails rather than passing on zero values.
func TestCollectorExploreEmptySpool(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	queryAddr, _, _ := serveCollectorWithSpool(t, false, nil)

	stats, status := getExplore(t, "http://"+queryAddr+"/query/spans/stats")
	if status != http.StatusOK {
		t.Fatalf("stats status = %d, want 200 for empty spool", status)
	}
	requireKeys(t, stats, "heatmap", "matched", "exemplar_trace_ids",
		"skipped_lines", "group_by", "groups", "dropped_groups", "dropped_span_total")
	var matched int
	if err := json.Unmarshal(stats["matched"], &matched); err != nil {
		t.Fatalf("decode matched: %v", err)
	}
	if matched != 0 {
		t.Errorf("empty-spool matched = %d, want 0", matched)
	}
	var heatmap struct {
		TimeBucketBoundaries     []int64 `json:"time_bucket_boundaries"`
		DurationBucketBoundaries []int64 `json:"duration_bucket_boundaries"`
		Cells                    [][]int `json:"cells"`
	}
	if err := json.Unmarshal(stats["heatmap"], &heatmap); err != nil {
		t.Fatalf("decode heatmap: %v", err)
	}
	// A well-formed empty heatmap has rectangular cells that sum to zero.
	sum, width := 0, -1
	for _, row := range heatmap.Cells {
		if width == -1 {
			width = len(row)
		} else if len(row) != width {
			t.Errorf("heatmap cells not rectangular: row width %d != %d", len(row), width)
		}
		for _, c := range row {
			sum += c
		}
	}
	if sum != 0 {
		t.Errorf("empty-spool heatmap cells sum = %d, want 0", sum)
	}
	var groups []json.RawMessage
	if err := json.Unmarshal(stats["groups"], &groups); err != nil {
		t.Fatalf("decode groups: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("empty-spool groups = %d, want 0", len(groups))
	}

	breakdown, status := getExplore(t, "http://"+queryAddr+"/query/spans/breakdown?selection_max_duration_ms=100")
	if status != http.StatusOK {
		t.Fatalf("breakdown status = %d, want 200 for empty spool", status)
	}
	requireKeys(t, breakdown, "inside_total", "outside_total",
		"exemplar_trace_ids", "ranked", "dropped", "skipped_lines")
	var inside, outside int
	if err := json.Unmarshal(breakdown["inside_total"], &inside); err != nil {
		t.Fatalf("decode inside_total: %v", err)
	}
	if err := json.Unmarshal(breakdown["outside_total"], &outside); err != nil {
		t.Fatalf("decode outside_total: %v", err)
	}
	if inside != 0 || outside != 0 {
		t.Errorf("empty-spool inside=%d outside=%d, want 0/0", inside, outside)
	}
	var ranked []json.RawMessage
	if err := json.Unmarshal(breakdown["ranked"], &ranked); err != nil {
		t.Fatalf("decode ranked: %v", err)
	}
	if len(ranked) != 0 {
		t.Errorf("empty-spool ranked = %d, want 0", len(ranked))
	}
}

// TestCollectorExploreRouteDeclared proves the collector UI descriptor wires the
// Explore page and the actions that bind the stats and breakdown machine
// requests, so the served SPA exposes the Explore drill-in (srd020 R9, AC8).
func TestCollectorExploreRouteDeclared(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(ProfilePath(filepath.Join("agents", "collector", "ui", "ux.yaml")))
	if err != nil {
		t.Fatalf("read collector ux.yaml: %v", err)
	}
	ux := string(data)
	for _, want := range []string{"path: /explore", "query_span_stats", "query_span_breakdown"} {
		if !strings.Contains(ux, want) {
			t.Errorf("collector ux.yaml missing %q", want)
		}
	}
}
