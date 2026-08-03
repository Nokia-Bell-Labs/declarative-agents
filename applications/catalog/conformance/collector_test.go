// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// collectorEnv pins the OTLP receiver to a free address. The receiver default
// lives in agent-core's builtin declaration (${COLLECTOR_RECEIVER_ADDRESS:-
// 0.0.0.0:4317}), outside the copied profile directory, so profile patching
// cannot reach it and only the environment prevents a bind collision with a
// concurrent run or a live rig.
func collectorEnv(receiverAddr string) []string {
	return []string{"COLLECTOR_RECEIVER_ADDRESS=" + receiverAddr}
}

// occupyDefaultOTLPPort holds 4317 when it is free, proving the test does not
// depend on it; an already-taken port proves the same thing.
func occupyDefaultOTLPPort(t *testing.T) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:4317")
	if err != nil {
		return
	}
	t.Cleanup(func() { _ = listener.Close() })
}

// TestCollectorDefaultEnvironmentBoot proves the shipped profile starts in
// spool mode with no relay endpoint configured (GH-1163): only ports and the
// spool path come from the environment, and no profile file is rewritten.
func TestCollectorDefaultEnvironmentBoot(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	controlPort := freePort(t)
	env := []string{
		"COLLECTOR_RECEIVER_ADDRESS=" + FreeAddr(t),
		"COLLECTOR_CONTROL_PORT=" + controlPort,
		"COLLECTOR_MONITOR_PORT=" + freePort(t),
		"COLLECTOR_QUERY_PORT=" + freePort(t),
		"COLLECTOR_SPOOL_PATH=" + filepath.Join(t.TempDir(), "collector.ndjson"),
	}

	server := Serve(t, ServeConfig{
		Profile: ProfilePath(filepath.Join("agents", "collector", "profile.yaml")),
		Env:     env,
	})
	control := "http://127.0.0.1:" + controlPort
	server.WaitHealthy(control+"/api/lifecycle/health", 15*time.Second)
	if status := server.Post(control+"/api/lifecycle/exit", `{"reason":"conformance"}`); status != http.StatusAccepted {
		t.Fatalf("control exit POST status = %d, want %d", status, http.StatusAccepted)
	}
	result := server.WaitExit(35 * time.Second)
	result.RequireExit(t, 0)
	result.RequireNoErrorSpans(t)
	result.RequireTerminalState(t, "Done")
}

