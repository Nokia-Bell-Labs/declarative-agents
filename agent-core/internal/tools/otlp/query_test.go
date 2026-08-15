// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

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

func TestListTracesAcrossActiveAndRotated(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "collector.ndjson")

	writeSpoolSpans(t, basePath+".1", []testSpan{
		{traceID: "t1", spanID: "s1", parentID: "", service: "svc-a", name: "root-1",
			start: time.Unix(1_700_000_000, 0).UTC(), end: time.Unix(1_700_000_002, 0).UTC()},
	})
	writeSpoolSpans(t, basePath, []testSpan{
		{traceID: "t2", spanID: "s2", parentID: "", service: "svc-b", name: "root-2",
			start: time.Unix(1_700_000_010, 0).UTC(), end: time.Unix(1_700_000_011, 0).UTC()},
		{traceID: "t2", spanID: "s3", parentID: "s2", service: "svc-b", name: "child",
			start: time.Unix(1_700_000_010, 100).UTC(), end: time.Unix(1_700_000_010, 900).UTC()},
	})

	result := ListTracesBuilder{
		ToolName: "spool_list_traces",
		Config:   QueryListConfig{Path: basePath, PageSize: 10, MaxPageSize: 100},
	}.Build(core.Result{}).Execute()

	require.Equal(t, core.Signal("TracesListed"), result.Signal, result.Output)
	var output struct {
		Traces       []traceSummary `json:"traces"`
		Total        int            `json:"total"`
		SkippedLines int            `json:"skipped_lines"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Equal(t, 2, output.Total)
	require.Equal(t, "t2", output.Traces[0].TraceID)
	require.Equal(t, "svc-b", output.Traces[0].RootService)
	require.Equal(t, "root-2", output.Traces[0].RootSpanName)
	require.Equal(t, 2, output.Traces[0].SpanCount)
	require.Equal(t, "t1", output.Traces[1].TraceID)
	require.Equal(t, 1, output.Traces[1].SpanCount)
	require.Equal(t, 0, output.SkippedLines)
}

func TestListTracesPagination(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "collector.ndjson")

	var spans []testSpan
	base := time.Unix(1_700_000_000, 0).UTC()
	for i := range 5 {
		tid := "trace-" + string(rune('a'+i))
		spans = append(spans, testSpan{
			traceID: tid, spanID: "s" + string(rune('0'+i)), parentID: "",
			service: "svc", name: "root",
			start: base.Add(time.Duration(i) * time.Second),
			end:   base.Add(time.Duration(i)*time.Second + 500*time.Millisecond),
		})
	}
	writeSpoolSpans(t, basePath, spans)

	result := ListTracesBuilder{
		ToolName: "spool_list_traces",
		Config:   QueryListConfig{Path: basePath, PageSize: 2, MaxPageSize: 10, Offset: 1},
	}.Build(core.Result{}).Execute()

	require.Equal(t, core.Signal("TracesListed"), result.Signal)
	var output struct {
		Traces   []traceSummary `json:"traces"`
		Total    int            `json:"total"`
		Offset   int            `json:"offset"`
		PageSize int            `json:"page_size"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Equal(t, 5, output.Total)
	require.Equal(t, 1, output.Offset)
	require.Equal(t, 2, output.PageSize)
	require.Len(t, output.Traces, 2)
	require.Equal(t, "trace-d", output.Traces[0].TraceID)
	require.Equal(t, "trace-c", output.Traces[1].TraceID)
}

func TestListTracesPageSizeCap(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "collector.ndjson")
	writeSpoolSpans(t, basePath, []testSpan{
		{traceID: "t1", spanID: "s1", parentID: "", service: "svc", name: "root",
			start: time.Unix(1_700_000_000, 0).UTC(), end: time.Unix(1_700_000_001, 0).UTC()},
	})

	result := ListTracesBuilder{
		ToolName: "spool_list_traces",
		Config:   QueryListConfig{Path: basePath, PageSize: 50, MaxPageSize: 5},
	}.Build(core.Result{}).Execute()

	require.Equal(t, core.Signal("TracesListed"), result.Signal)
	var output struct {
		PageSize int `json:"page_size"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Equal(t, 5, output.PageSize)
}

func TestListTracesEmptySpool(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "missing.ndjson")

	result := ListTracesBuilder{
		ToolName: "spool_list_traces",
		Config:   QueryListConfig{Path: basePath},
	}.Build(core.Result{}).Execute()

	require.Equal(t, core.Signal("TracesListed"), result.Signal)
	var output struct {
		Traces []traceSummary `json:"traces"`
		Total  int            `json:"total"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Equal(t, 0, output.Total)
	require.Empty(t, output.Traces)
}

