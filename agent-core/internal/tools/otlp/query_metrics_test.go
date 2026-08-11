// Copyright (c) 2026 Nokia. All rights reserved.

package otlp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
)

// spoolMetricBatch appends one metric batch to path through spool_metrics,
// the same word that writes the spool a query reads back.
func spoolMetricBatch(t *testing.T, path string, request *colmetricpb.ExportMetricsServiceRequest) {
	t.Helper()
	awaitJSON, err := awaitMetricOutput(MetricBatch{
		ID: "b", Request: request, Received: time.Now().UTC(),
	})
	require.NoError(t, err)
	result := SpoolMetricsBuilder{
		ToolName: "spool_metrics",
		Config:   SpoolConfig{Path: path, BatchSource: "$.batch"},
	}.Build(core.Result{Output: awaitJSON}).Execute()
	require.Equal(t, core.Signal("MetricsSpooled"), result.Signal, result.Output)
}

func TestSpoolListMetricsSummaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.ndjson")
	spoolMetricBatch(t, path, metricRequest("chatbot", "dispatch_count", 3))
	spoolMetricBatch(t, path, metricRequest("rag0", "dispatch_count", 2))
	spoolMetricBatch(t, path, metricRequest("dolt", "dss_rows", 1))

	result := ListMetricsBuilder{
		ToolName: "spool_list_metrics",
		Config:   QueryListMetricsConfig{Path: path, PageSize: 20, MaxPageSize: 100},
	}.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("MetricsListed"), result.Signal, result.Output)

	var output struct {
		Metrics []metricSummary `json:"metrics"`
		Total   int             `json:"total"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Equal(t, 2, output.Total) // two distinct metric names
	// Sorted by name: dispatch_count, dss_rows.
	require.Equal(t, "dispatch_count", output.Metrics[0].Name)
	require.Equal(t, "Sum", output.Metrics[0].Type)
	require.Equal(t, 2, output.Metrics[0].RecordCount)
	require.Equal(t, 5, output.Metrics[0].DataPointCount)
	require.Equal(t, []string{"chatbot", "rag0"}, output.Metrics[0].Services)
	require.Equal(t, "dss_rows", output.Metrics[1].Name)
}

func TestSpoolListMetricsPagination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.ndjson")
	for _, name := range []string{"a", "b", "c", "d"} {
		spoolMetricBatch(t, path, metricRequest("svc", name, 1))
	}
	result := ListMetricsBuilder{
		ToolName: "spool_list_metrics",
		Config:   QueryListMetricsConfig{Path: path, PageSize: 2, MaxPageSize: 100, Offset: 1},
	}.Build(core.Result{}).Execute()
	var output struct {
		Metrics  []metricSummary `json:"metrics"`
		Total    int             `json:"total"`
		Offset   int             `json:"offset"`
		PageSize int             `json:"page_size"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Equal(t, 4, output.Total)
	require.Equal(t, 1, output.Offset)
	require.Equal(t, 2, output.PageSize)
	require.Len(t, output.Metrics, 2)
	require.Equal(t, "b", output.Metrics[0].Name) // sorted, offset 1
	require.Equal(t, "c", output.Metrics[1].Name)
}

