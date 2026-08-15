// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const (
	// InitSpoolListMetrics identifies the paginated metric summary factory.
	InitSpoolListMetrics = "spool_list_metrics"
	// InitSpoolGetMetric identifies the single-metric detail factory.
	InitSpoolGetMetric = "spool_get_metric"
)

// QueryListMetricsConfig configures paginated metric summary reads.
type QueryListMetricsConfig struct {
	Path        string
	PageSize    int
	MaxPageSize int
	Offset      int
}

// QueryGetMetricConfig configures single-metric detail reads.
type QueryGetMetricConfig struct {
	Path        string
	MetricName  string
	PageSize    int
	MaxPageSize int
	Offset      int
}

// ListMetricsBuilder constructs paginated metric list commands.
type ListMetricsBuilder struct {
	ToolName string
	Config   QueryListMetricsConfig
}

// Build creates one list-metrics command. When previous carries a
// machine_request seed, page_size and offset override config defaults.
func (b ListMetricsBuilder) Build(previous core.Result) core.Command {
	cfg := b.Config
	if p := seedParams(previous); p != nil {
		if v, ok := seedInt(p, "page_size"); ok && v > 0 {
			cfg.PageSize = v
		}
		if v, ok := seedInt(p, "offset"); ok {
			cfg.Offset = v
		}
	}
	return &listMetricsCommand{toolName: b.ToolName, config: cfg}
}

// GetMetricBuilder constructs single-metric detail commands.
type GetMetricBuilder struct {
	ToolName string
	Config   QueryGetMetricConfig
}

// Build creates one get-metric command. When previous carries a
// machine_request seed, metric_name, page_size, and offset override config
// defaults.
func (b GetMetricBuilder) Build(previous core.Result) core.Command {
	cfg := b.Config
	if p := seedParams(previous); p != nil {
		if v, _ := p["metric_name"].(string); v != "" {
			cfg.MetricName = v
		}
		if v, ok := seedInt(p, "page_size"); ok && v > 0 {
			cfg.PageSize = v
		}
		if v, ok := seedInt(p, "offset"); ok {
			cfg.Offset = v
		}
	}
	return &getMetricCommand{toolName: b.ToolName, config: cfg}
}

type listMetricsCommand struct {
	toolName string
	config   QueryListMetricsConfig
}

func (c *listMetricsCommand) Name() string { return c.toolName }

func (c *listMetricsCommand) Execute() core.Result {
	records, skipped, err := readMetricSpoolFiles(c.config.Path)
	if err != nil {
		return receiverError(c.Name(), fmt.Errorf("%s: %w", c.Name(), err))
	}
	summaries := aggregateMetrics(records)
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Name < summaries[j].Name
	})
	page, total, offset, pageSize := paginateMetrics(summaries, c.config)

	output := struct {
		Metrics      []metricSummary `json:"metrics"`
		Total        int             `json:"total"`
		Offset       int             `json:"offset"`
		PageSize     int             `json:"page_size"`
		SkippedLines int             `json:"skipped_lines"`
	}{
		Metrics: page, Total: total, Offset: offset,
		PageSize: pageSize, SkippedLines: skipped,
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return receiverError(c.Name(), err)
	}
	return core.Result{Signal: core.Signal("MetricsListed"), CommandName: c.Name(), Output: string(encoded)}
}

func (c *listMetricsCommand) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.Name())
}

type getMetricCommand struct {
	toolName string
	config   QueryGetMetricConfig
}

func (c *getMetricCommand) Name() string { return c.toolName }

func (c *getMetricCommand) Execute() core.Result {
	if c.config.MetricName == "" {
		return receiverError(c.Name(), fmt.Errorf("%s: metric_name is required", c.Name()))
	}
	records, skipped, err := readMetricSpoolFiles(c.config.Path)
	if err != nil {
		return receiverError(c.Name(), fmt.Errorf("%s: %w", c.Name(), err))
	}
	matched := matchMetricRecords(records, c.config.MetricName)
	page, total, offset, pageSize := paginateMetricDetails(matched, c.config)

	output := struct {
		MetricName      string         `json:"metric_name"`
		Records         []metricDetail `json:"records"`
		Total           int            `json:"total"`
		RecordCount     int            `json:"record_count"`
		PageRecordCount int            `json:"page_record_count"`
		DataPointCount  int            `json:"data_point_count"`
		Offset          int            `json:"offset"`
		PageSize        int            `json:"page_size"`
		SkippedLines    int            `json:"skipped_lines"`
	}{
		MetricName: c.config.MetricName, Records: page, Total: total,
		RecordCount: total, PageRecordCount: len(page),
		DataPointCount: sumDetailDataPoints(page),
		Offset:         offset, PageSize: pageSize, SkippedLines: skipped,
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return receiverError(c.Name(), err)
	}
	return core.Result{Signal: core.Signal("MetricRetrieved"), CommandName: c.Name(), Output: string(encoded)}
}

