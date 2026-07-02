// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
)

func TestRecordDeclaredToolMetricsRecordsDiagnosticsForMissingSources(t *testing.T) {
	t.Parallel()
	store := monitor.NewStore(monitor.Limits{Diagnostics: 5, Samples: 5})
	rec := monitor.NewRecorder(store, nil)
	cfg := MetricConfig{Instruments: []MetricInstrument{{
		Name: "exec.exit_code", Kind: "gauge", Unit: "1",
		Description: "Exit code.", ValueSource: "exit_code",
	}}}

	RecordDeclaredToolMetrics(context.Background(), rec, "build", cfg, nil, nil)

	snapshot := store.Snapshot()
	require.Empty(t, snapshot.RecentSamples)
	require.Len(t, snapshot.Diagnostics, 1)
	require.Equal(t, "exec.exit_code", snapshot.Diagnostics[0].Metric)
	require.Contains(t, snapshot.Diagnostics[0].Message, "value source unavailable")
}

func TestRecordDeclaredToolMetricsOmitsUndeclaredAndDisallowedAttributes(t *testing.T) {
	t.Parallel()
	rec := &coreMetricRecorder{}
	cfg := MetricConfig{
		Instruments: []MetricInstrument{{
			Name: "rest.http_status_code", Kind: "gauge", Unit: "1",
			Description: "HTTP status.", ValueSource: "http_status_code",
			Attributes: []string{"operation", "url", "missing"},
		}},
		Attributes: []MetricAttribute{
			{Name: "operation", Source: "configured_operation", Cardinality: "bounded", AllowedValues: []string{"get"}, Redaction: "none"},
			{Name: "url", Source: "config_literal", Cardinality: "low", Redaction: "omit"},
		},
	}

	RecordDeclaredToolMetrics(context.Background(), rec, "rest_client_get", cfg,
		map[string]float64{"http_status_code": 200},
		map[string]string{"configured_operation": "get", "url": "https://example.invalid"},
	)

	require.Len(t, rec.samples, 1)
	require.Equal(t, map[string]string{"operation": "get"}, rec.samples[0].Attributes)
}

type coreMetricRecorder struct {
	samples []monitor.MetricSample
}

func (r *coreMetricRecorder) RecordMetric(_ context.Context, sample monitor.MetricSample) error {
	r.samples = append(r.samples, sample)
	return nil
}