func TestListTracesMalformedLineTolerance(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "collector.ndjson")

	validSpan := writeSpoolSpans(t, basePath, []testSpan{
		{traceID: "t1", spanID: "s1", parentID: "", service: "svc", name: "root",
			start: time.Unix(1_700_000_000, 0).UTC(), end: time.Unix(1_700_000_001, 0).UTC()},
	})
	corrupt := []byte("this is not json\n{\"bad\": true}\n")
	require.NoError(t, os.WriteFile(basePath, append(corrupt, validSpan...), 0o600))

	result := ListTracesBuilder{
		ToolName: "spool_list_traces",
		Config:   QueryListConfig{Path: basePath},
	}.Build(core.Result{}).Execute()

	require.Equal(t, core.Signal("TracesListed"), result.Signal)
	var output struct {
		Traces       []traceSummary `json:"traces"`
		Total        int            `json:"total"`
		SkippedLines int            `json:"skipped_lines"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Equal(t, 1, output.Total)
	require.Equal(t, 2, output.SkippedLines)
}

func TestGetTraceReturnsSpanDetails(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "collector.ndjson")

	writeSpoolSpans(t, basePath, []testSpan{
		{traceID: "t1", spanID: "s1", parentID: "", service: "svc-a", name: "root",
			start: time.Unix(1_700_000_000, 0).UTC(), end: time.Unix(1_700_000_002, 0).UTC()},
		{traceID: "t1", spanID: "s2", parentID: "s1", service: "svc-a", name: "child",
			start: time.Unix(1_700_000_000, 100).UTC(), end: time.Unix(1_700_000_001, 0).UTC()},
		{traceID: "t2", spanID: "s3", parentID: "", service: "svc-b", name: "other",
			start: time.Unix(1_700_000_005, 0).UTC(), end: time.Unix(1_700_000_006, 0).UTC()},
	})

	result := GetTraceBuilder{
		ToolName: "spool_get_trace",
		Config:   QueryGetConfig{Path: basePath, TraceID: "t1"},
	}.Build(core.Result{}).Execute()

	require.Equal(t, core.Signal("TraceRetrieved"), result.Signal)
	var output struct {
		TraceID   string       `json:"trace_id"`
		Spans     []spanDetail `json:"spans"`
		SpanCount int          `json:"span_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Equal(t, "t1", output.TraceID)
	require.Equal(t, 2, output.SpanCount)
	require.Equal(t, "root", output.Spans[0].Name)
	require.Equal(t, "svc-a", output.Spans[0].Service)
	require.Equal(t, "", output.Spans[0].ParentSpanID)
	require.Equal(t, "child", output.Spans[1].Name)
	require.Equal(t, "s1", output.Spans[1].ParentSpanID)
}

func TestGetTraceAcrossRotatedFiles(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "collector.ndjson")

	writeSpoolSpans(t, basePath+".1", []testSpan{
		{traceID: "t1", spanID: "s1", parentID: "", service: "svc", name: "root",
			start: time.Unix(1_700_000_000, 0).UTC(), end: time.Unix(1_700_000_002, 0).UTC()},
	})
	writeSpoolSpans(t, basePath, []testSpan{
		{traceID: "t1", spanID: "s2", parentID: "s1", service: "svc", name: "child",
			start: time.Unix(1_700_000_000, 500).UTC(), end: time.Unix(1_700_000_001, 0).UTC()},
	})

	result := GetTraceBuilder{
		ToolName: "spool_get_trace",
		Config:   QueryGetConfig{Path: basePath, TraceID: "t1"},
	}.Build(core.Result{}).Execute()

	require.Equal(t, core.Signal("TraceRetrieved"), result.Signal)
	var output struct {
		SpanCount int          `json:"span_count"`
		Spans     []spanDetail `json:"spans"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Equal(t, 2, output.SpanCount)
	require.Equal(t, "root", output.Spans[0].Name)
	require.Equal(t, "child", output.Spans[1].Name)
}

func TestListTracesDirectorySpoolPathIsExplicitError(t *testing.T) {
	result := ListTracesBuilder{
		ToolName: "spool_list_traces",
		Config:   QueryListConfig{Path: t.TempDir()},
	}.Build(core.Result{}).Execute()

	require.Equal(t, core.CommandError, result.Signal)
	require.ErrorContains(t, result.Err, "is a directory")
}

func TestGetTraceUnknownTraceID(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "collector.ndjson")

	writeSpoolSpans(t, basePath, []testSpan{
		{traceID: "t1", spanID: "s1", parentID: "", service: "svc", name: "root",
			start: time.Unix(1_700_000_000, 0).UTC(), end: time.Unix(1_700_000_001, 0).UTC()},
	})

	result := GetTraceBuilder{
		ToolName: "spool_get_trace",
		Config:   QueryGetConfig{Path: basePath, TraceID: "nonexistent"},
	}.Build(core.Result{}).Execute()

	require.Equal(t, core.Signal("TraceRetrieved"), result.Signal)
	var output struct {
		SpanCount int          `json:"span_count"`
		Spans     []spanDetail `json:"spans"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Equal(t, 0, output.SpanCount)
	require.Empty(t, output.Spans)
}