// TestCollectorQueryEmptySpool proves a collector with no traffic answers the
// query surface with an empty list, not an error: the spool file does not
// exist until the first batch arrives, and a directory misconfigured as the
// spool path is reported as such (GH-1168).
func TestCollectorQueryEmptySpool(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	controlPort := freePort(t)
	queryPort := freePort(t)
	env := []string{
		"COLLECTOR_RECEIVER_ADDRESS=" + FreeAddr(t),
		"COLLECTOR_CONTROL_PORT=" + controlPort,
		"COLLECTOR_MONITOR_PORT=" + freePort(t),
		"COLLECTOR_QUERY_PORT=" + queryPort,
		"COLLECTOR_SPOOL_PATH=" + filepath.Join(t.TempDir(), "collector.ndjson"),
	}

	server := Serve(t, ServeConfig{
		Profile: ProfilePath(filepath.Join("agents", "collector", "profile.yaml")),
		Env:     env,
	})
	control := "http://127.0.0.1:" + controlPort
	server.WaitHealthy(control+"/api/lifecycle/health", 15*time.Second)

	resp, err := http.Get("http://127.0.0.1:" + queryPort + "/query/traces")
	if err != nil {
		t.Fatalf("GET /query/traces before any span: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("empty-spool query status = %d, body = %s", resp.StatusCode, body)
	}
	var empty struct {
		Traces []json.RawMessage `json:"traces"`
		Total  int               `json:"total"`
	}
	if err := json.Unmarshal(body, &empty); err != nil {
		t.Fatalf("decode empty-spool response: %v", err)
	}
	if empty.Total != 0 || len(empty.Traces) != 0 {
		t.Fatalf("empty-spool query = %s, want zero traces", body)
	}

	if status := server.Post(control+"/api/lifecycle/exit", `{"reason":"conformance"}`); status != http.StatusAccepted {
		t.Fatalf("exit POST status = %d", status)
	}
	server.WaitExit(35*time.Second).RequireExit(t, 0)
}

func freePort(t *testing.T) string {
	t.Helper()
	_, port, err := net.SplitHostPort(FreeAddr(t))
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func TestCollectorSpoolModeConformance(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	controlAddr := FreeAddr(t)
	monitorAddr := FreeAddr(t)
	queryAddr := FreeAddr(t)
	receiverAddr := FreeAddr(t)

	profilePath := CopyShippedProfile(t, filepath.Join("agents", "collector", "profile.yaml"), map[string]string{
		"127.0.0.1:${COLLECTOR_CONTROL_PORT:-18191}":                         controlAddr,
		"127.0.0.1:${COLLECTOR_MONITOR_PORT:-18192}":                         monitorAddr,
		"127.0.0.1:${COLLECTOR_QUERY_PORT:-18193}":                           queryAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_CONTROL_PORT:-18191}": controlAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_MONITOR_PORT:-18192}": monitorAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_QUERY_PORT:-18193}":   queryAddr,
		"0.0.0.0:4317": receiverAddr,
	})
	occupyDefaultOTLPPort(t)

	server := Serve(t, ServeConfig{Profile: profilePath, Env: collectorEnv(receiverAddr)})
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

// TestCollectorQueryResponseContract pins the JSON keys the query surface
// emits per trace summary and per span. The collector trace UI
// (agents/collector/ui/src/api/client.ts) and the coding-agent smoke verdict
// decode exactly these keys; a drift on either side must fail here first
// (GH-1164).
func TestCollectorQueryResponseContract(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	controlAddr := FreeAddr(t)
	monitorAddr := FreeAddr(t)
	queryAddr := FreeAddr(t)
	receiverAddr := FreeAddr(t)

	profilePath := CopyShippedProfile(t, filepath.Join("agents", "collector", "profile.yaml"), map[string]string{
		"127.0.0.1:${COLLECTOR_CONTROL_PORT:-18191}":                         controlAddr,
		"127.0.0.1:${COLLECTOR_MONITOR_PORT:-18192}":                         monitorAddr,
		"127.0.0.1:${COLLECTOR_QUERY_PORT:-18193}":                           queryAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_CONTROL_PORT:-18191}": controlAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_MONITOR_PORT:-18192}": monitorAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_QUERY_PORT:-18193}":   queryAddr,
		"0.0.0.0:4317": receiverAddr,
	})
	seedCollectorSpool(t, filepath.Join(filepath.Dir(profilePath), "traces", "collector.ndjson"))

	server := Serve(t, ServeConfig{Profile: profilePath, Env: collectorEnv(receiverAddr)})
	server.WaitHealthy("http://"+controlAddr+"/api/lifecycle/health", 15*time.Second)

	assertKeys := func(url, listField string, want []string) {
		t.Helper()
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
		defer func() { _ = resp.Body.Close() }()
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode %s: %v", url, err)
		}
		var items []map[string]json.RawMessage
		if err := json.Unmarshal(payload[listField], &items); err != nil {
			t.Fatalf("decode %s.%s: %v", url, listField, err)
		}
		if len(items) == 0 {
			t.Fatalf("%s returned no %s", url, listField)
		}
		got := make([]string, 0, len(items[0]))
		for key := range items[0] {
			got = append(got, key)
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s %s keys = %v, want %v (keep agents/collector/ui/src/api/client.ts in sync)", url, listField, got, want)
		}
	}
	assertKeys("http://"+queryAddr+"/query/traces?page_size=1", "traces",
		[]string{"duration_ms", "root_service", "root_span_name", "span_count", "start_time", "trace_id"})
	assertKeys("http://"+queryAddr+"/query/traces/trace-aaa", "spans",
		[]string{"attributes", "end_time", "name", "parent_span_id", "service", "span_id", "start_time", "status"})

	if status := server.Post("http://"+controlAddr+"/api/lifecycle/exit", `{"reason":"conformance"}`); status != http.StatusAccepted {
		t.Fatalf("exit POST status = %d", status)
	}
	server.WaitExit(35 * time.Second)
}

