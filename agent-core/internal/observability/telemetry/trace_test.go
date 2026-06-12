// Copyright (c) 2026 Nokia. All rights reserved.

package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

func TestNewRoot_EmptyConfigError(t *testing.T) {
	t.Parallel()
	_, _, err := NewRoot("test", "root", ExporterConfig{}, context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one exporter required")
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