func TestSpoolListMetricsSeedOverridesPageSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.ndjson")
	for _, name := range []string{"a", "b", "c"} {
		spoolMetricBatch(t, path, metricRequest("svc", name, 1))
	}
	seed := core.Result{Output: `{"parameters":{"page_size":1}}`}
	result := ListMetricsBuilder{
		ToolName: "spool_list_metrics",
		Config:   QueryListMetricsConfig{Path: path, PageSize: 20, MaxPageSize: 100},
	}.Build(seed).Execute()
	var output struct {
		Metrics  []metricSummary `json:"metrics"`
		PageSize int             `json:"page_size"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Equal(t, 1, output.PageSize)
	require.Len(t, output.Metrics, 1)
}

func TestSpoolGetMetricByName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.ndjson")
	chatbotRequest := metricRequest("chatbot", "dispatch_count", 3)
	chatbotRequest.ResourceMetrics[0].Resource.Attributes = append(
		chatbotRequest.ResourceMetrics[0].Resource.Attributes,
		stringAttribute("test.run.id", "run-detail-42"),
	)
	spoolMetricBatch(t, path, chatbotRequest)
	spoolMetricBatch(t, path, metricRequest("rag0", "dispatch_count", 2))
	spoolMetricBatch(t, path, metricRequest("dolt", "dss_rows", 1))

	result := GetMetricBuilder{
		ToolName: "spool_get_metric",
		Config:   QueryGetMetricConfig{Path: path, MetricName: "dispatch_count"},
	}.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("MetricRetrieved"), result.Signal, result.Output)

	var output struct {
		MetricName     string         `json:"metric_name"`
		Records        []metricDetail `json:"records"`
		Total          int            `json:"total"`
		RecordCount    int            `json:"record_count"`
		DataPointCount int            `json:"data_point_count"`
		Offset         int            `json:"offset"`
		PageSize       int            `json:"page_size"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Equal(t, "dispatch_count", output.MetricName)
	require.Equal(t, 2, output.Total)
	require.Equal(t, 2, output.RecordCount)
	require.Equal(t, 5, output.DataPointCount)
	require.Equal(t, 0, output.Offset)
	require.Equal(t, 20, output.PageSize)
	// Sorted by service: chatbot, rag0.
	require.Equal(t, "chatbot", output.Records[0].Service)
	require.Equal(t, "rag0", output.Records[1].Service)
	require.Equal(t, []map[string]any{
		{"Key": "integration.run_id", "Value": map[string]any{"Type": "STRING", "Value": "run-42"}},
		{"Key": "service.name", "Value": map[string]any{"Type": "STRING", "Value": "chatbot"}},
		{"Key": "test.run.id", "Value": map[string]any{"Type": "STRING", "Value": "run-detail-42"}},
	}, output.Records[0].Resource)
	require.NotNil(t, output.Records[0].Metric)
}

func TestSpoolGetMetricPagination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.ndjson")
	for _, service := range []string{"a", "b", "c", "d"} {
		spoolMetricBatch(t, path, metricRequest(service, "hits", 1))
	}
	result := GetMetricBuilder{
		ToolName: "spool_get_metric",
		Config: QueryGetMetricConfig{
			Path: path, MetricName: "hits",
			PageSize: 2, MaxPageSize: 3, Offset: 1,
		},
	}.Build(core.Result{}).Execute()
	var output struct {
		Records         []metricDetail `json:"records"`
		Total           int            `json:"total"`
		RecordCount     int            `json:"record_count"`
		PageRecordCount int            `json:"page_record_count"`
		Offset          int            `json:"offset"`
		PageSize        int            `json:"page_size"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Equal(t, 4, output.Total)
	require.Equal(t, 4, output.RecordCount)
	require.Equal(t, 2, output.PageRecordCount)
	require.Equal(t, 1, output.Offset)
	require.Equal(t, 2, output.PageSize)
	require.Equal(t, "b", output.Records[0].Service)
	require.Equal(t, "c", output.Records[1].Service)
}

func TestSpoolGetMetricDefaultPageReportsTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.ndjson")
	for index := range defaultPageSize + 5 {
		spoolMetricBatch(t, path, metricRequest(fmt.Sprintf("svc-%02d", index), "hits", 1))
	}
	result := GetMetricBuilder{
		ToolName: "spool_get_metric",
		Config:   QueryGetMetricConfig{Path: path, MetricName: "hits"},
	}.Build(core.Result{}).Execute()
	var output struct {
		Records         []metricDetail `json:"records"`
		RecordCount     int            `json:"record_count"`
		PageRecordCount int            `json:"page_record_count"`
		Total           int            `json:"total"`
		PageSize        int            `json:"page_size"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Len(t, output.Records, defaultPageSize)
	require.Equal(t, defaultPageSize+5, output.RecordCount)
	require.Equal(t, output.RecordCount, output.Total)
	require.Equal(t, defaultPageSize, output.PageRecordCount)
	require.Equal(t, defaultPageSize, output.PageSize)
}