func TestCollectorQueryListTraces(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	controlAddr := FreeAddr(t)
	monitorAddr := FreeAddr(t)
	queryAddr := FreeAddr(t)
	receiverAddr := FreeAddr(t)

	profilePath := CopyShippedProfile(t, filepath.Join("agents", "collector", "profile.yaml"), map[string]string{
		"127.0.0.1:${COLLECTOR_CONTROL_PORT:-18191}":                         controlAddr,
		"127.0.0.1:${COLLECTOR_MONITOR_PORT:-18192}":                         monitorAddr,
		"127.0.0.1:${COLLECTOR_QUERY_PORT:-18193}":                           queryAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_CONTROL_PORT:-18191}": controlAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_MONITOR_PORT:-18192}": monitorAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_QUERY_PORT:-18193}":   queryAddr,
		"0.0.0.0:4317": receiverAddr,
	})

	spoolDir := filepath.Dir(profilePath)
	seedCollectorSpool(t, filepath.Join(spoolDir, "traces", "collector.ndjson"))

	server := Serve(t, ServeConfig{Profile: profilePath, Env: collectorEnv(receiverAddr)})
	server.WaitHealthy("http://"+controlAddr+"/api/lifecycle/health", 15*time.Second)

	resp, err := http.Get("http://" + queryAddr + "/query/traces?page_size=1")
	if err != nil {
		t.Fatalf("GET /query/traces: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
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
	t.Parallel()
	RequireCoreRoot(t)
	controlAddr := FreeAddr(t)
	monitorAddr := FreeAddr(t)
	queryAddr := FreeAddr(t)
	receiverAddr := FreeAddr(t)

	profilePath := CopyShippedProfile(t, filepath.Join("agents", "collector", "profile.yaml"), map[string]string{
		"127.0.0.1:${COLLECTOR_CONTROL_PORT:-18191}":                         controlAddr,
		"127.0.0.1:${COLLECTOR_MONITOR_PORT:-18192}":                         monitorAddr,
		"127.0.0.1:${COLLECTOR_QUERY_PORT:-18193}":                           queryAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_CONTROL_PORT:-18191}": controlAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_MONITOR_PORT:-18192}": monitorAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_QUERY_PORT:-18193}":   queryAddr,
		"0.0.0.0:4317": receiverAddr,
	})

	spoolDir := filepath.Dir(profilePath)
	seedCollectorSpool(t, filepath.Join(spoolDir, "traces", "collector.ndjson"))

	server := Serve(t, ServeConfig{Profile: profilePath, Env: collectorEnv(receiverAddr)})
	server.WaitHealthy("http://"+controlAddr+"/api/lifecycle/health", 15*time.Second)

	resp, err := http.Get("http://" + queryAddr + "/query/traces/trace-aaa")
	if err != nil {
		t.Fatalf("GET /query/traces/trace-aaa: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
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

func TestCollectorQueryListMetrics(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	controlAddr := FreeAddr(t)
	monitorAddr := FreeAddr(t)
	queryAddr := FreeAddr(t)
	receiverAddr := FreeAddr(t)

	profilePath := CopyShippedProfile(t, filepath.Join("agents", "collector", "profile.yaml"), map[string]string{
		"127.0.0.1:${COLLECTOR_CONTROL_PORT:-18191}":                         controlAddr,
		"127.0.0.1:${COLLECTOR_MONITOR_PORT:-18192}":                         monitorAddr,
		"127.0.0.1:${COLLECTOR_QUERY_PORT:-18193}":                           queryAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_CONTROL_PORT:-18191}": controlAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_MONITOR_PORT:-18192}": monitorAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_QUERY_PORT:-18193}":   queryAddr,
		"0.0.0.0:4317": receiverAddr,
	})
	seedCollectorMetricSpool(t, filepath.Join(filepath.Dir(profilePath), "metrics", "collector.ndjson"))

	server := Serve(t, ServeConfig{Profile: profilePath, Env: collectorEnv(receiverAddr)})
	server.WaitHealthy("http://"+controlAddr+"/api/lifecycle/health", 15*time.Second)

	resp, err := http.Get("http://" + queryAddr + "/query/metrics?page_size=1")
	if err != nil {
		t.Fatalf("GET /query/metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /query/metrics status = %d, body = %s", resp.StatusCode, body)
	}
	var listResult struct {
		Metrics  []map[string]json.RawMessage `json:"metrics"`
		Total    int                          `json:"total"`
		PageSize int                          `json:"page_size"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResult); err != nil {
		t.Fatalf("decode metric list response: %v", err)
	}
	if listResult.Total != 2 {
		t.Errorf("total = %d, want 2", listResult.Total)
	}
	if listResult.PageSize != 1 {
		t.Errorf("page_size = %d, want 1 (request clamped)", listResult.PageSize)
	}
	if len(listResult.Metrics) != 1 {
		t.Fatalf("metrics returned = %d, want 1", len(listResult.Metrics))
	}
	// Pin the summary key contract a metric UI would decode.
	got := make([]string, 0, len(listResult.Metrics[0]))
	for key := range listResult.Metrics[0] {
		got = append(got, key)
	}
	sort.Strings(got)
	want := []string{"data_point_count", "name", "record_count", "services", "type", "unit"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("metric summary keys = %v, want %v", got, want)
	}

	if status := server.Post("http://"+controlAddr+"/api/lifecycle/exit", `{"reason":"conformance"}`); status != http.StatusAccepted {
		t.Fatalf("exit POST status = %d", status)
	}
	server.WaitExit(35 * time.Second)
}

func TestCollectorQueryGetMetric(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	controlAddr := FreeAddr(t)
	monitorAddr := FreeAddr(t)
	queryAddr := FreeAddr(t)
	receiverAddr := FreeAddr(t)

	profilePath := CopyShippedProfile(t, filepath.Join("agents", "collector", "profile.yaml"), map[string]string{
		"127.0.0.1:${COLLECTOR_CONTROL_PORT:-18191}":                         controlAddr,
		"127.0.0.1:${COLLECTOR_MONITOR_PORT:-18192}":                         monitorAddr,
		"127.0.0.1:${COLLECTOR_QUERY_PORT:-18193}":                           queryAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_CONTROL_PORT:-18191}": controlAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_MONITOR_PORT:-18192}": monitorAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_QUERY_PORT:-18193}":   queryAddr,
		"0.0.0.0:4317": receiverAddr,
	})
	seedCollectorMetricSpool(t, filepath.Join(filepath.Dir(profilePath), "metrics", "collector.ndjson"))

	server := Serve(t, ServeConfig{Profile: profilePath, Env: collectorEnv(receiverAddr)})
	server.WaitHealthy("http://"+controlAddr+"/api/lifecycle/health", 15*time.Second)

	resp, err := http.Get("http://" + queryAddr + "/query/metrics/dispatch_count")
	if err != nil {
		t.Fatalf("GET /query/metrics/dispatch_count: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /query/metrics/dispatch_count status = %d, body = %s", resp.StatusCode, body)
	}
	var getResult struct {
		MetricName     string            `json:"metric_name"`
		Records        []json.RawMessage `json:"records"`
		RecordCount    int               `json:"record_count"`
		DataPointCount int               `json:"data_point_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&getResult); err != nil {
		t.Fatalf("decode metric get response: %v", err)
	}
	if getResult.MetricName != "dispatch_count" {
		t.Errorf("metric_name = %q, want %q", getResult.MetricName, "dispatch_count")
	}
	if getResult.RecordCount != 2 {
		t.Errorf("record_count = %d, want 2", getResult.RecordCount)
	}
	if getResult.DataPointCount != 5 {
		t.Errorf("data_point_count = %d, want 5", getResult.DataPointCount)
	}

	if status := server.Post("http://"+controlAddr+"/api/lifecycle/exit", `{"reason":"conformance"}`); status != http.StatusAccepted {
		t.Fatalf("exit POST status = %d", status)
	}
	server.WaitExit(35 * time.Second)
}

