// Copyright (c) 2026 Nokia. All rights reserved.

package telemetry

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type recordingExporter struct {
	name        string
	shutdownErr error
	record      func(string)
}

type recordingMetricExporter struct {
	recordingExporter
}

func (e *recordingExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error {
	return nil
}

func (e *recordingExporter) Shutdown(context.Context) error {
	e.record(e.name)
	return e.shutdownErr
}

func (e *recordingMetricExporter) Temporality(kind sdkmetric.InstrumentKind) metricdata.Temporality {
	return sdkmetric.DefaultTemporalitySelector(kind)
}

func (e *recordingMetricExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(kind)
}

func (e *recordingMetricExporter) Export(context.Context, *metricdata.ResourceMetrics) error {
	return nil
}

func (e *recordingMetricExporter) ForceFlush(context.Context) error {
	return nil
}

func TestNewRoot_EmptyConfigError(t *testing.T) {
	t.Parallel()
	_, _, err := NewRoot("test", "root", ExporterConfig{}, context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one exporter required")
}

func TestMetricOTLPEndpoint(t *testing.T) {
	t.Parallel()
	require.Equal(t, "trace:4317", metricOTLPEndpoint(ExporterConfig{OTLPEndpoint: "trace:4317"}))
	require.Equal(t, "metrics:4317", metricOTLPEndpoint(ExporterConfig{
		OTLPEndpoint:       "trace:4317",
		MetricOTLPEndpoint: "metrics:4317",
	}))
}

func TestNewRoot_FileExporter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.json")

	tr, shutdown, err := NewRoot("myagent", "test-root", ExporterConfig{FilePath: path}, context.Background())
	require.NoError(t, err)
	require.NotNil(t, shutdown)

	tr.Event("hello", attribute.String("key", "value"))
	shutdown()

	_, err = os.Stat(path)
	require.NoError(t, err, "trace file should exist after shutdown")
}

func TestNewRoot_MergesEnvironmentResourceAndKeepsExplicitServiceName(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES",
		"test.repository=declarative-agents,test.module=agent-core,test.target=unit,test.run.id=run-123,service.name=environment-name")
	path := filepath.Join(t.TempDir(), "trace.json")

	_, shutdown, err := NewRoot("explicit-name", "test-root",
		ExporterConfig{FilePath: path}, context.Background())
	require.NoError(t, err)
	shutdown()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	output := string(data)
	for _, value := range []string{
		"test.repository", "declarative-agents",
		"test.module", "agent-core",
		"test.target", "unit",
		"test.run.id", "run-123",
		"service.name", "explicit-name",
	} {
		require.Contains(t, output, value)
	}
	require.NotContains(t, output, "environment-name")
}

func TestNewRoot_TempFileUsesServiceName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.json")

	_, shutdown, err := NewRoot("planner", "root", ExporterConfig{FilePath: path}, context.Background())
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	foundPrefix := false
	for _, e := range entries {
		if len(e.Name()) > 0 && e.Name()[0] == '.' {
			require.Contains(t, e.Name(), "planner-trace-")
			foundPrefix = true
		}
	}
	require.True(t, foundPrefix, "temp file should use service name as prefix")

	shutdown()
}

func TestNewRoot_PushAndEvent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.json")

	tr, shutdown, err := NewRoot("svc", "root", ExporterConfig{FilePath: path}, context.Background())
	require.NoError(t, err)
	defer shutdown()

	child, done := tr.Push("child-span", attribute.String("a", "b"))
	child.Event("child-event")
	child.SetAttributes(attribute.Int("count", 42))
	done()

	require.NotNil(t, tr.Context())
	require.NotNil(t, tr.Meter())
}

func TestNewRoot_ShutdownIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.json")

	_, shutdown, err := NewRoot("svc", "root", ExporterConfig{FilePath: path}, context.Background())
	require.NoError(t, err)

	shutdown()
	shutdown() // second call should not panic
	_, err = os.Stat(path)
	require.NoError(t, err)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestNewRoot_NoHardcodedHarness(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.json")

	_, shutdown, err := NewRoot("custom-agent", "root", ExporterConfig{FilePath: path}, context.Background())
	require.NoError(t, err)
	defer shutdown()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		require.NotContains(t, e.Name(), "harness", "no hardcoded harness in temp file names")
	}
}