func TestSpoolGetMetricSeedCapsAndFinalPage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.ndjson")
	for _, service := range []string{"a", "b", "c", "d"} {
		spoolMetricBatch(t, path, metricRequest(service, "hits", 1))
	}
	seed := core.Result{Output: `{"parameters":{"metric_name":"hits","page_size":99,"offset":3}}`}
	result := GetMetricBuilder{
		ToolName: "spool_get_metric",
		Config: QueryGetMetricConfig{
			Path: path, PageSize: 20, MaxPageSize: 2,
		},
	}.Build(seed).Execute()
	var output struct {
		Records  []metricDetail `json:"records"`
		Total    int            `json:"total"`
		Offset   int            `json:"offset"`
		PageSize int            `json:"page_size"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Equal(t, 4, output.Total)
	require.Equal(t, 3, output.Offset)
	require.Equal(t, 2, output.PageSize)
	require.Len(t, output.Records, 1)
	require.Equal(t, "d", output.Records[0].Service)
}

func TestSpoolGetMetricOffsetPastEndIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.ndjson")
	spoolMetricBatch(t, path, metricRequest("svc", "hits", 1))
	result := GetMetricBuilder{
		ToolName: "spool_get_metric",
		Config: QueryGetMetricConfig{
			Path: path, MetricName: "hits", PageSize: 1, MaxPageSize: 2, Offset: 9,
		},
	}.Build(core.Result{}).Execute()
	var output struct {
		Records []metricDetail `json:"records"`
		Total   int            `json:"total"`
		Offset  int            `json:"offset"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	require.Equal(t, 1, output.Total)
	require.Equal(t, 1, output.Offset)
	require.Empty(t, output.Records)
}

func TestSpoolGetMetricSeedAndErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.ndjson")
	spoolMetricBatch(t, path, metricRequest("svc", "hits", 1))

	// Missing metric_name is a CommandError.
	missing := GetMetricBuilder{
		ToolName: "spool_get_metric",
		Config:   QueryGetMetricConfig{Path: path},
	}.Build(core.Result{}).Execute()
	require.Equal(t, core.CommandError, missing.Signal)

	// A machine_request seed supplies metric_name.
	seed := core.Result{Output: `{"parameters":{"metric_name":"hits"}}`}
	seeded := GetMetricBuilder{
		ToolName: "spool_get_metric",
		Config:   QueryGetMetricConfig{Path: path},
	}.Build(seed).Execute()
	require.Equal(t, core.Signal("MetricRetrieved"), seeded.Signal, seeded.Output)
	var output struct {
		RecordCount int `json:"record_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(seeded.Output), &output))
	require.Equal(t, 1, output.RecordCount)
}

func TestSpoolMetricsQuerySkipsMalformedAndEmpty(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.ndjson")
	result := ListMetricsBuilder{
		ToolName: "spool_list_metrics",
		Config:   QueryListMetricsConfig{Path: empty, PageSize: 20, MaxPageSize: 100},
	}.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("MetricsListed"), result.Signal)
	var emptyOut struct {
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &emptyOut))
	require.Equal(t, 0, emptyOut.Total)

	path := filepath.Join(dir, "metrics.ndjson")
	spoolMetricBatch(t, path, metricRequest("svc", "hits", 1))
	appendLine(t, path, "not json\n")
	appendLine(t, path, `{"Name":""}`+"\n") // record with empty name is skipped
	listed := ListMetricsBuilder{
		ToolName: "spool_list_metrics",
		Config:   QueryListMetricsConfig{Path: path, PageSize: 20, MaxPageSize: 100},
	}.Build(core.Result{}).Execute()
	var out struct {
		Total        int `json:"total"`
		SkippedLines int `json:"skipped_lines"`
	}
	require.NoError(t, json.Unmarshal([]byte(listed.Output), &out))
	require.Equal(t, 1, out.Total)
	require.Equal(t, 2, out.SkippedLines)
}

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	require.NoError(t, err)
	_, err = file.WriteString(line)
	require.NoError(t, err)
	require.NoError(t, file.Close())
}
