// Copyright (c) 2026 Nokia. All rights reserved.

package otlp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const (
	// InitSpoolListTraces identifies the paginated trace summary factory.
	InitSpoolListTraces = "spool_list_traces"
	// InitSpoolGetTrace identifies the single-trace span detail factory.
	InitSpoolGetTrace = "spool_get_trace"

	defaultPageSize    = 20
	defaultMaxPageSize = 100
)

// QueryListConfig configures paginated trace summary reads from the spool.
type QueryListConfig struct {
	Path        string
	PageSize    int
	MaxPageSize int
	Offset      int
}

// QueryGetConfig configures single-trace span detail reads from the spool.
type QueryGetConfig struct {
	Path    string
	TraceID string
}

// ListTracesBuilder constructs paginated trace list commands.
type ListTracesBuilder struct {
	ToolName string
	Config   QueryListConfig
}

// Build creates one list-traces command. When previous carries a
// machine_request seed, page_size and offset override config defaults.
func (b ListTracesBuilder) Build(previous core.Result) core.Command {
	cfg := b.Config
	if p := seedParams(previous); p != nil {
		if v, ok := seedInt(p, "page_size"); ok && v > 0 {
			cfg.PageSize = v
		}
		if v, ok := seedInt(p, "offset"); ok {
			cfg.Offset = v
		}
	}
	return &listTracesCommand{toolName: b.ToolName, config: cfg}
}

// GetTraceBuilder constructs single-trace detail commands.
type GetTraceBuilder struct {
	ToolName string
	Config   QueryGetConfig
}

// Build creates one get-trace command. When previous carries a
// machine_request seed, trace_id overrides the config value.
func (b GetTraceBuilder) Build(previous core.Result) core.Command {
	cfg := b.Config
	if p := seedParams(previous); p != nil {
		if v, _ := p["trace_id"].(string); v != "" {
			cfg.TraceID = v
		}
	}
	return &getTraceCommand{toolName: b.ToolName, config: cfg}
}

type listTracesCommand struct {
	toolName string
	config   QueryListConfig
}

func (c *listTracesCommand) Name() string { return c.toolName }

func (c *listTracesCommand) Execute() core.Result {
	spans, skipped, err := readSpoolFiles(c.config.Path)
	if err != nil {
		return receiverError(c.Name(), fmt.Errorf("%s: %w", c.Name(), err))
	}
	summaries := aggregateTraces(spans)
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].StartTime.After(summaries[j].StartTime)
	})
	page, total, offset, pageSize := paginateTraces(summaries, c.config)

	output := struct {
		Traces       []traceSummary `json:"traces"`
		Total        int            `json:"total"`
		Offset       int            `json:"offset"`
		PageSize     int            `json:"page_size"`
		SkippedLines int            `json:"skipped_lines"`
	}{
		Traces: page, Total: total, Offset: offset,
		PageSize: pageSize, SkippedLines: skipped,
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return receiverError(c.Name(), err)
	}
	return core.Result{Signal: core.Signal("TracesListed"), CommandName: c.Name(), Output: string(encoded)}
}

func (c *listTracesCommand) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.Name())
}

type getTraceCommand struct {
	toolName string
	config   QueryGetConfig
}

func (c *getTraceCommand) Name() string { return c.toolName }

func (c *getTraceCommand) Execute() core.Result {
	if c.config.TraceID == "" {
		return receiverError(c.Name(), fmt.Errorf("%s: trace_id is required", c.Name()))
	}
	spans, skipped, err := readSpoolFiles(c.config.Path)
	if err != nil {
		return receiverError(c.Name(), fmt.Errorf("%s: %w", c.Name(), err))
	}
	matched := matchTraceSpans(spans, c.config.TraceID)

	output := struct {
		TraceID      string       `json:"trace_id"`
		Spans        []spanDetail `json:"spans"`
		SpanCount    int          `json:"span_count"`
		SkippedLines int          `json:"skipped_lines"`
	}{
		TraceID: c.config.TraceID, Spans: matched,
		SpanCount: len(matched), SkippedLines: skipped,
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return receiverError(c.Name(), err)
	}
	return core.Result{Signal: core.Signal("TraceRetrieved"), CommandName: c.Name(), Output: string(encoded)}
}

func (c *getTraceCommand) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.Name())
}

type traceSummary struct {
	TraceID      string    `json:"trace_id"`
	RootService  string    `json:"root_service"`
	RootSpanName string    `json:"root_span_name"`
	SpanCount    int       `json:"span_count"`
	StartTime    time.Time `json:"start_time"`
	DurationMs   int64     `json:"duration_ms"`
}

type spanDetail struct {
	SpanID       string          `json:"span_id"`
	ParentSpanID string          `json:"parent_span_id"`
	Service      string          `json:"service"`
	Name         string          `json:"name"`
	StartTime    time.Time       `json:"start_time"`
	EndTime      time.Time       `json:"end_time"`
	Status       spoolStatus     `json:"status"`
	Attributes   json.RawMessage `json:"attributes"`
}

type spoolSpan struct {
	Name        string          `json:"Name"`
	SpanContext spoolSpanCtx    `json:"SpanContext"`
	Parent      spoolSpanCtx    `json:"Parent"`
	StartTime   time.Time       `json:"StartTime"`
	EndTime     time.Time       `json:"EndTime"`
	Attributes  json.RawMessage `json:"Attributes"`
	Status      spoolStatus     `json:"Status"`
	Resource    json.RawMessage `json:"Resource"`
}

type spoolSpanCtx struct {
	TraceID string `json:"TraceID"`
	SpanID  string `json:"SpanID"`
}

