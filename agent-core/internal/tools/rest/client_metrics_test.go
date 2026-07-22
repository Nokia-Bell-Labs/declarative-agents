// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRESTClientRecordsMonitorMetrics(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"title": "ok"})
	}))
	defer upstream.Close()
	rec := &restMetricRecorder{}
	cmd := clientCommand(t, clientDefinition(t, upstream.URL, issueClient()), InitClientGet, "get", params("1"))
	cmd.(core.MonitorRecorderAware).SetMonitorRecorder(rec)

	result := cmd.Execute()

	require.Equal(t, core.Signal("RESTResourceRead"), result.Signal, result.Output)
	requireRestMetric(t, rec.samples, "rest.http_status_code", 200)
	requireRestMetric(t, rec.samples, "rest.retry_count", 0)
	requirePositiveRestMetric(t, rec.samples, "rest.response_bytes")
	for _, sample := range rec.samples {
		require.Equal(t, "get", sample.Attributes["operation"])
		require.NotContains(t, sample.Attributes, "url")
		require.NotContains(t, sample.Attributes, "authorization")
	}
}

func TestRESTClientMetricConfigCanDisableSamples(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"title": "ok"})
	}))
	defer upstream.Close()
	rec := &restMetricRecorder{}
	cmd := clientCommandWithMetrics(
		t,
		clientDefinition(t, upstream.URL, issueClient()),
		InitClientGet,
		"get",
		params("1"),
		core.MetricConfig{Disabled: true},
	)
	cmd.(core.MonitorRecorderAware).SetMonitorRecorder(rec)

	result := cmd.Execute()

	require.Equal(t, core.Signal("RESTResourceRead"), result.Signal, result.Output)
	require.Empty(t, rec.samples)
}

func TestRESTClientMetricsCarryDispatchEnvelope(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"title": "ok"})
	}))
	defer upstream.Close()
	cmd := clientCommand(t, clientDefinition(t, upstream.URL, issueClient()), InitClientGet, "get", params("1"))

	samples := runRESTMetricLoop(t, cmd, core.Signal("RESTResourceRead"))

	requireRestMetric(t, samples, "rest.http_status_code", 200)
	requireRESTEnvelope(t, samples, "rest.http_status_code", cmd.Name())
}