func TestGetTraceEmptyTraceIDError(t *testing.T) {
	result := GetTraceBuilder{
		ToolName: "spool_get_trace",
		Config:   QueryGetConfig{Path: "/tmp/any.ndjson", TraceID: ""},
	}.Build(core.Result{}).Execute()

	require.Equal(t, core.CommandError, result.Signal)
}

func TestListTracesBuildReadsSeedParams(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "collector.ndjson")
	writeSpoolSpans(t, basePath, []testSpan{
		{traceID: "t1", spanID: "s1", service: "a", name: "r1",
			start: time.Unix(1_700_000_010, 0).UTC(), end: time.Unix(1_700_000_011, 0).UTC()},
		{traceID: "t2", spanID: "s2", service: "b", name: "r2",
			start: time.Unix(1_700_000_009, 0).UTC(), end: time.Unix(1_700_000_010, 0).UTC()},
		{traceID: "t3", spanID: "s3", service: "c", name: "r3",
			start: time.Unix(1_700_000_008, 0).UTC(), end: time.Unix(1_700_000_009, 0).UTC()},
	})
	seed := core.Result{Output: `{"parameters":{"page_size":"1","offset":"1"}}`}
	cmd := ListTracesBuilder{
		ToolName: "spool_list_traces",
		Config:   QueryListConfig{Path: basePath, PageSize: 20, MaxPageSize: 100},
	}.Build(seed)
	result := cmd.Execute()
	require.Equal(t, core.Signal("TracesListed"), result.Signal)
	var out struct {
		Traces   []traceSummary `json:"traces"`
		Total    int            `json:"total"`
		Offset   int            `json:"offset"`
		PageSize int            `json:"page_size"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &out))
	require.Equal(t, 1, out.PageSize)
	require.Equal(t, 1, out.Offset)
	require.Len(t, out.Traces, 1)
	require.Equal(t, "t2", out.Traces[0].TraceID)
}

func TestGetTraceBuildReadsSeedTraceID(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "collector.ndjson")
	writeSpoolSpans(t, basePath, []testSpan{
		{traceID: "abc", spanID: "s1", service: "svc", name: "op",
			start: time.Unix(1_700_000_000, 0).UTC(), end: time.Unix(1_700_000_001, 0).UTC()},
	})
	seed := core.Result{Output: `{"parameters":{"trace_id":"abc"}}`}
	cmd := GetTraceBuilder{
		ToolName: "spool_get_trace",
		Config:   QueryGetConfig{Path: basePath},
	}.Build(seed)
	result := cmd.Execute()
	require.Equal(t, core.Signal("TraceRetrieved"), result.Signal)
	var out struct {
		TraceID   string       `json:"trace_id"`
		Spans     []spanDetail `json:"spans"`
		SpanCount int          `json:"span_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &out))
	require.Equal(t, "abc", out.TraceID)
	require.Equal(t, 1, out.SpanCount)
}

func TestListTracesEmptyPathError(t *testing.T) {
	result := ListTracesBuilder{
		ToolName: "spool_list_traces",
		Config:   QueryListConfig{Path: ""},
	}.Build(core.Result{}).Execute()

	require.Equal(t, core.CommandError, result.Signal)
}

type testSpan struct {
	traceID, spanID, parentID, service, name string
	start, end                               time.Time
}

func writeSpoolSpans(t *testing.T, path string, spans []testSpan) []byte {
	t.Helper()
	var data []byte
	for _, s := range spans {
		line := map[string]any{
			"Name":        s.name,
			"SpanContext": map[string]any{"TraceID": s.traceID, "SpanID": s.spanID},
			"Parent":      map[string]any{"TraceID": s.traceID, "SpanID": s.parentID},
			"StartTime":   s.start,
			"EndTime":     s.end,
			"Status":      map[string]any{"Code": 0, "Description": ""},
			"Attributes":  []any{},
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
	return data
}