type spoolStatus struct {
	Code        int    `json:"Code"`
	Description string `json:"Description"`
}

func paginateTraces(summaries []traceSummary, cfg QueryListConfig) (page []traceSummary, total, offset, pageSize int) {
	pageSize = cfg.PageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	maxPage := cfg.MaxPageSize
	if maxPage <= 0 {
		maxPage = defaultMaxPageSize
	}
	if pageSize > maxPage {
		pageSize = maxPage
	}
	total = len(summaries)
	offset = cfg.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + pageSize
	if end > total {
		end = total
	}
	return summaries[offset:end], total, offset, pageSize
}

func matchTraceSpans(spans []spoolSpan, traceID string) []spanDetail {
	var matched []spanDetail
	for _, s := range spans {
		if s.SpanContext.TraceID == traceID {
			matched = append(matched, spanDetail{
				SpanID:       s.SpanContext.SpanID,
				ParentSpanID: s.Parent.SpanID,
				Service:      serviceFromResource(s.Resource),
				Name:         s.Name,
				StartTime:    s.StartTime,
				EndTime:      s.EndTime,
				Status:       s.Status,
				Attributes:   s.Attributes,
			})
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].StartTime.Before(matched[j].StartTime)
	})
	return matched
}

// readSpoolFiles reads the active spool and all rotated files, returning
// parsed spans and a count of skipped malformed lines.
func readSpoolFiles(basePath string) ([]spoolSpan, int, error) {
	if basePath == "" {
		return nil, 0, fmt.Errorf("spool path is required")
	}
	if info, err := os.Stat(basePath); err == nil && info.IsDir() {
		return nil, 0, fmt.Errorf("spool path %s is a directory; configure the spool file path", basePath)
	}
	paths := discoverSpoolFiles(basePath)
	if len(paths) == 0 {
		return nil, 0, nil
	}
	var allSpans []spoolSpan
	totalSkipped := 0
	for _, path := range paths {
		spans, skipped, err := parseSpoolFile(path)
		if err != nil {
			return nil, 0, fmt.Errorf("read spool %s: %w", path, err)
		}
		allSpans = append(allSpans, spans...)
		totalSkipped += skipped
	}
	return allSpans, totalSkipped, nil
}

func discoverSpoolFiles(basePath string) []string {
	var paths []string
	maxGeneration := 100
	for gen := maxGeneration; gen >= 1; gen-- {
		rotated := rotatedPath(basePath, gen)
		if info, err := os.Stat(rotated); err == nil && info.Mode().IsRegular() {
			paths = append(paths, rotated)
		}
	}
	if info, err := os.Stat(basePath); err == nil && info.Mode().IsRegular() {
		paths = append(paths, basePath)
	}
	return paths
}

func parseSpoolFile(path string) ([]spoolSpan, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = file.Close() }()

	var spans []spoolSpan
	skipped := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var s spoolSpan
		if err := json.Unmarshal(line, &s); err != nil {
			skipped++
			continue
		}
		if s.SpanContext.TraceID == "" && s.SpanContext.SpanID == "" {
			skipped++
			continue
		}
		spans = append(spans, s)
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}
	return spans, skipped, nil
}

func aggregateTraces(spans []spoolSpan) []traceSummary {
	type traceAcc struct {
		spans    []spoolSpan
		earliest time.Time
		latest   time.Time
	}
	traces := make(map[string]*traceAcc)
	for _, s := range spans {
		tid := s.SpanContext.TraceID
		acc, ok := traces[tid]
		if !ok {
			acc = &traceAcc{earliest: s.StartTime, latest: s.EndTime}
			traces[tid] = acc
		}
		acc.spans = append(acc.spans, s)
		if s.StartTime.Before(acc.earliest) {
			acc.earliest = s.StartTime
		}
		if s.EndTime.After(acc.latest) {
			acc.latest = s.EndTime
		}
	}

	summaries := make([]traceSummary, 0, len(traces))
	for tid, acc := range traces {
		root := findRootSpan(acc.spans)
		summaries = append(summaries, traceSummary{
			TraceID:      tid,
			RootService:  serviceFromResource(root.Resource),
			RootSpanName: root.Name,
			SpanCount:    len(acc.spans),
			StartTime:    acc.earliest,
			DurationMs:   acc.latest.Sub(acc.earliest).Milliseconds(),
		})
	}
	return summaries
}

func findRootSpan(spans []spoolSpan) spoolSpan {
	zeroID := "0000000000000000"
	for _, s := range spans {
		if s.Parent.SpanID == "" || s.Parent.SpanID == zeroID {
			return s
		}
	}
	if len(spans) > 0 {
		return spans[0]
	}
	return spoolSpan{}
}

func seedParams(r core.Result) map[string]interface{} {
	if r.Output == "" {
		return nil
	}
	var envelope struct {
		Params map[string]interface{} `json:"parameters"`
	}
	if json.Unmarshal([]byte(r.Output), &envelope) != nil || envelope.Params == nil {
		return nil
	}
	return envelope.Params
}

func seedInt(params map[string]interface{}, key string) (int, bool) {
	v, ok := params[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case string:
		var i int
		if _, err := fmt.Sscanf(n, "%d", &i); err == nil {
			return i, true
		}
	}
	return 0, false
}

func serviceFromResource(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var attrs []struct {
		Key   string `json:"Key"`
		Value struct {
			Value any `json:"Value"`
		} `json:"Value"`
	}
	if json.Unmarshal(raw, &attrs) != nil {
		return ""
	}
	for _, a := range attrs {
		if a.Key == "service.name" {
			if s, ok := a.Value.Value.(string); ok {
				return s
			}
		}
	}
	return ""
}