// seedCollectorMetricSpool writes metric NDJSON records in the shape
// spool_metrics emits, so the query surface reads them back without a live
// intake cycle (mirrors seedCollectorSpool for traces).
func seedCollectorMetricSpool(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir metric spool: %v", err)
	}
	resource := func(service string) []map[string]any {
		return []map[string]any{{"Key": "service.name", "Value": map[string]any{"Type": "STRING", "Value": service}}}
	}
	records := []map[string]any{
		{"Name": "dispatch_count", "Type": "Sum", "Unit": "1", "DataPointCount": 3, "Resource": resource("chatbot"), "Scope": map[string]any{"Name": "agent-core"}, "Metric": map[string]any{"name": "dispatch_count"}},
		{"Name": "dispatch_count", "Type": "Sum", "Unit": "1", "DataPointCount": 2, "Resource": resource("rag0"), "Scope": map[string]any{"Name": "agent-core"}, "Metric": map[string]any{"name": "dispatch_count"}},
		{"Name": "dss_rows", "Type": "Gauge", "Unit": "1", "DataPointCount": 1, "Resource": resource("dolt"), "Scope": map[string]any{"Name": "agent-core"}, "Metric": map[string]any{"name": "dss_rows"}},
	}
	var data []byte
	for _, record := range records {
		line, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("marshal metric record: %v", err)
		}
		data = append(data, line...)
		data = append(data, '\n')
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write metric spool: %v", err)
	}
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