func (c *getMetricCommand) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.Name())
}

type metricSummary struct {
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Unit           string   `json:"unit"`
	RecordCount    int      `json:"record_count"`
	DataPointCount int      `json:"data_point_count"`
	Services       []string `json:"services"`
}

type metricDetail struct {
	Name           string           `json:"name"`
	Type           string           `json:"type"`
	Unit           string           `json:"unit"`
	DataPointCount int              `json:"data_point_count"`
	Service        string           `json:"service"`
	Resource       []map[string]any `json:"resource"`
	Metric         json.RawMessage  `json:"metric"`
}

func paginateMetrics(summaries []metricSummary, cfg QueryListMetricsConfig) (page []metricSummary, total, offset, pageSize int) {
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

func paginateMetricDetails(
	records []metricDetail,
	cfg QueryGetMetricConfig,
) (page []metricDetail, total, offset, pageSize int) {
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
	total = len(records)
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
	return records[offset:end], total, offset, pageSize
}

func matchMetricRecords(records []metricRecord, name string) []metricDetail {
	var matched []metricDetail
	for _, record := range records {
		if record.Name != name {
			continue
		}
		matched = append(matched, metricDetail{
			Name: record.Name, Type: record.Type, Unit: record.Unit,
			DataPointCount: record.DataPointCount,
			Service:        serviceFromRecordResource(record.Resource),
			Resource:       record.Resource,
			Metric:         record.Metric,
		})
	}
	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].Service < matched[j].Service
	})
	return matched
}

// readMetricSpoolFiles reads the active metric spool and all rotated files,
// returning parsed metric records and a count of skipped malformed lines.
func readMetricSpoolFiles(basePath string) ([]metricRecord, int, error) {
	if basePath == "" {
		return nil, 0, fmt.Errorf("metric spool path is required")
	}
	if info, err := os.Stat(basePath); err == nil && info.IsDir() {
		return nil, 0, fmt.Errorf("metric spool path %s is a directory; configure the spool file path", basePath)
	}
	paths := discoverSpoolFiles(basePath)
	if len(paths) == 0 {
		return nil, 0, nil
	}
	var all []metricRecord
	totalSkipped := 0
	for _, path := range paths {
		records, skipped, err := parseMetricSpoolFile(path)
		if err != nil {
			return nil, 0, fmt.Errorf("read metric spool %s: %w", path, err)
		}
		all = append(all, records...)
		totalSkipped += skipped
	}
	return all, totalSkipped, nil
}

func parseMetricSpoolFile(path string) ([]metricRecord, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = file.Close() }()

	var records []metricRecord
	skipped := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var record metricRecord
		if err := json.Unmarshal(line, &record); err != nil {
			skipped++
			continue
		}
		if record.Name == "" {
			skipped++
			continue
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}
	return records, skipped, nil
}

func aggregateMetrics(records []metricRecord) []metricSummary {
	type metricAcc struct {
		typeName   string
		unit       string
		records    int
		dataPoints int
		services   map[string]bool
	}
	metrics := make(map[string]*metricAcc)
	for _, record := range records {
		acc, ok := metrics[record.Name]
		if !ok {
			acc = &metricAcc{typeName: record.Type, unit: record.Unit, services: make(map[string]bool)}
			metrics[record.Name] = acc
		}
		acc.records++
		acc.dataPoints += record.DataPointCount
		if service := serviceFromRecordResource(record.Resource); service != "" {
			acc.services[service] = true
		}
	}

	summaries := make([]metricSummary, 0, len(metrics))
	for name, acc := range metrics {
		summaries = append(summaries, metricSummary{
			Name: name, Type: acc.typeName, Unit: acc.unit,
			RecordCount: acc.records, DataPointCount: acc.dataPoints,
			Services: sortedSet(acc.services),
		})
	}
	return summaries
}

func sumDetailDataPoints(details []metricDetail) int {
	total := 0
	for _, detail := range details {
		total += detail.DataPointCount
	}
	return total
}

// serviceFromRecordResource extracts service.name from a spooled metric
// record's already-decoded resource attributes.
func serviceFromRecordResource(resource []map[string]any) string {
	for _, attribute := range resource {
		if key, _ := attribute["Key"].(string); key != "service.name" {
			continue
		}
		value, ok := attribute["Value"].(map[string]any)
		if !ok {
			return ""
		}
		if name, ok := value["Value"].(string); ok {
			return name
		}
	}
	return ""
}
