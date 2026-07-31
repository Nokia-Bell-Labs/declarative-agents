// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectorSpoolModeConformance(t *testing.T) {
	RequireCoreRoot(t)
	controlAddr := FreeAddr(t)
	monitorAddr := FreeAddr(t)
	queryAddr := FreeAddr(t)
	receiverAddr := FreeAddr(t)

	profilePath := CopyShippedProfile(t, filepath.Join("agents", "collector", "profile.yaml"), map[string]string{
		"127.0.0.1:${COLLECTOR_CONTROL_PORT:-18191}": controlAddr,
		"127.0.0.1:${COLLECTOR_MONITOR_PORT:-18192}":  monitorAddr,
		"127.0.0.1:${COLLECTOR_QUERY_PORT:-18193}":     queryAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_CONTROL_PORT:-18191}": controlAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_MONITOR_PORT:-18192}": monitorAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_QUERY_PORT:-18193}":   queryAddr,
		"0.0.0.0:4317": receiverAddr,
	})

	server := Serve(t, ServeConfig{Profile: profilePath})
	server.WaitHealthy("http://"+controlAddr+"/api/lifecycle/health", 15*time.Second)

	if status := server.Post("http://"+controlAddr+"/api/lifecycle/exit", `{"reason":"conformance"}`); status != http.StatusAccepted {
		t.Fatalf("control exit POST status = %d, want %d", status, http.StatusAccepted)
	}
	result := server.WaitExit(35 * time.Second)

	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireNoErrorSpans(t)
	result.RequireToolSpans(t,
		"otlp_receiver_launch",
		"launch_collector_control",
		"launch_collector_monitor",
		"launch_collector_query",
		"await_spans",
		"await_collector_control",
		"exit_agent",
		"otlp_receiver_stop",
		"stop_collector_query",
		"stop_collector_monitor",
		"stop_collector_control",
	)
	result.RequireTerminalState(t, "Done")
}

func TestCollectorQueryListTraces(t *testing.T) {
	RequireCoreRoot(t)
	controlAddr := FreeAddr(t)
	monitorAddr := FreeAddr(t)
	queryAddr := FreeAddr(t)
	receiverAddr := FreeAddr(t)

	profilePath := CopyShippedProfile(t, filepath.Join("agents", "collector", "profile.yaml"), map[string]string{
		"127.0.0.1:${COLLECTOR_CONTROL_PORT:-18191}": controlAddr,
		"127.0.0.1:${COLLECTOR_MONITOR_PORT:-18192}":  monitorAddr,
		"127.0.0.1:${COLLECTOR_QUERY_PORT:-18193}":     queryAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_CONTROL_PORT:-18191}": controlAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_MONITOR_PORT:-18192}": monitorAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_QUERY_PORT:-18193}":   queryAddr,
		"0.0.0.0:4317": receiverAddr,
	})

	spoolDir := filepath.Dir(profilePath)
	seedCollectorSpool(t, filepath.Join(spoolDir, "traces", "collector.ndjson"))

	server := Serve(t, ServeConfig{Profile: profilePath})
	server.WaitHealthy("http://"+controlAddr+"/api/lifecycle/health", 15*time.Second)

	resp, err := http.Get("http://" + queryAddr + "/query/traces?page_size=1")
	if err != nil {
		t.Fatalf("GET /query/traces: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /query/traces status = %d, body = %s", resp.StatusCode, body)
	}
	var listResult struct {
		Traces   []json.RawMessage `json:"traces"`
		Total    int               `json:"total"`
		PageSize int               `json:"page_size"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResult); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listResult.Total != 2 {
		t.Errorf("total = %d, want 2", listResult.Total)
	}
	if listResult.PageSize != 1 {
		t.Errorf("page_size = %d, want 1 (request clamped)", listResult.PageSize)
	}
	if len(listResult.Traces) != 1 {
		t.Errorf("traces returned = %d, want 1", len(listResult.Traces))
	}

	if status := server.Post("http://"+controlAddr+"/api/lifecycle/exit", `{"reason":"conformance"}`); status != http.StatusAccepted {
		t.Fatalf("exit POST status = %d", status)
	}
	server.WaitExit(35 * time.Second)
}

func TestCollectorQueryGetTrace(t *testing.T) {
	RequireCoreRoot(t)
	controlAddr := FreeAddr(t)
	monitorAddr := FreeAddr(t)
	queryAddr := FreeAddr(t)
	receiverAddr := FreeAddr(t)

	profilePath := CopyShippedProfile(t, filepath.Join("agents", "collector", "profile.yaml"), map[string]string{
		"127.0.0.1:${COLLECTOR_CONTROL_PORT:-18191}": controlAddr,
		"127.0.0.1:${COLLECTOR_MONITOR_PORT:-18192}":  monitorAddr,
		"127.0.0.1:${COLLECTOR_QUERY_PORT:-18193}":     queryAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_CONTROL_PORT:-18191}": controlAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_MONITOR_PORT:-18192}": monitorAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_QUERY_PORT:-18193}":   queryAddr,
		"0.0.0.0:4317": receiverAddr,
	})

	spoolDir := filepath.Dir(profilePath)
	seedCollectorSpool(t, filepath.Join(spoolDir, "traces", "collector.ndjson"))

	server := Serve(t, ServeConfig{Profile: profilePath})
	server.WaitHealthy("http://"+controlAddr+"/api/lifecycle/health", 15*time.Second)

	resp, err := http.Get("http://" + queryAddr + "/query/traces/trace-aaa")
	if err != nil {
		t.Fatalf("GET /query/traces/trace-aaa: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /query/traces/trace-aaa status = %d, body = %s", resp.StatusCode, body)
	}
	var getResult struct {
		TraceID   string            `json:"trace_id"`
		Spans     []json.RawMessage `json:"spans"`
		SpanCount int               `json:"span_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&getResult); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if getResult.TraceID != "trace-aaa" {
		t.Errorf("trace_id = %q, want %q", getResult.TraceID, "trace-aaa")
	}
	if getResult.SpanCount != 2 {
		t.Errorf("span_count = %d, want 2", getResult.SpanCount)
	}

	if status := server.Post("http://"+controlAddr+"/api/lifecycle/exit", `{"reason":"conformance"}`); status != http.StatusAccepted {
		t.Fatalf("exit POST status = %d", status)
	}
	server.WaitExit(35 * time.Second)
}