// collectorAddrs holds the four loopback addresses a collector run binds.
type collectorAddrs struct{ control, monitor, query, receiver string }

// copyCollectorProfileWithPorts copies the shipped collector profile tree
// (including ui/dist) into a temp dir and patches its listen addresses to
// free ports. The copy's directory is the working directory the served UI
// root (ui/dist) resolves against.
func copyCollectorProfileWithPorts(t *testing.T) (string, collectorAddrs) {
	t.Helper()
	a := collectorAddrs{control: FreeAddr(t), monitor: FreeAddr(t), query: FreeAddr(t), receiver: FreeAddr(t)}
	profilePath := CopyShippedProfile(t, filepath.Join("agents", "collector", "profile.yaml"), map[string]string{
		"127.0.0.1:${COLLECTOR_CONTROL_PORT:-18191}":                         a.control,
		"127.0.0.1:${COLLECTOR_MONITOR_PORT:-18192}":                         a.monitor,
		"127.0.0.1:${COLLECTOR_QUERY_PORT:-18193}":                           a.query,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_CONTROL_PORT:-18191}": a.control,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_MONITOR_PORT:-18192}": a.monitor,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_QUERY_PORT:-18193}":   a.query,
		"0.0.0.0:4317": a.receiver,
	})
	return profilePath, a
}

// serveCollectorUI launches the copied collector with its working directory set
// to the profile directory, so the literal ui/dist static root resolves against
// the profile (srd020 R7.4), and waits until the control server is healthy.
func serveCollectorUI(t *testing.T, profilePath string, a collectorAddrs) *Server {
	t.Helper()
	server := Serve(t, ServeConfig{Profile: profilePath, WorkDir: filepath.Dir(profilePath), Env: collectorEnv(a.receiver)})
	server.WaitHealthy("http://"+a.control+"/api/lifecycle/health", 15*time.Second)
	return server
}

// getBody issues a GET and returns the body, content type, and status.
func getBody(t *testing.T, url string) (string, string, int) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return string(body), resp.Header.Get("Content-Type"), resp.StatusCode
}

// stopCollector posts the lifecycle exit and drains the run.
func stopCollector(t *testing.T, server *Server, controlAddr string) {
	t.Helper()
	if status := server.Post("http://"+controlAddr+"/api/lifecycle/exit", `{"reason":"conformance"}`); status != http.StatusAccepted {
		t.Fatalf("exit POST status = %d", status)
	}
	server.WaitExit(35 * time.Second)
}

// TestCollectorServesUIIndex proves the collector_query server serves the
// shipped SPA index at / from the ui/dist root, same-origin with /query/*
// (srd020 R7.1).
func TestCollectorServesUIIndex(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	profilePath, a := copyCollectorProfileWithPorts(t)
	server := serveCollectorUI(t, profilePath, a)

	body, ctype, status := getBody(t, "http://"+a.query+"/")
	if status != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200; body:\n%s", status, body)
	}
	if !strings.HasPrefix(ctype, "text/html") {
		t.Errorf("GET / content-type = %q, want text/html", ctype)
	}
	if !strings.Contains(body, "Collector Traces") {
		t.Errorf("GET / body is not the SPA index; got:\n%s", body)
	}
	stopCollector(t, server, a.control)
}

// TestCollectorServesUIDeepLink proves a client-router deep link that is not a
// real file resolves to the SPA index via spa fallback, so the browser can
// render the waterfall route (srd020 R7.2).
func TestCollectorServesUIDeepLink(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	profilePath, a := copyCollectorProfileWithPorts(t)
	server := serveCollectorUI(t, profilePath, a)

	body, ctype, status := getBody(t, "http://"+a.query+"/traces/trace-aaa")
	if status != http.StatusOK {
		t.Fatalf("GET /traces/trace-aaa status = %d, want 200; body:\n%s", status, body)
	}
	if !strings.HasPrefix(ctype, "text/html") {
		t.Errorf("deep-link content-type = %q, want text/html", ctype)
	}
	if !strings.Contains(body, "Collector Traces") {
		t.Errorf("deep link did not fall back to the SPA index; got:\n%s", body)
	}
	stopCollector(t, server, a.control)
}