func TestBuildProviders_RollsBackPartialInitialization(t *testing.T) {
	t.Parallel()
	setupErr := errors.New("injected setup failure")
	tests := []struct {
		name         string
		failAt       string
		wantShutdown []string
	}{
		{name: "temporary file created", failAt: "file-trace"},
		{name: "file trace exporter created", failAt: "file-metric", wantShutdown: []string{"file-trace"}},
		{
			name:         "file exporters created",
			failAt:       "otlp-trace",
			wantShutdown: []string{"file-metric", "file-trace"},
		},
		{
			name:         "OTLP trace exporter created",
			failAt:       "otlp-metric",
			wantShutdown: []string{"otlp-trace", "file-metric", "file-trace"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			var mu sync.Mutex
			var shutdowns []string
			record := func(name string) {
				mu.Lock()
				defer mu.Unlock()
				shutdowns = append(shutdowns, name)
			}
			factories := faultFactories(tt.failAt, setupErr, record)
			createTemp := factories.createTemp
			var tempFile *os.File
			factories.createTemp = func(dir, pattern string) (*os.File, error) {
				file, err := createTemp(dir, pattern)
				tempFile = file
				return file, err
			}

			tp, mp, file, err := buildProvidersWithFactories(
				ExporterConfig{
					FilePath:           filepath.Join(dir, "trace.json"),
					OTLPEndpoint:       "trace:4317",
					MetricOTLPEndpoint: "metric:4317",
				},
				nil,
				"test",
				factories,
			)

			require.Nil(t, tp)
			require.Nil(t, mp)
			require.Nil(t, file)
			require.ErrorIs(t, err, setupErr)
			require.Equal(t, tt.wantShutdown, shutdowns)
			require.NotNil(t, tempFile)
			_, statErr := tempFile.Stat()
			require.Error(t, statErr, "rollback must close the temporary file")
			entries, readErr := os.ReadDir(dir)
			require.NoError(t, readErr)
			require.Empty(t, entries, "rollback must remove the temporary file")
		})
	}
}

func TestBuildProviders_PreservesSetupAndCleanupErrors(t *testing.T) {
	t.Parallel()
	setupErr := errors.New("metric setup failed")
	cleanupErr := errors.New("trace shutdown failed")
	dir := t.TempDir()
	factories := faultFactories("file-metric", setupErr, func(string) {})
	factories.fileTrace = func(io.Writer) (sdktrace.SpanExporter, error) {
		return &recordingExporter{
			name:        "file-trace",
			shutdownErr: cleanupErr,
			record:      func(string) {},
		}, nil
	}

	_, _, _, err := buildProvidersWithFactories(
		ExporterConfig{FilePath: filepath.Join(dir, "trace.json")},
		nil,
		"test",
		factories,
	)

	require.ErrorIs(t, err, setupErr)
	require.ErrorIs(t, err, cleanupErr)
	require.Contains(t, err.Error(), "cleanup file trace exporter")
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	require.Empty(t, entries)
}

func faultFactories(
	failAt string,
	setupErr error,
	record func(string),
) exporterFactories {
	factories := defaultExporterFactories()
	factories.fileTrace = func(io.Writer) (sdktrace.SpanExporter, error) {
		if failAt == "file-trace" {
			return nil, setupErr
		}
		return &recordingExporter{name: "file-trace", record: record}, nil
	}
	factories.fileMetric = func(io.Writer) (sdkmetric.Exporter, error) {
		if failAt == "file-metric" {
			return nil, setupErr
		}
		return &recordingMetricExporter{
			recordingExporter: recordingExporter{name: "file-metric", record: record},
		}, nil
	}
	factories.otlpTrace = func(string) (sdktrace.SpanExporter, error) {
		if failAt == "otlp-trace" {
			return nil, setupErr
		}
		return &recordingExporter{name: "otlp-trace", record: record}, nil
	}
	factories.otlpMetric = func(string) (sdkmetric.Exporter, error) {
		if failAt == "otlp-metric" {
			return nil, setupErr
		}
		return &recordingMetricExporter{
			recordingExporter: recordingExporter{name: "otlp-metric", record: record},
		}, nil
	}
	return factories
}