func seedCollectorSpool(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir spool: %v", err)
	}
	spans := []map[string]any{
		{
			"Name":        "root-a",
			"SpanContext": map[string]any{"TraceID": "trace-aaa", "SpanID": "span-1"},
			"Parent":      map[string]any{"TraceID": "trace-aaa", "SpanID": ""},
			"StartTime":   "2026-01-01T00:00:00Z",
			"EndTime":     "2026-01-01T00:00:02Z",
			"Status":      map[string]any{"Code": 0, "Description": ""},
			"Attributes":  []any{},
			"Resource":    []map[string]any{{"Key": "service.name", "Value": map[string]any{"Type": "STRING", "Value": "svc-a"}}},
		},
		{
			"Name":        "child-a",
			"SpanContext": map[string]any{"TraceID": "trace-aaa", "SpanID": "span-2"},
			"Parent":      map[string]any{"TraceID": "trace-aaa", "SpanID": "span-1"},
			"StartTime":   "2026-01-01T00:00:00.5Z",
			"EndTime":     "2026-01-01T00:00:01Z",
			"Status":      map[string]any{"Code": 0, "Description": ""},
			"Attributes":  []any{},
			"Resource":    []map[string]any{{"Key": "service.name", "Value": map[string]any{"Type": "STRING", "Value": "svc-a"}}},
		},
		{
			"Name":        "root-b",
			"SpanContext": map[string]any{"TraceID": "trace-bbb", "SpanID": "span-3"},
			"Parent":      map[string]any{"TraceID": "trace-bbb", "SpanID": ""},
			"StartTime":   "2026-01-01T00:01:00Z",
			"EndTime":     "2026-01-01T00:01:01Z",
			"Status":      map[string]any{"Code": 0, "Description": ""},
			"Attributes":  []any{},
			"Resource":    []map[string]any{{"Key": "service.name", "Value": map[string]any{"Type": "STRING", "Value": "svc-b"}}},
		},
	}
	var data []byte
	for _, s := range spans {
		line, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("marshal spool span: %v", err)
		}
		data = append(data, line...)
		data = append(data, '\n')
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write spool: %v", err)
	}
}