// TestCollectorQueryUnaffectedByUI proves the /query/* API keeps its response
// contract while the UI is served: the literal query routes outrank the
// static catch-all, so serving the SPA changes no query response (srd020 R7.3).
func TestCollectorQueryUnaffectedByUI(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	profilePath, a := copyCollectorProfileWithPorts(t)
	seedCollectorSpool(t, filepath.Join(filepath.Dir(profilePath), "traces", "collector.ndjson"))
	server := serveCollectorUI(t, profilePath, a)

	// The SPA is served at root.
	if _, _, status := getBody(t, "http://"+a.query+"/"); status != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", status)
	}
	// The query API answers its pinned contract, not the SPA.
	body, ctype, status := getBody(t, "http://"+a.query+"/query/traces?page_size=1")
	if status != http.StatusOK {
		t.Fatalf("GET /query/traces status = %d, want 200; body:\n%s", status, body)
	}
	if !strings.HasPrefix(ctype, "application/json") {
		t.Fatalf("GET /query/traces content-type = %q, want application/json (catch-all intercepted the query route)", ctype)
	}
	var listResult struct {
		Traces   []map[string]json.RawMessage `json:"traces"`
		Total    int                          `json:"total"`
		PageSize int                          `json:"page_size"`
	}
	if err := json.Unmarshal([]byte(body), &listResult); err != nil {
		t.Fatalf("decode /query/traces: %v; body:\n%s", err, body)
	}
	if listResult.Total != 2 || listResult.PageSize != 1 || len(listResult.Traces) != 1 {
		t.Fatalf("query response drifted: total=%d page_size=%d returned=%d, want 2/1/1",
			listResult.Total, listResult.PageSize, len(listResult.Traces))
	}
	got := make([]string, 0, len(listResult.Traces[0]))
	for key := range listResult.Traces[0] {
		got = append(got, key)
	}
	sort.Strings(got)
	want := []string{"duration_ms", "root_service", "root_span_name", "span_count", "start_time", "trace_id"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("summary keys = %v, want %v", got, want)
	}
	stopCollector(t, server, a.control)
}

// TestCollectorMissingUIRoot proves an absent UI root yields an explicit error
// status rather than a silent empty 200, and the query API still works
// (srd020 R7.5).
func TestCollectorMissingUIRoot(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	profilePath, a := copyCollectorProfileWithPorts(t)
	if err := os.RemoveAll(filepath.Join(filepath.Dir(profilePath), "ui", "dist")); err != nil {
		t.Fatalf("remove ui/dist: %v", err)
	}
	server := serveCollectorUI(t, profilePath, a)

	body, _, status := getBody(t, "http://"+a.query+"/")
	if status == http.StatusOK {
		t.Fatalf("GET / with missing UI root returned 200 (blank page); want an explicit error. body:\n%s", body)
	}
	// The query API is unaffected by the missing UI root.
	if _, _, qstatus := getBody(t, "http://"+a.query+"/query/traces"); qstatus != http.StatusOK {
		t.Fatalf("GET /query/traces status = %d, want 200 even with UI root absent", qstatus)
	}
	stopCollector(t, server, a.control)
}

// TestCollectorUIRootLiteral proves the shipped rest.yaml declares the UI root
// as a literal path with no environment-variable expansion and introduces no
// new UI environment variable (srd020 R7.4, GH-1228).
func TestCollectorUIRootLiteral(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(ProfilePath(filepath.Join("agents", "collector", "rest.yaml")))
	if err != nil {
		t.Fatalf("read collector rest.yaml: %v", err)
	}
	rest := string(data)
	if !strings.Contains(rest, "root: ui/dist") {
		t.Fatalf("collector rest.yaml does not declare the literal static root 'ui/dist'")
	}
	for _, line := range strings.Split(rest, "\n") {
		if strings.Contains(line, "root:") && strings.Contains(line, "ui/dist") && strings.Contains(line, "${") {
			t.Fatalf("collector UI root uses an environment variable: %q", strings.TrimSpace(line))
		}
	}
	if strings.Contains(rest, "COLLECTOR_UI_ROOT") {
		t.Fatalf("collector rest.yaml introduces a new UI environment variable")
	}
}
